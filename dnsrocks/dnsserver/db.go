/*
Copyright (c) Meta Platforms, Inc. and affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dnsserver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	lru "github.com/hashicorp/golang-lru"

	"github.com/facebookincubator/dns/dnsrocks/db"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/stats"
)

// CacheConfig has knobs to modify caching behaviour.
type CacheConfig struct {
	Enabled    bool
	LRUSize    int
	WRSTimeout int64
}

// DBConfig contains our DNS Database configuration.
type DBConfig struct {
	Path           string
	ControlPath    string
	Driver         string
	ReloadInterval int
	ReloadTimeout  time.Duration
	WatchDB        bool
	ValidationKey  []byte
}

// ReloadType - how to reload the DB
type ReloadType int

const (
	// FullReload - close and open DB
	FullReload ReloadType = iota
	// PartialReload - Catch up on WAL
	PartialReload
)

// ReloadSignal is a signal we use to tell server that something/someone requested DB reload
type ReloadSignal struct {
	Kind    ReloadType
	Payload string
}

// NewFullReloadSignal is a helper to create new ReloadSignal of kind FullReload
func NewFullReloadSignal(newDBPath string) *ReloadSignal {
	return &ReloadSignal{
		Kind:    FullReload,
		Payload: newDBPath,
	}
}

// NewPartialReloadSignal is a helper to create new ReloadSignal of kind PartialReload
func NewPartialReloadSignal() *ReloadSignal {
	return &ReloadSignal{
		Kind:    PartialReload,
		Payload: "",
	}
}

// group of control files we watch for to reload DB
const (
	ControlFileFullReload    = "switchdb"
	ControlFilePartialReload = "reload"
)

// HandlerConfig contains config used when handling a DNS request.
type HandlerConfig struct {
	// Controls whether responses are always compressed not depending on response buffer size
	AlwaysCompress bool
}

// FBDNSDB is the DNS DB handler.
type FBDNSDB struct {
	ReloadChan    chan ReloadSignal
	dnsdb         *db.DB
	dbConfig      DBConfig
	handlerConfig HandlerConfig
	cacheConfig   CacheConfig
	reloadMu      sync.RWMutex
	done          chan struct{}
	lru           *lru.Cache
	logger        Logger
	stats         stats.Stats
	Next          plugin.Handler
}

// NewFBDNSDBBasic initialize a new FBDNSDB. Reloading strategy is left to be set.
func NewFBDNSDBBasic(handlerConfig HandlerConfig, dbConfig DBConfig, cacheConfig CacheConfig, l Logger, s stats.Stats) (t *FBDNSDB, err error) {
	var lrucache *lru.Cache
	if cacheConfig.Enabled {
		if lrucache, err = lru.New(cacheConfig.LRUSize); err != nil {
			return
		}
	}

	tdb := &FBDNSDB{
		handlerConfig: handlerConfig,
		dbConfig:      dbConfig,
		cacheConfig:   cacheConfig,
		lru:           lrucache,
		logger:        l,
		stats:         s,
		done:          make(chan struct{}),
		ReloadChan:    make(chan ReloadSignal),
	}

	return tdb, nil
}

// NewFBDNSDB initialize a new FBDNSDB and set up DB reloading
func NewFBDNSDB(handlerConfig HandlerConfig, dbConfig DBConfig, cacheConfig CacheConfig, l Logger, s stats.Stats) (t *FBDNSDB, err error) {
	tdb, err := NewFBDNSDBBasic(handlerConfig, dbConfig, cacheConfig, l, s)
	if err != nil {
		return nil, err
	}
	go func() {
		for s := range tdb.ReloadChan {
			err := tdb.Reload(s)
			if err != nil {
				glog.Errorf("Failed to reload: %v", err)
			}
		}
	}()

	if tdb.dbConfig.ReloadInterval > 0 {
		go tdb.PeriodicDBReload(tdb.dbConfig.ReloadInterval)
	}

	return tdb, nil
}

func filterEvent(op fsnotify.Op) bool {
	return op&fsnotify.Create != 0 ||
		op&fsnotify.Write != 0 ||
		op&fsnotify.Rename != 0 ||
		op&fsnotify.Chmod != 0
}

func filterControlEvent(op fsnotify.Op) bool {
	return op&fsnotify.Create != 0
}

func getNewDBPath(path string) (string, error) {
	newPath := ""
	f, err := os.Open(path)
	if err != nil {
		return newPath, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return newPath, err
	}
	newPath = strings.TrimSpace(string(b))
	_, err = os.Stat(newPath)
	if os.IsNotExist(err) {
		return newPath, fmt.Errorf("New path %s doesn't exist", newPath)
	}
	return newPath, nil
}

func prepareDBWatcher(watchPath string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		glog.Errorf("Can't setup watcher: %v", err)
		return nil, fmt.Errorf("setting up fsnotify watcher: %w", err)
	}

	if err = watcher.Add(watchPath); err != nil {
		glog.Errorf("Can't add file to watcher :%v", err)
		return watcher, fmt.Errorf("adding %q to fsnotify watcher: %w", watchPath, err)
	}
	return watcher, nil
}

// PeriodicDBReload is to enforce db reload in case db watch fails or stuck
func (h *FBDNSDB) PeriodicDBReload(reloadInt int) {
	d := time.Duration(reloadInt) * time.Second
	ticker := time.NewTicker(d)
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.ReloadChan <- *NewPartialReloadSignal()
		}
	}
}

func (h *FBDNSDB) watchDBAndReload(watcher *fsnotify.Watcher) (err error) {
	for {
		select {
		case err = <-watcher.Errors:
			if err == nil {
				return nil
			}
			glog.Errorf("Watcher encountered error: %v", err)
			return fmt.Errorf("fsnotify watcher error: %w", err)
		case <-h.done:
			return nil
		case ev := <-watcher.Events:
			if filterEvent(ev.Op) && path.Clean(ev.Name) == h.dbConfig.Path {
				h.ReloadChan <- *NewPartialReloadSignal()
			}
		}
	}
}

// WatchDBAndReload refreshes the data view on DB file change
func (h *FBDNSDB) WatchDBAndReload() error {
	// Watch the whole dir as file FD might change
	watchdir := path.Dir(h.dbConfig.Path)
	watcher, err := prepareDBWatcher(watchdir)
	if watcher != nil {
		defer watcher.Close()
	}
	if err != nil {
		return err
	}

	return h.watchDBAndReload(watcher)
}

// cleanupSignalFile removes processed signal files
func (h *FBDNSDB) cleanupSignalFile(s ReloadSignal) error {
	if h.dbConfig.ControlPath == "" {
		return nil
	}
	p := ""
	switch s.Kind {
	case FullReload:
		p = path.Join(h.dbConfig.ControlPath, ControlFileFullReload)
	case PartialReload:
		p = path.Join(h.dbConfig.ControlPath, ControlFilePartialReload)
	default:
		return fmt.Errorf("Unknown reload signal %v", s)
	}
	return os.RemoveAll(p)
}

func (h *FBDNSDB) watchControlDirAndReload(watcher *fsnotify.Watcher) (err error) {
	for {
		select {
		case err = <-watcher.Errors:
			if err == nil {
				return nil
			}
			glog.Errorf("Watcher encountered error: %v", err)
			return fmt.Errorf("fsnotify watcher error: %w", err)
		case <-h.done:
			return nil
		case ev := <-watcher.Events:
			if !filterControlEvent(ev.Op) {
				continue
			}
			cp := path.Clean(ev.Name)
			_, name := path.Split(cp)

			switch name {
			case ControlFilePartialReload:
				glog.Infof("Found partial reload trigger file")
				h.ReloadChan <- *NewPartialReloadSignal()
			case ControlFileFullReload:
				glog.Infof("Found full reload trigger file")
				newPath, err := getNewDBPath(cp)
				if err != nil {
					return fmt.Errorf("getting new DB path: %w", err)
				}
				h.ReloadChan <- *NewFullReloadSignal(newPath)
			default:
				glog.Infof("Ignoring unknown file in control directory: %s", name)
			}
		}
	}
}

// WatchControlDirAndReload refreshes the data view on control file change.
// We monitor for two type of reload signals: full reload, when we switch to new file/directory,
// and partial reload when we try to catch up on WAL if possible.
// CDB does full reload in both cases as it's immutable.
func (h *FBDNSDB) WatchControlDirAndReload() error {
	watcher, err := prepareDBWatcher(h.dbConfig.ControlPath)
	if watcher != nil {
		defer watcher.Close()
	}
	if err != nil {
		return err
	}
	return h.watchControlDirAndReload(watcher)
}

// Load loads a DB file
func (h *FBDNSDB) Load() (err error) {
	var dnsdb *db.DB
	glog.Infof("Loading %s using %s driver", h.dbConfig.Path, h.dbConfig.Driver)
	if dnsdb, err = db.Open(h.dbConfig.Path, h.dbConfig.Driver); err != nil {
		return err
	}
	h.dnsdb = dnsdb
	h.stats.IncrementCounter("DNS_db.reload")
	h.stats.ResetCounter("DNS_db.ErrReloadTimeout")
	return nil
}

// Reload reload the db
func (h *FBDNSDB) Reload(s ReloadSignal) (err error) {
	newPath := ""

	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()

	switch s.Kind {
	case FullReload:
		if s.Payload == "" {
			return fmt.Errorf("Asked for full reload but no path provided")
		}
		newPath = s.Payload
	case PartialReload:
		newPath = h.dbConfig.Path
	}

	var newDB *db.DB
	newDB, err = h.dnsdb.Reload(newPath, h.dbConfig.ValidationKey, h.dbConfig.ReloadTimeout)
	if err != nil {
		if errors.Is(err, db.ErrValidationKeyNotFound) {
			h.stats.IncrementCounter("DNS_db.ErrValidationKeyNotFound")
		}
		if errors.Is(err, db.ErrReloadTimeout) {
			h.stats.IncrementCounter("DNS_db.ErrReloadTimeout")
		}
		return
	}

	// if we didn't timeout and reloading finished without errors
	h.dnsdb = newDB
	h.dbConfig.Path = newPath

	if h.cacheConfig.Enabled && h.lru != nil {
		h.lru.Purge()
	}

	if err := h.cleanupSignalFile(s); err != nil {
		return err
	}
	h.stats.IncrementCounter("DNS_db.reload")
	return nil
}

// AcquireReader return a DB reader which increment the refcount to the DB.
// This makes sure that we can handle DB reloading from other goroutine while
// providing a consistent view on the DB during a query.
// The Reader must be `Close`d when not needed anymore.
func (h *FBDNSDB) AcquireReader() (db.Reader, error) {
	h.reloadMu.RLock()
	defer h.reloadMu.RUnlock()
	return db.NewReader(h.dnsdb)
}

// Close closes the database. It also takes care of closing the channel used
// for periodic reloading.
func (h *FBDNSDB) Close() {
	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()
	glog.Infof("Closing DB")
	close(h.done)
	close(h.ReloadChan)
	h.dnsdb.Destroy()
}

// ReportBackendStats refreshes backend statistics in server stats
func (h *FBDNSDB) ReportBackendStats() {
	// ReportBackendStats can be called the moment we reload
	h.reloadMu.RLock()
	defer h.reloadMu.RUnlock()
	for k, v := range h.dnsdb.GetStats() {
		h.stats.ResetCounterTo(k, v)
	}
}

// ValidateDbKey checks whether record of certain key is in db
func (h *FBDNSDB) ValidateDbKey(dbKey []byte) error {
	return h.dnsdb.ValidateDbKey(dbKey)
}
