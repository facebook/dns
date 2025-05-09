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
	"github.com/miekg/dns"
)

// GetNs will find and return the authoritative NS for
// a specific domain `q`. `q` is in wire-format.
// This is not expecting that `q` is the actual qname which has NS records.
func GetNs(r Reader, q []byte, zonename string, class uint16, locID ID) (ns []dns.RR, err error) {
	var rr dns.RR

	parseResult := func(result []byte) error {
		if rec, err := ExtractRRFromRow(result, false); err == nil {
			// rec is nil if not matching location

			if rec.Qtype == dns.TypeNS {
				name, _, err := dns.UnpackDomainName(result, rec.Offset)

				if err != nil {
					return err
				}
				rr = new(dns.NS)
				rr.(*dns.NS).Hdr = dns.RR_Header{Name: zonename, Rrtype: dns.TypeNS, Class: class, Ttl: rec.TTL}
				rr.(*dns.NS).Ns = name
				ns = append(ns, rr)
			}
		}
		return nil
	}

	err = r.ForEachResourceRecord(q, locID, parseResult)
	if err != nil {
		return nil, err
	}

	return ns, nil
}
