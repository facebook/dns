/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package db

import (
	"bytes"
	"net"
	"os"

	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// Define some constants
// maskLensKeyElement
var maskLensKeyElement = []byte{0, '/'}
var maskLensKeyElementv4 = []byte{0, '4'}
var maskLensKeyElementv6 = []byte{0, '6'}

// ipMapKeyElement
var ipMapKeyElement = []byte{0, '%'}

// same as ../dnsdata/subnetranger.go:/RangePointKeyMarker
var ipMapRangePointKeyElement = []byte("\000\000\000!")

// wildcardKeyElement
var wildcardKeyElement = []byte{'*'}

// exactMatchKeyElement
var exactMatchKeyElement = []byte{'='}

var cachedCIDRMask [129]net.IPMask

var SeparateBitMap bool

func init() {
	SeparateBitMap = os.Getenv("FBDNS_SEPARATE_MASKLENS") != ""

	for i := 0; i < len(cachedCIDRMask); i++ {
		cachedCIDRMask[i] = net.CIDRMask(i, 128)
	}
}

// ID is an alias for []byte, used for Map IDs and Location IDs.
// It is always at least 2 bytes long.  For long IDs, it contains
// the header and contents.
type ID []byte

var (
	// ZeroID is the value used to represent an empty Map ID or Location ID
	ZeroID ID = []byte{0, 0}

	// EmptyLocation is used to index (prefix) non-location aware qnames for RR's
	EmptyLocation = Location{MapID: ZeroID, LocID: ZeroID}
)

// IsZero is true when its LocID is ZeroID.
func (id ID) IsZero() bool {
	return bytes.Equal(id, ZeroID)
}

// Contents are the ID bytes minus any long-ID header.
func (id ID) Contents() []byte {
	if len(id) <= 2 {
		return id
	}
	return id[2:]
}

// Location is a native representation of a DNS location representation.
// It holds:
// the MapID it belongs to
// For ECS MapID, the associated mask.
// The LocID, the location ID used to find matching records.
type Location struct {
	MapID ID    // The map in which we found the name.
	Mask  uint8 // The subnet mask we found a match for. Used for ECS.
	LocID ID    // The location ID.
}

// FindECS finds a EDNS0_SUBNET option in a DNS Msg.
// It returns a pointer to dns.EDNS0_SUBNET or nil when there is not such
// EDNS0 option.
func FindECS(m *dns.Msg) *dns.EDNS0_SUBNET {
	if o := m.IsEdns0(); o != nil {
		for _, opt := range o.Option {
			switch opt := opt.(type) {
			case *dns.EDNS0_SUBNET:
				return opt
			}
		}
	}
	return nil
}

// FindLocation find a Location given a wired format qname, *dns.Msg and a
// resolver IP
// First, it tries to find a matching location for a subnet client.
// Then, it tries to find a matching location for a resolver IP.
// return nil for Location if no location were found.
// error will be set on error.
func (r *DataReader) FindLocation(qname []byte, ecs *dns.EDNS0_SUBNET, ip string) (loc *Location, err error) {
	// This defer block is used to catch bad DB and recover the panic that
	// used to be handled in db.find.
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
			ecs = nil
			loc = nil
		}
	}()

	// Check if there is ECS option and look for a matching location
	if ecs != nil {
		// ECS location lookup and set Scope accordingly
		loc, err = r.EcsLocation(qname, ecs)
		if err != nil {
			glog.Errorf("Failed to lookup ECS location %s", err)
			return nil, err
		}
	}
	// resolver location lookup if we did not find any Client subnet match.
	if loc == nil || loc.LocID.IsZero() {
		loc, err = r.ResolverLocation(qname, ip)
	}
	return loc, err
}

// findLocation finds the `Location` in mtype maps that matches the `ipnet`, and returns Location.
// If no mtype is found for the domain, Location.MapID will be {0, 0}
func (r *DataReader) findLocation(q []byte, mtype []byte, ipnet *net.IPNet) (*Location, error) {
	var location = EmptyLocation

	// FindMap looks up mapID for domain e.g DB key "{mtype}{packed_domain}{MapID}"
	// Starting from a domain = q, first we try to get an exact match
	// then, we remove 1 label at a time and try to find a wildcard match.
	// If there is a map found, overwrites mapID.
	mapID, err := r.db.dbi.FindMap(q, mtype, r.context)
	if err != nil {
		return nil, err
	}
	if mapID != nil {
		location.MapID = make([]byte, len(mapID))
		copy(location.MapID, mapID)
	}

	// Find the location id
	locID, mask, err := r.db.dbi.GetLocationByMap(ipnet, location.MapID, r.context)
	if err != nil {
		return nil, err
	}
	if locID != nil {
		location.LocID = make([]byte, len(locID))
		copy(location.LocID, locID)
		location.Mask = mask
	}
	return &location, nil
}

// ResolverLocation find the location associated with a client IP (resolver)
func (r *DataReader) ResolverLocation(q []byte, ip string) (*Location, error) {
	resolverIP := net.ParseIP(ip)
	bits := 8 * net.IPv6len
	prefixlen := 128

	if resolverIP.To4() != nil {
		prefixlen = 8 * net.IPv4len
		bits = 8 * net.IPv4len
	}

	mask := net.CIDRMask(prefixlen, bits)
	ipnet := net.IPNet{IP: resolverIP, Mask: mask}
	return r.findLocation(q, []byte{0, 'M'}, &ipnet)
}

// EcsLocation find a Location ID that matches this Client Subnet.
// If we do not find a match, Location will be nil and ECS's SourceScope will
// be set to 0.
// If we find a match, Location will contain the matching LocationID and ECS
// option will have SourceScope set.
func (r *DataReader) EcsLocation(q []byte, ecs *dns.EDNS0_SUBNET) (*Location, error) {
	bits := 8 * net.IPv4len
	if ecs.Family == 2 {
		bits = 8 * net.IPv6len
	}
	mask := net.CIDRMask(int(ecs.SourceNetmask), bits)
	ipnet := net.IPNet{IP: ecs.Address, Mask: mask}

	loc, err := r.findLocation(q, []byte{0, '8'}, &ipnet)
	if err != nil {
		return nil, err
	}
	// There is no mapping for this qname
	if loc.MapID.IsZero() {
		return nil, nil
	}
	// We found a match
	if !loc.LocID.IsZero() {
		ecs.SourceScope = loc.Mask
		if ecs.Family == 1 {
			ecs.SourceScope -= 96
		}
	} else {
		// Set default scope
		ecs.SourceScope = 24
		if ecs.Family == 2 {
			ecs.SourceScope = 48
		}
		return nil, nil
	}

	return loc, nil
}
