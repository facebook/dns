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

package dnsdata

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

type rangePointKind uint8

const (
	pointKindStart rangePointKind = iota
	pointKindEnd
)

var (
	firstIPv6  = ParseIP("0000:0000:0000:0000:0000:0000:0000:0000")
	firstIPv4  = ParseIP("0000:0000:0000:0000:0000:ffff:0000:0000") // the very first IPv4 address according to RFC-2765
	afterIPv4  = ParseIP("0000:0000:0000:0000:0001:0000:0000:0000") // the very first address after IPv4 addresses according to RFC-2765
	veryLastIP = ParseIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
)

type rangeLocation struct {
	// it might look illogical that the maskLen is associated with the location and not RangePoint, however it is correct:
	// this mask is the length of the matched prefix
	maskLen     uint8
	locIDIsNull bool
	locID       [2]byte
}

// RangePoint stores range point along with associated location ID
type RangePoint struct {
	rangeStart IPv6 // the first IP address of this range in IPv6 form
	location   rangeLocation
	pointKind  rangePointKind
}

// RangePoints is an array of RangePoint, the only reason for it to exist is the String() method
type RangePoints []*RangePoint

// Rearranger translates IP ranges with associated locations to "Start IP -> LocID" ranges
type Rearranger struct {
	hasDefaultIPv4Range bool
	hasDefaultIPv6Range bool
	points              RangePoints
}

// String returns a string representation of these RangePoints, useful for debugging
func (r RangePoints) String() string {
	result := make([]string, len(r))
	for i, p := range r {
		result[i] = fmt.Sprintf("%d) %s", i, p.String())
	}
	return strings.Join(result, "\n")
}

// To16 returns 16-byte IP representation of the rangeStart
func (p *RangePoint) To16() IPv6 {
	return p.rangeStart
}

func lpad(a []byte, size int) []byte {
	if len(a) >= size {
		return a
	}
	b := make([]byte, size)
	copy(b[size-len(a):], a)
	return b
}

// MaskLen returns the length of the matched mask
func (p *RangePoint) MaskLen() uint8 {
	return p.location.maskLen
}

// LocIsNull checks if the rangeLocation is a null value (no location defined)
func (p *RangePoint) LocIsNull() bool {
	return p.location.locIDIsNull
}

// LocID returns the location ID for this range
func (p *RangePoint) LocID() []byte {
	return p.location.locID[:]
}

// String returns a string representation of this location, used for debugging
func (p *RangePoint) String() string {
	var kind string
	if p.pointKind == pointKindStart {
		kind = ">"
	} else {
		kind = "<"
	}

	var loc string
	if p.LocIsNull() {
		loc = "null"
	} else {
		loc = fmt.Sprintf("%d", p.LocID())
	}

	return fmt.Sprintf(
		"%s %s/%d at %s",
		kind, p.To16().String(), p.MaskLen(), loc,
	)
}

// NewRearranger creates an instance of Rearranger for estimated locationCount
func NewRearranger(locationCount int) *Rearranger {
	return &Rearranger{
		points: make([]*RangePoint, 0, locationCount*2),
	}
}

// ErrInvalidLocation is used when location doesn't match expectations (nil or exactly 2 bytes)
var ErrInvalidLocation = errors.New("location should be either nil or exactly 2 bytes long as FBDNS depends on it")

func copyLocID(locID []byte) ([2]byte, error) {
	var x [2]byte

	if locID == nil || len(locID) != 2 {
		return x, fmt.Errorf("%w. location value '%v'", ErrInvalidLocation, locID)
	}
	copy(x[:], locID)

	return x, nil
}

// AddLocation converts the location into a pair of RangePoints, where:
// * the starting RangePoint is the first IP of the Location, and we immediately know the LocID for this RangePoint
// * the next RangePoint is the first IP _after_ the end of this Location, so it is marked as rangePointEnd, and the LocID is to be determined
func (r *Rearranger) AddLocation(ipnet *net.IPNet, locID []byte) error {
	maskLen, _ := ipnet.Mask.Size()
	copiedLocID, err := copyLocID(locID)
	if err != nil {
		return err
	}

	if firstIPv6.EqualToNetIP(ipnet.IP.To16()) {
		// it is ::/0
		r.hasDefaultIPv6Range = true
		defaultIPv6Location := rangeLocation{
			maskLen: uint8(maskLen),
			locID:   copiedLocID,
		}
		r.points = append(r.points, &RangePoint{
			rangeStart: firstIPv6,
			pointKind:  pointKindStart,
			location:   defaultIPv6Location,
		})
		// add pseudo points for IPv4
		r.points = append(r.points, &RangePoint{
			rangeStart: afterIPv4,
			pointKind:  pointKindStart,
			location:   defaultIPv6Location,
		})
	} else if firstIPv4.EqualToNetIP(ipnet.IP.To16()) {
		// it is 0.0.0.0/0
		r.hasDefaultIPv4Range = true
		r.points = append(r.points, &RangePoint{
			rangeStart: firstIPv4,
			pointKind:  pointKindStart,
			location: rangeLocation{
				maskLen: uint8(maskLen),
				locID:   copiedLocID,
			},
		})
		r.points = append(r.points, &RangePoint{
			rangeStart: afterIPv4,
			pointKind:  pointKindEnd,
			location: rangeLocation{
				maskLen: uint8(maskLen),
				locID:   copiedLocID,
			},
		})
	} else {
		// it is not a default range
		startIP := ipCleanMask(&ipnet.IP, &ipnet.Mask)
		r.points = append(r.points, &RangePoint{
			rangeStart: startIP,
			pointKind:  pointKindStart,
			location: rangeLocation{
				maskLen: uint8(maskLen),
				locID:   copiedLocID,
			},
		})

		// add the range point that is right after the end of this location
		lastIP := ipFillUnmasked(&ipnet.IP, &ipnet.Mask) // set lastIP to the last IP of the range

		// IP+1 does not exist, skip
		if !veryLastIP.Equal(lastIP) {
			// and increment by 1, which gets to the first IP address _after_ the end of this location
			nextRangeStart := ipIncrementByOne(lastIP)

			r.points = append(r.points, &RangePoint{
				rangeStart: nextRangeStart,
				pointKind:  pointKindEnd,
				location: rangeLocation{
					// locID of the nextRangePoint is not known yet
					maskLen:     uint8(maskLen),
					locIDIsNull: true,
				},
			})
		}
	}

	return nil
}

// incrementByOne increments ip by one, if it is last ip then it will overflow to 0::0
func ipIncrementByOne(x IPv6) IPv6 {
	for i := len(x) - 1; i >= 0; i-- {
		if x[i] == 255 {
			x[i] = 0
		} else {
			x[i]++
			break
		}
	}

	return x
}

// ipCleanMask returns a copy of ipaddr with unmasked bits set to 0,
// it is a form of sanitizing IP address
func ipCleanMask(ipaddr *net.IP, mask *net.IPMask) IPv6 {
	result := IPv6{}

	copy(result[:], []byte(ipaddr.To16()))

	offset := 0
	if len(*mask) == net.IPv4len {
		offset = net.IPv6len - net.IPv4len
	}

	for i, val := range *mask {
		result[i+offset] &= val
	}

	return result
}

// ipFillUnmasked returns a copy of ipaddr with unmasked bits set to 1,
// it allows to get the last address for this IP range
func ipFillUnmasked(ipaddr *net.IP, mask *net.IPMask) IPv6 {
	result := IPv6{}

	copy(result[:], []byte(ipaddr.To16()))

	offset := 0
	if len(*mask) == net.IPv4len {
		offset = net.IPv6len - net.IPv4len
	}

	for i, val := range *mask {
		result[offset+i] |= val ^ 0xFF
	}

	return result
}

// Rearrange returns a slice with RangePoints with resolved LocID for
// finish RangePoint. It also adds implicit "null" locations spanning
// all unmatched ranges (if necessary).
func (r *Rearranger) Rearrange() RangePoints {
	if len(r.points) == 0 {
		return nil
	}

	result := make(RangePoints, len(r.points), len(r.points)+4) // 4 = 2+1+1 values for default ranges
	copy(result, r.points)

	// If there is no location spanning all other locations - add the zero point, so it is a null value.
	// This has to be a real range point that will be saved in the database, so the location search works correctly.
	if !r.hasDefaultIPv4Range {
		// IPv4 range is in the middle of IPv6 (according to RFC-2765), which means that:
		// 1) IPv4 range has to have the start
		result = append(result, &RangePoint{
			rangeStart: firstIPv4,
			pointKind:  pointKindStart,
			location: rangeLocation{
				locIDIsNull: true,
			},
		})
		// 2) IPv4 range has to have the end
		result = append(result, &RangePoint{
			rangeStart: afterIPv4,
			pointKind:  pointKindEnd,
			location: rangeLocation{
				locIDIsNull: true,
			},
		})
	}
	if !r.hasDefaultIPv6Range {
		// default range for IPv6 starts with zero
		result = append(result, &RangePoint{
			rangeStart: firstIPv6,
			pointKind:  pointKindStart,
			location: rangeLocation{
				locIDIsNull: true,
			},
		})
		// and then starts again after IPv4 range
		result = append(result, &RangePoint{
			rangeStart: afterIPv4,
			pointKind:  pointKindStart,
			location: rangeLocation{
				locIDIsNull: true,
			},
		})
	}

	// sort by nest
	sort.Slice(result, func(i, j int) bool {
		cmp := bytes.Compare(result[i].rangeStart[:], result[j].rangeStart[:])
		if cmp != 0 {
			return cmp == -1
		}
		k1, k2 := result[i].pointKind, result[j].pointKind
		if k1 != k2 {
			// between pointKindStart and pointKindEnd: pointKindEnd goes first (it is less)
			return k1 == pointKindEnd
		}
		if k1 == pointKindStart {
			// for pointKindStart between pointKindStart and pointKindStart: shortest prefix first
			return result[i].location.maskLen < result[j].location.maskLen
		}
		// for pointKindEnd between pointKindEnd and pointKindEnd: longest prefix first
		return result[i].location.maskLen > result[j].location.maskLen
	})

	locationStack := make([]rangeLocation, 0, 129) // normally 129 values from /0 to /128, but can be more if the same IP range was declared more than once
	stackTop := -1
	for _, point := range result {
		switch point.pointKind {
		case pointKindStart:
			// push the location
			stackTop++
			if stackTop == len(locationStack) {
				// extend stack
				locationStack = append(locationStack, point.location)
			} else {
				locationStack[stackTop] = point.location
			}
		case pointKindEnd:
			stackTop--                               // pop
			point.location = locationStack[stackTop] // location comes from the range that spans this range point
		}
	}

	// Squash IP duplicates: if two consecutive range points have the same IP and MaskLen, then the latter wins.
	// This can happen if there were two locations back-to-back, then the finish of the first location will be the start of the second
	// If it is the same IP, but the latter MaskLen is shorter (and the former is more exact), then the latter wins.
	// NOTE: we should not squash points with the same location but different MaskLen, because otherwise we risk capturing matches with shorter masks.
	squashedIP := make(RangePoints, 1, len(result))
	squashedIP[0] = result[0]
	for i := 1; i < len(result); i++ {
		prevPoint, thisPoint := squashedIP[len(squashedIP)-1], result[i]
		if prevPoint.rangeStart.Equal(thisPoint.rangeStart) && prevPoint.MaskLen() >= thisPoint.MaskLen() {
			// duplicate IP, squash
			squashedIP[len(squashedIP)-1] = thisPoint
		} else {
			// new value, add
			squashedIP = append(squashedIP, thisPoint)
		}
	}

	return squashedIP
}
