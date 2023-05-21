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
	"fmt"
)

// WireRecord is the interface implemented by all record types which could be
// transferred on a wire directly (non-composite or specialized ones)
type WireRecord interface {
	WireType() WireType
	DomainName() string
	Location() Loc
	TTL() uint32
}

// DomainName returns associated record domain name
func (r *rshared) DomainName() string {
	if r.iswildcard {
		return fmt.Sprintf("*.%s", string(r.dom))
	}
	return string(r.dom)
}

// Location returns location for which record could be used
func (r *rshared) Location() Loc {
	return r.lo
}

// TTL returns TTL for the record
func (r *rshared) TTL() uint32 {
	return r.ttl
}

// WireType implements WireRecord interface
func (r *Raddr) WireType() WireType {
	if ip4 := r.ip.To4(); ip4 != nil {
		return TypeA
	}
	return TypeAAAA
}

// WireType implements WireRecord interface
func (r *Rns1) WireType() WireType {
	return TypeNS
}

// WireType implements WireRecord interface
func (r *Rcname) WireType() WireType {
	return TypeCNAME
}

// WireType implements WireRecord interface
func (r *Rsoa) WireType() WireType {
	return TypeSOA
}

// WireType implements WireRecord interface
func (r *Rptr) WireType() WireType {
	return TypePTR
}

// WireType implements WireRecord interface
func (r *Rmx1) WireType() WireType {
	return TypeMX
}

// WireType implements WireRecord interface
func (r *Rtxt) WireType() WireType {
	return TypeTXT
}

// WireType implements WireRecord interface
func (r *Rsrv1) WireType() WireType {
	return TypeSRV
}

// WireType implements WireRecord interface
func (r *Rhttps) WireType() WireType {
	return TypeHTTPS
}
