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
	"fmt"
	"math"
	"net"

	"github.com/miekg/dns"
)

// WrsItem Weighted Random Sample Item
type WrsItem struct {
	Key  float64
	TTL  uint32
	Addr net.IP
}

// Wrs Weighted Random Sample
type Wrs struct {
	MaxAnswers int
	V4         []WrsItem
	V4Count    uint32
	V6         []WrsItem
	V6Count    uint32
}

/*
By getting this package its own source, we can seed it without stepping on the
global source which may be initialize in some other way.
Also, any application using this package, will get the seeding for free rather
than having to do it themselves.
*/
var localRand = NewRand()

// Add adds a ResourceRecord to Wrs if its randomly computed weight is greater
// then the existing record.
func (w *Wrs) Add(rec ResourceRecord, data []byte) error {
	if rec.Qtype != dns.TypeA && rec.Qtype != dns.TypeAAAA {
		return fmt.Errorf("Unsupported type %d", rec.Qtype)
	}

	key := math.Pow(float64(localRand.Uint32())*float64(1.0/math.MaxUint32), 1.0/float64(rec.Weight))
	wrsItem := WrsItem{Key: key,
		TTL:  rec.TTL,
		Addr: data[rec.Offset:]}
	addRecord := func(items []WrsItem) []WrsItem {
		if len(items) < w.MaxAnswers {
			items = append(items, wrsItem)
		} else {
			minKey := key
			idx := -1
			for i, v := range items {
				if v.Key < minKey {
					minKey = v.Key
					idx = i
				}
			}
			if idx != -1 {
				items[idx] = wrsItem
			}
		}
		return items
	}
	checkAndReplaceRecord := func(items []WrsItem) []WrsItem {
		if len(items) == 0 {
			items = append(items, wrsItem)
		} else if wrsItem.Key > items[0].Key {
			items[0] = wrsItem
		}
		return items
	}

	if rec.Qtype == dns.TypeA {
		w.V4Count++
		if w.MaxAnswers == 1 {
			w.V4 = checkAndReplaceRecord(w.V4)
		} else {
			w.V4 = addRecord(w.V4)
		}
	}
	if rec.Qtype == dns.TypeAAAA {
		w.V6Count++
		if w.MaxAnswers == 1 {
			w.V6 = checkAndReplaceRecord(w.V6)
		} else {
			w.V6 = addRecord(w.V6)
		}
	}
	return nil
}

func (w *Wrs) record(name string, class uint16, qtype uint16) (rrs []dns.RR, err error) {
	var items []WrsItem
	switch qtype {
	case dns.TypeA:
		items = w.V4
	case dns.TypeAAAA:
		items = w.V6
	default:
		return nil, fmt.Errorf("Unsupported type %d", qtype)
	}
	localRand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})

	for _, item := range items {
		if item.Key > 0.0 {
			hdr := dns.RR_Header{Name: name, Rrtype: qtype, Class: class, Ttl: item.TTL, Rdlength: uint16(len(item.Addr))}
			var rr dns.RR
			rr, _, err = dns.UnpackRRWithHeader(hdr, item.Addr, 0)
			if err != nil {
				return nil, fmt.Errorf("failed to convert from tinydns format %d, %d: %w", hdr.Rdlength, len(item.Addr), err)
			}
			rrs = append(rrs, rr)
		}
	}

	return rrs, nil
}

// ARecord returns the weighted random sample for A qtype if there is any.
func (w *Wrs) ARecord(name string, class uint16) (rrs []dns.RR, err error) {
	return w.record(name, class, dns.TypeA)
}

// AAAARecord returns the weighted random sample for AAAA qtype if there is any.
func (w *Wrs) AAAARecord(name string, class uint16) (rrs []dns.RR, err error) {
	return w.record(name, class, dns.TypeAAAA)
}

// WeightedAnswer returns true if the answer selected 1 out of many results.
func (w *Wrs) WeightedAnswer() bool {
	return w.V4Count > 1 || w.V6Count > 1
}
