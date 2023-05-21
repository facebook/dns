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

import "net"

// IPv6 is a storage-efficient replacement for net.IP struct
// net.IP stores IPv6/IPv4 addresses as slices which is really inefficient
// on 64-bit architecture:
// * IPv6 takes 40 bytes: 24 bytes slice header + 16 bytes data
// * IPv4 takes 28 bytes: 24 bytes slice header + 4 bytes data
// on top of it slice has a reference so it is more expensive for GC handling
// so this class address that storage inefficiency though delegating a lot of
// functionality to net.IP class
type IPv6 [16]byte

// String gets string representation of IP
func (ip IPv6) String() string {
	return net.IP(ip[:]).String()
}

// ParseIP parses string IP representation to the byte structure
func ParseIP(s string) IPv6 {
	var r IPv6
	copy(r[:], []byte(net.ParseIP(s).To16()))
	return r
}

// Equal checks equality with another IPv6 object
func (ip IPv6) Equal(other IPv6) bool {
	for i := 0; i < len(ip); i++ {
		if ip[i] != other[i] {
			return false
		}
	}
	return true
}

// EqualToNetIP checks equality to net.IP object
func (ip IPv6) EqualToNetIP(other net.IP) bool {
	t := other.To16()

	for i := 0; i < len(ip); i++ {
		if ip[i] != t[i] {
			return false
		}
	}
	return true
}

// FromNetIP performs conversion from net.IP structure to more storage efficient IPv6
func FromNetIP(other net.IP) IPv6 {
	result := IPv6{}

	copy(result[:], []byte(other.To16()))

	return result
}

// MarshalText implements encoding.TextMarshaler
func (ip IPv6) MarshalText() ([]byte, error) {
	return net.IP(ip[:]).MarshalText()
}

// To4 attempts to extract IPv4 if it possible
// if not - returns nil
func (ip IPv6) To4() net.IP {
	return net.IP(ip[:]).To4()
}
