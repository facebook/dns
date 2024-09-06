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
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/repustate/go-cdb" // local changes include the use of Spooky Hash v1 instead of hash.Hash32
)

// implement db.DBI interface with repustate/go-cdb
type cdbdriver struct {
	db          *cdb.Cdb
	contextPool sync.Pool
}

var newCdbContextFunc = func() interface{} {
	return cdb.NewContext()
}

func openCDB(name string) (DBI, error) {
	c, err := cdb.Open(name)
	if err != nil {
		return nil, err
	}
	driver := &cdbdriver{c, sync.Pool{New: newCdbContextFunc}}
	return driver, nil
}

func (c *cdbdriver) NewContext() Context {
	context := c.contextPool.Get().(Context)
	return context
}

func (c *cdbdriver) FreeContext(context Context) {
	context.Reset()
	c.contextPool.Put(context)
}

func (c *cdbdriver) FindStart(context Context) {
	ctx := context.(*cdb.Context)
	c.db.FindStart(ctx)
}

func (c *cdbdriver) FindNext(key []byte, context Context) ([]byte, error) {
	ctx := context.(*cdb.Context)
	return c.db.FindNext(key, ctx)
}

// Find returns the first data value for the given key as a byte slice.
// Find is the same as FindStart followed by FindNext.
func (c *cdbdriver) Find(key []byte, context Context) ([]byte, error) {
	c.FindStart(context)
	return c.FindNext(key, context)
}

// FindMap returns mapID for domain e.g DB key "{mtype}{packed_domain}{MapID}"
// Starting from a domain = q, first we try to get an exact match
// then, we remove 1 label at a time and try to find a wildcard match.
func (c *cdbdriver) FindMap(domain, mtype []byte, context Context) ([]byte, error) {
	var (
		k = make([]byte, 0, 50) // prime the byte array capacity
	)

	k = append(k, mtype...)
	firstLoop := true

	for {
		dlen := 2
		k = append(k[:dlen], domain[:]...)
		dlen += len(domain)

		if !firstLoop {
			k = append(k[:dlen], wildcardKeyElement...)
		} else {
			k = append(k[:dlen], exactMatchKeyElement...)
		}
		c.FindStart(context)
		mapID, err := c.FindNext(k, context)

		// We found a match. Copy this to the MapID
		if err == nil {
			return mapID, nil
		} else if !errors.Is(err, io.EOF) {
			return nil, err
		}
		// If there is no more label, break out of the loop
		if domain[0] == 0 {
			return nil, nil
		}
		// otherwise, pop 1 label
		domain = domain[1+domain[0]:]
		firstLoop = false
	}
}

// GetLocationByMap finds and returns location and mask. If the location is not found, returns nil and 0.
func (c *cdbdriver) GetLocationByMap(ipnet *net.IPNet, mapID []byte, context Context) ([]byte, uint8, error) {
	var (
		clientIP = ipnet.IP
		// maskLens is an array or mask length
		maskLens []byte
		// Result with more specific mask cannot be used, there is no need to search
		// for matching subnet in them.
		maxMask uint8
		isv4    bool
	)

	// Lookup the subnet in IP MAP: map ID, IP subnet -> LocID
	// Build key prefix: "\000%{MapID}"
	k := make([]byte, 4+net.IPv6len+1)
	copy(k, ipMapKeyElement)
	dlen := 2
	copy(k[dlen:], mapID)

	dlen += 2

	// Find the maskLens
	tmpmask, _ := ipnet.Mask.Size()
	maxMask = uint8(tmpmask)

	if ipnet.IP.To4() != nil {
		// We only work with v6-mapped IPs
		maxMask += 96
		isv4 = true
	}
	// maskLens DB key: "\000/"
	bitmapKey := maskLensKeyElement
	if SeparateBitMap {
		if isv4 {
			bitmapKey = maskLensKeyElementv4
		} else {
			bitmapKey = maskLensKeyElementv6
		}
	}
	c.FindStart(context)
	maskLens, err := c.FindNext(bitmapKey, context)

	if errors.Is(err, io.EOF) {
		// No maskLens found, return what we have. e.g default l.LocID == {0, 0}
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	// We prime the key byte array (k) with the key prefix ([0:dlen[),
	// followed by the original IP address in V6 format.
	// While we iterate through the maxLens array, we will apply the
	// cachedCIDRMask to the IP we copied starting at offset `dlen` for a
	// length of net.IPv6len. Because we go from the longest CIDRMask, to the
	// smallest, we can safely mutate `k` inplace.
	copy(k[dlen:], clientIP.To16())

	// Iterate through the masks, as soon as we find a match, break out the loop.
	// NOTE: this assumes masks are from the most to the least specific as in:
	// [128 120 96 56 0]
	for _, mask := range maskLens {
		if mask > maxMask {
			continue
		}
		// Finish creating the search key:
		// "{key_prefix}{ipv6_subnet_bitmap}"
		currentCIDRMask := cachedCIDRMask[mask]
		for i := 0; i < net.IPv6len; i++ {
			k[dlen+i] &= currentCIDRMask[i]
		}
		k[dlen+net.IPv6len] = mask
		c.FindStart(context)
		locID, err := c.FindNext(k, context)
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}

		if len(locID) < 2 {
			err = fmt.Errorf("Invalid location length %d, value %v", len(locID), locID)
			return nil, 0, err
		}
		if locID[0] != 0xff {
			return locID, mask, nil
		}
		locLen, locID := locID[1], locID[2:]
		if int(locLen) > len(locID) {
			err = fmt.Errorf("invalid location length byte %d > %d", locLen, len(locID))
			return nil, 0, err
		}
		locID = locID[:locLen]
		return locID, mask, nil
	}
	return nil, 0, nil
}

func (c *cdbdriver) Close() error {
	return c.db.Close()
}

func (c *cdbdriver) Reload(path string) (DBI, error) {
	start := time.Now()
	glog.Infof("Doing full CDB reload, new path=%s", path)
	newDBI, err := openCDB(path)
	if err != nil {
		return nil, err
	}
	reloadTime := time.Now()
	glog.Infof("Finished full CDB reload in %v", reloadTime.Sub(start))
	return newDBI, nil
}

// GetStats reports DB backend stats
func (c *cdbdriver) GetStats() map[string]int64 {
	// dunno if we can derive any from CDB
	return map[string]int64{}
}

// ForEach calls a function for each key match.
// The function takes a byte slice as a value and return an error.
// if error is not nil, the loop will stop.
func (c *cdbdriver) ForEach(key []byte, f func(value []byte) error, context Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	c.FindStart(context)
	for {
		v, err := c.FindNext(key, context)

		// No more row
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		if err = f(v); err != nil {
			break
		}
	}

	return err
}

// ClosestKeyFinder always returns nil for CDB as keys are not sorted
func (c *cdbdriver) ClosestKeyFinder() ClosestKeyFinder {
	return nil
}
