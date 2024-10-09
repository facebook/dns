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

package db

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/facebook/dns/dnsrocks/dnsdata"

	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// DBI interface represents the pluggable API for the backing storage
// nolint:revive
type DBI interface {
	NewContext() Context
	Find(key []byte, context Context) ([]byte, error)
	ForEach(key []byte, f func(value []byte) error, context Context) error
	FreeContext(Context)
	FindMap(domain, mtype []byte, context Context) ([]byte, error)
	// GetLocationByMap returns the location ID, including the 2-byte header for long IDs.
	GetLocationByMap(ipnet *net.IPNet, mapID []byte, context Context) ([]byte, uint8, error)
	Close() error
	Reload(path string) (DBI, error)
	GetStats() map[string]int64

	// Gets ClosestKeyFinder associated with DBI if it is supported, nil otherwise
	ClosestKeyFinder() ClosestKeyFinder
}

// ClosestKeyFinder allows to search for closest smaller or equal key in the database
type ClosestKeyFinder interface {
	// FindClosestKey checks DB if provided key exists or
	// closest smaller key
	FindClosestKey(key []byte, context Context) ([]byte, error)
}

// Context interface is an ADT representing the state carried across queries to DBI
// like iterator state or potentially query cache
type Context interface {
	Reset()
}

// DB implements a customized db
type DB struct {
	dbi         DBI
	destroyable bool
	refCount    uint64
	l           sync.RWMutex
}

// Reader will be able to perform DNS queries.
// It wraps DB to carry query context and properly count reference count to DB
type Reader interface {
	FindLocation(qname []byte, m *dns.Msg, ip string) (ecs *dns.EDNS0_SUBNET, loc *Location, err error)
	IsAuthoritative(q []byte, locID ID) (ns bool, auth bool, zoneCut []byte, err error)
	FindAnswer(q []byte, packedControlName []byte, qname string, qtype uint16, locID ID, a *dns.Msg, maxAnswer int) (bool, bool)

	EcsLocation(q []byte, ecs *dns.EDNS0_SUBNET) (*Location, error)
	ResolverLocation(q []byte, ip string) (*Location, error)
	findLocation(q []byte, mtype []byte, ipnet *net.IPNet) (*Location, error)

	ForEach(key []byte, f func(value []byte) error) (err error)
	ForEachResourceRecord(domainName []byte, locID ID, parseRecord func(result []byte) error) error

	Close()
}

// DataReader wraps an DB to carry a Context around.
// This structure will be able to perform DNS queries.
type DataReader struct {
	db      *DB
	context Context
}

type sortedDataReader struct {
	DataReader
	closestKeyFinder ClosestKeyFinder
}

// ErrValidationKeyNotFound - Key not found in DB file
var ErrValidationKeyNotFound = errors.New("validation key not found in DB")

// ErrReloadTimeout - DB reload timeout
var ErrReloadTimeout = errors.New("DB reload timeout")

// Open opens the named file read-only and returns a new db object.  The file
// should exist and be a compatible (CDB or RDB) database file.
func Open(name string, driver string) (*DB, error) {
	var openfunc func(string) (DBI, error)

	switch driver {
	case "cdb":
		openfunc = openCDB
	case "rocksdb":
		openfunc = openRDB
	default:
		return nil, fmt.Errorf("%s: invalid argument; valid values are: cdb, rocksdb", driver)
	}
	dbi, err := openfunc(name)
	if err != nil {
		return nil, err
	}
	return &DB{dbi: dbi}, nil
}

// NewReader returns a new DB reader to be used to perform DNS record search in DB.
func NewReader(db *DB) (Reader, error) {
	if db == nil {
		return &DataReader{}, fmt.Errorf("Cannot create new reader, DB is not initialized")
	}
	db.l.Lock()
	defer db.l.Unlock()
	db.refCount++
	context := db.dbi.NewContext()

	reader := DataReader{db: db, context: context}

	closestKeyFinder := db.dbi.ClosestKeyFinder()

	if closestKeyFinder != nil {
		return &sortedDataReader{DataReader: reader, closestKeyFinder: closestKeyFinder}, nil
	}

	return &reader, nil
}

// Destroy destroys a DB. It marks it as destroyable. If there is no refCount
// it will close the db, if there is refcounts left, the last reference to
// call Reader.Close() will close the db.
func (f *DB) Destroy() {
	f.l.Lock()
	defer f.l.Unlock()
	f.destroyable = true
	if f.refCount == 0 {
		glog.Infof("refcount == 0: Closing DB")
		f.dbi.Close()
	}
}

// Reload reloads a DB. In case of immutable CDB it will return new DB and close the old one.
// In case of RocksDB, if path is the same as it was it tries to catch up with WAL and returns existing DB.
// If path is different it will return new DB and close the old one. The validationKey is used
// to verify that the format of the DB file is valid, by checking for the existence of a key
// that is known to exist. If the DB file is invalid, the old DB will continue to be used.
func (f *DB) Reload(path string, validationKey []byte, reloadTimeout time.Duration) (*DB, error) {
	c := make(chan int)
	ctx, cancel := context.WithTimeout(context.Background(), reloadTimeout)
	defer cancel()
	var newDBI DBI
	var m sync.Mutex
	var destroyNewDbi bool
	var err error

	// reload goroutine
	go func() {
		var localDBI DBI
		localDBI, err = f.dbi.Reload(path)
		m.Lock()
		defer m.Unlock()
		if localDBI != nil && destroyNewDbi && localDBI != f.dbi {
			localDBI.Close()
		} else {
			newDBI = localDBI
		}
		close(c)
	}()

	select {
	case <-ctx.Done():
		// when we hit timeout
		// 1) If the newDBI is already created, we want to have it freed.
		// This can due to current select block been put to sleep\preempted for more then reloadTimeout
		// 2) If newDBI is NOT yet created, we want to set destroyNewDbi to inform reload routine the newDBI
		// is no longer needed.
		// Both newDBI and destroyNewDbi are protected by critical section.
		m.Lock()
		defer m.Unlock()
		if newDBI != nil && newDBI != f.dbi {
			newDBI.Close()
		} else {
			destroyNewDbi = true
		}
		return f, ErrReloadTimeout
	case <-c:
		// if we reach here, it mean the above reload goroutine already returned
		if err != nil {
			return f, err
		}

		// Validate newDBI
		newDB := &DB{dbi: newDBI}
		err = newDB.validateDbKeyOrDestroy(validationKey)
		if err != nil {
			glog.Errorf("Key validation for New DBI failed, using old DB instead")
			return f, err
		}

		if newDBI != f.dbi {
			glog.Infof("New DBI, old one will be destroyed")
			// we have to deal with it here in this fashion because we handle refcounter on this level
			f.Destroy()
			return newDB, nil
		}
	}

	return f, nil
}

// validateDbKeyOrDestroy validates DB with the validationKey, and destroys the
// DB on failure
func (f *DB) validateDbKeyOrDestroy(validationKey []byte) error {
	err := f.ValidateDbKey(validationKey)
	if err != nil {
		f.Destroy()
		return err
	}
	return nil
}

// ValidateDbKey checks whether record of certain key is in db
func (f *DB) ValidateDbKey(dbKey []byte) error {
	if len(dbKey) == 0 {
		return nil
	}

	var (
		err      error
		keyFound bool
	)
	parseResult := func(_ []byte) error {
		// set to true as soon as first key found
		keyFound = true
		return nil
	}
	reader, err := NewReader(f)
	if err != nil {
		return err
	}

	err = reader.ForEach(dbKey, parseResult)
	reader.Close()
	if err != nil {
		return fmt.Errorf("reader error in ValidateDBKey: %w", err)
	}

	if !keyFound {
		return ErrValidationKeyNotFound
	}

	return nil
}

// GetStats reports DB backend stats
func (f *DB) GetStats() map[string]int64 {
	return f.dbi.GetStats()
}

// Data returns the first data value for the given key.
// If no such record exists, it returns EOF.
func (r *DataReader) Data(key []byte) ([]byte, error) {
	return r.Find(key)
}

// Find returns the first data value for the given key as a byte slice.
// Find is the same as FindStart followed by FindNext.
func (r *DataReader) Find(key []byte) ([]byte, error) {
	return r.db.dbi.Find(key, r.context)
}

// ForEach calls a function for each key match.
// The function takes a byte slice as a value and return an error.
// if error is not nil, the loop will stop.
func (r *DataReader) ForEach(key []byte, f func(value []byte) error) (err error) {
	return r.db.dbi.ForEach(key, f, r.context)
}

// Close close a reader. This puts back a context in the pool
func (r *DataReader) Close() {
	r.db.dbi.FreeContext(r.context)
	r.db.l.Lock()
	defer r.db.l.Unlock()
	r.db.refCount--
	if r.db.destroyable && r.db.refCount == 0 {
		glog.Infof("refcount == 0 && destroyable: Closing DB")
		r.db.dbi.Close()
	}
}

// ForEachResourceRecord calls parseRecord for each RR record in DB in provided AND default location
func (r *DataReader) ForEachResourceRecord(domainName []byte, locID ID, parseRecord func(result []byte) error) error {
	var err error

	key := make([]byte, len(locID)+len(domainName))

	if !locID.IsZero() {
		key = append(key[:0], locID...)
		key = append(key, domainName...)
		err = r.ForEach(key, parseRecord)
		if err != nil {
			glog.Errorf("Error %v", err)
			return err
		}
	}

	key = append(key[:0], ZeroID...)
	key = append(key, domainName...)
	err = r.ForEach(key, parseRecord)
	if err != nil {
		glog.Errorf("Error %v", err)
		return err
	}

	return nil
}

// ForEachResourceRecord calls parseRecord for each RR record in DB in provided AND default location
func (r *sortedDataReader) ForEachResourceRecord(domainName []byte, locID ID, parseRecord func(result []byte) error) error {
	var err error

	key := make([]byte, len(domainName)+max(len(locID), len(ZeroID))+len(dnsdata.ResourceRecordsKeyMarker))
	copy(key, []byte(dnsdata.ResourceRecordsKeyMarker))

	reverseZoneNameToBuffer(domainName, key[len(dnsdata.ResourceRecordsKeyMarker):])

	locationIndex := len(dnsdata.ResourceRecordsKeyMarker) + len(domainName)

	if !locID.IsZero() {
		copy(key[locationIndex:], locID)
		err = r.ForEach(key, parseRecord)
		if err != nil {
			glog.Errorf("Error %v", err)
			return err
		}
	}

	copy(key[locationIndex:], ZeroID)
	err = r.ForEach(key, parseRecord)
	if err != nil {
		glog.Errorf("Error %v", err)
		return err
	}

	return nil
}
