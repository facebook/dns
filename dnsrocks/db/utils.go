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
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// HasRecord goes over existing records in all sections and checks wether or not
// it exists in the message payload.
func HasRecord(msg *dns.Msg, record string, qtype uint16) bool {
	for _, a := range msg.Answer {
		if a.Header().Rrtype == qtype && a.Header().Name == record {
			return true
		}
	}
	for _, a := range msg.Ns {
		if a.Header().Rrtype == qtype && a.Header().Name == record {
			return true
		}
	}
	for _, a := range msg.Extra {
		if a.Header().Rrtype == qtype && a.Header().Name == record {
			return true
		}
	}
	return false
}

// AdditionalSectionForRecords given a list of records and a reader, add the
// required records to the Extra (additional) section.
// returns wether or not a weighted record was added to the additional section.
func AdditionalSectionForRecords(r Reader, a *dns.Msg, loc *Location, qclass uint16, records []dns.RR) (weighted bool) {
	var offset int
	var err error
	for _, x := range records {
		// TODO (jinyuan): according to unit tests, additional section record number
		// is 1 for each. Change to a better way to code it in the future
		var wrs = Wrs{MaxAnswers: 1}
		var packedName = make([]byte, 255)
		name := ""
		switch x.Header().Rrtype {
		case dns.TypeNS:
			name = x.(*dns.NS).Ns
		case dns.TypeMX:
			name = x.(*dns.MX).Mx
		case dns.TypeHTTPS:
			name = x.(*dns.HTTPS).Hdr.Name
		}
		if name == "" {
			continue
		}
		want4 := !HasRecord(a, name, dns.TypeA)
		want6 := !HasRecord(a, name, dns.TypeAAAA)

		if want4 || want6 {
			if offset, err = dns.PackDomainName(name, packedName, 0, nil, false); err != nil {
				glog.Errorf("Failed at packing domain name %s %v", name, err)
				continue
			}

			parseRecord := func(result []byte) error {
				if rr, err := ExtractRRFromRow(result, false); err == nil {
					// rr is nill when not matching the correct location
					if (rr.Qtype == dns.TypeA && want4) || (rr.Qtype == dns.TypeAAAA && want6) {
						err := wrs.Add(rr, result)
						return err
					}
				}
				return nil
			}

			err = r.ForEachResourceRecord(packedName[:offset], loc, parseRecord)
			if err != nil {
				glog.Errorf("Failed at parse records %v", err)
			}

			if rr, err := wrs.AAAARecord(name, qclass); err == nil && rr != nil {
				a.Extra = append(a.Extra, rr...)
			}
			if rr, err := wrs.ARecord(name, qclass); err == nil && rr != nil {
				a.Extra = append(a.Extra, rr...)
			}
		}
		if wrs.WeightedAnswer() {
			weighted = true
		}
	}
	return weighted
}
