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
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/facebook/dns/dnsrocks/dnsdata/rdb"

	"github.com/golang/glog"
)

// implement db.DBI interface over RocksDB
type rdbdriver struct {
	db           *rdb.RDB
	path         string
	isDataSorted bool
}

func openRDB(path string) (DBI, error) {
	db, err := rdb.NewReader(path)
	if err != nil {
		return nil, err
	}

	isDataSorted := db.IsV2KeySyntaxUsed()

	driver := &rdbdriver{db: db, path: path, isDataSorted: isDataSorted}
	return driver, nil
}

func (r *rdbdriver) NewContext() Context {
	return rdb.NewContext()
}

func (r *rdbdriver) FreeContext(context Context) {
	context.Reset()
	// could do pooling like CDB but there is no buffer reusing in RocksDB/cgo at the moment - no need to bother now
}

// Find returns the first data value for the given key as a byte slice.
func (r *rdbdriver) Find(key []byte, context Context) ([]byte, error) {
	ctx := context.(*rdb.Context)

	return r.db.Find(key, ctx)
}

// FindMap returns mapID for domain e.g DB key "{mtype}{packed_domain}"
// Starting from a domain = q, first we try to get an exact match
// then, we remove 1 label at a time and try to find a wildcard match.
func (r *rdbdriver) FindMap(domain, mtype []byte, context Context) ([]byte, error) {
	if r.isDataSorted {
		return r.findMapInSortedData(domain, mtype, context)
	}

	var (
		k    = make([]byte, 0, 50)   // prime the byte array capacity
		keys = make([][]byte, 0, 10) // 10 is a sane number of subdomains we can expect in FQDN
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
		key := make([]byte, len(k))
		copy(key, k)
		keys = append(keys, key)

		// If there is no more label, break out of the loop
		if domain[0] == 0 {
			break
		}
		// otherwise, pop 1 label
		domain = domain[1+domain[0]:]
		firstLoop = false
	}

	mapID, _, err := r.db.FindFirst(keys)

	if err != nil {
		return nil, err
	}
	return mapID, nil
}

// FindMap returns mapID for domain e.g DB key "{mtype}{packed_domain}"
// Starting from a domain = q, first we try to get an exact match
// then, we remove 1 label at a time and try to find a wildcard match.
func (r *rdbdriver) findMapInSortedData(domain, mtype []byte, context Context) (mapID []byte, err error) {
	ctx := context.(*rdb.Context)

	reversedZone := reverseZoneName(domain)
	suffix := exactMatchKeyElement
	k := make([]byte, len(reversedZone)+len(mtype)+len(suffix))

	copy(k, mtype)
	copy(k[len(mtype):], reversedZone)

	prefixLen := len(mtype)

	for {
		copy(k[len(k)-len(suffix):], suffix)

		var foundKey []byte
		var foundValue []byte
		foundKey, foundValue, err = r.findClosest(k, ctx)
		if err != nil {
			break
		}

		if bytes.Equal(foundKey, k) {
			mapID = foundValue[4:]
			break
		}

		if len(foundKey) < prefixLen ||
			!bytes.Equal(foundKey[:prefixLen], k[:prefixLen]) {
			// reached end of maps data segment
			break
		}

		foundLabel := foundKey[prefixLen : len(foundKey)-1]
		length := findCommonLongestPrefix(reversedZone, foundLabel)
		if length == 0 {
			break
		}

		// k already has necessary data - we just need to cut it at proper point
		k[prefixLen+length] = 0
		k = k[:prefixLen+length+1+len(suffix)]
		suffix = wildcardKeyElement
	}

	return mapID, err
}

var (
	firstIPv4 = net.ParseIP("0000:0000:0000:0000:0000:ffff:0000:0000") // the very first IPv4 address according to RFC-2765
)

func isIPv4(addr net.IP) bool {
	return addr != nil && (len(addr) == net.IPv4len || net.IP.Equal(addr[:12], firstIPv4[:12]))
}

// GetLocationByMap finds and returns location and mask. If the location is not found, returns nil and 0.
func (r *rdbdriver) GetLocationByMap(ipnet *net.IPNet, mapID []byte, context Context) (loc []byte, mlen uint8, err error) {
	// Lookup the subnet in IP MAP: map ID, IP address -> LocID, mask
	// Build key prefix: "\000\000\000!{MapID}"
	// We prime the key byte array (fullKey) with the key prefix,
	// followed by the original IP address in V6 format.
	nmap := len(mapID)
	fullKey := make([]byte, 4+nmap+net.IPv6len+1) // 4 bytes for prefix, n bytes for mapID, and the rest is IP and masklen
	copy(fullKey, ipMapRangePointKeyElement)      // prefix, 4 bytes
	copy(fullKey[4:], mapID)                      // mapID, n bytes
	copy(fullKey[4+nmap:], ipnet.IP.To16())
	reqMaskLen, _ := ipnet.Mask.Size()
	if isIPv4(ipnet.IP) {
		reqMaskLen += 128 - 32
	}
	copy(fullKey[4+nmap+16:], []byte{uint8(reqMaskLen)})

	ctx := context.(*rdb.Context)

	// NOTE: Rearranger has merging on adjacent locations with same mask and locID,
	// so findClosest() might return the key that will match some other IP. It is fine for our purposes.
	// foundVal will consist of mask (1 byte) and LocID (2 bytes); if LocID is null, then there will be mask only.
	foundKey, foundVal, err := r.db.FindClosest(fullKey, ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(foundVal) == 0 {
		return nil, 0, nil // consistent with the return at the end of cdbdriver.go:/GetLocationByMap
	}
	if len(foundVal) < 4 {
		err = fmt.Errorf("short value: length %d, value %v, map %v", len(foundVal), foundVal, mapID)
		return nil, 0, err
	}
	foundVal = foundVal[4:] // skip over the multi-value header - see ../dnsdata/rdb/rdb_util.go:/Put
	mlen = foundKey[len(foundKey)-1]
	switch len(foundVal) {
	case 0:
		// Rearranger will always add /0 mask, so if anything - the empty location will match
		return nil, mlen, nil
	case 2:
		loc = foundVal
		return loc, mlen, nil
	default:
		if len(foundVal) < 2 {
			err = fmt.Errorf("Invalid location length %d, value %v", len(foundVal), foundVal)
			return nil, 0, err
		}
		if foundVal[0] != 0xff {
			loc = foundVal
			return loc, mlen, nil
		}
		locLen, foundVal := foundVal[1], foundVal[2:]
		if int(locLen) > len(foundVal) {
			err = fmt.Errorf("invalid location length byte %d > %d", locLen, len(foundVal))
			return nil, 0, err
		}
		loc = foundVal[:locLen]
		return loc, mlen, nil
	}
}

func (r *rdbdriver) Close() error {
	return r.db.Close()
}

func (r *rdbdriver) Reload(path string) (DBI, error) {
	start := time.Now()
	if path == r.path {
		glog.Infof("Doing catchUpWithPrimary for RDB")
		if err := r.db.CatchWithPrimary(); err != nil {
			return nil, err
		}
		reloadTime := time.Now()
		glog.Infof("Caught up on primary for RocksDB in %v", reloadTime.Sub(start))
		return r, nil
	}
	glog.Infof("Doing full RDB reload, new path=%s", path)
	newDB, err := openRDB(path)
	if err != nil {
		return nil, err
	}
	reloadTime := time.Now()
	glog.Infof("Finished full RDB reload in %v", reloadTime.Sub(start))
	return newDB, nil
}

// GetStats reports DB backend stats
func (r *rdbdriver) GetStats() map[string]int64 {
	return r.db.GetStats()
}

func (r *rdbdriver) findClosest(key []byte, ctx Context) (k []byte, v []byte, err error) {
	c := ctx.(*rdb.Context)
	k, v, err = r.db.FindClosest(key, c)
	if err != nil {
		return nil, nil, err
	}

	return k, v, err
}

// ForEach calls a function for each key match.
// The function takes a byte slice as a value and return an error.
// if error is not nil, the loop will stop.
func (r *rdbdriver) ForEach(key []byte, f func(value []byte) error, context Context) (err error) {
	ctx := context.(*rdb.Context)

	return r.db.ForEach(key, f, ctx)
}

// FindClosestKey searches for closest key which is smaller or equal to provided key
func (r *rdbdriver) FindClosestKey(key []byte, context Context) ([]byte, error) {
	k, _, err := r.findClosest(key, context)

	return k, err
}

func (r *rdbdriver) ClosestKeyFinder() ClosestKeyFinder {
	if r.isDataSorted {
		return r
	}

	return nil
}
