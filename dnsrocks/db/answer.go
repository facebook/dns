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
	"encoding/binary"
	"errors"
	"io"

	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// ResourceRecord holds the representation of a row from DB
type ResourceRecord struct {
	Weight uint32
	Qtype  uint16
	TTL    uint32
	Offset int
}

var (
	// ErrLocationMismatch is returned if the record is not matching the location
	ErrLocationMismatch = errors.New("Location mismatch")
	// ErrWildcardMismatch is returned if the record is a wildcard but we are not
	// looking for wildcard
	ErrWildcardMismatch = errors.New("Wildcard mismatch")

	// EmptyLocation is used to index (prefix) non-location aware qnames for RR's
	EmptyLocation = Location{}
)

// dnsLabelWildsafe checks that a label contains only characters that can be
// used in a wildcard record match.
// WARNING: Characters are expected to be lower case!
func dnsLabelWildsafe(q []byte) bool {
	for _, c := range q {
		if c >= 'a' && c <= 'z' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// ExtractRRFromRow extracts a ResourceRecord from a DB row.
// A DB row contains data ResourceRecord information:
// qtype (2) ch (1) recordloc (2)? ttl (4) ttd (8) weight (4)? rdata (vlen)
// recordloc is present only if ch == '+' + 1 or ch == '*' + 1
// weight is only present if qtype is A or AAAA
// It returns a pointer to a ResourceRecord, nil when not a proper match.
// nil is returned when the row is not matching the specific filters (e.g
// Location or wildcard).
func ExtractRRFromRow(row []byte, wildcard bool) (rr ResourceRecord, err error) {
	dpos := 0
	rr.Qtype = binary.BigEndian.Uint16(row[dpos : dpos+2])
	dpos += 2
	ch := row[dpos]
	dpos++

	// Only handle wildcard records when in wildcard mode.
	if wildcard != (ch == '*' || ch == '*'+1) {
		return rr, ErrWildcardMismatch
	}

	// If location based record, skip location offset.
	if (ch == '='+1) || (ch == '*'+1) {
		dpos += 2
	}

	rr.TTL = binary.BigEndian.Uint32(row[dpos : dpos+4])
	dpos += 4

	// the next 8 bytes contains `ttd` TAI timestamp, which we do not use... skip.
	dpos += 8

	rr.Offset = dpos
	// Only A and AAAA records have a weight.
	if rr.Qtype == dns.TypeAAAA || rr.Qtype == dns.TypeA {
		rr.Weight = binary.BigEndian.Uint32(row[dpos : dpos+4])
		rr.Offset += 4
	}

	return rr, nil
}

// IsAuthoritative find whether or not we are authoritative and have NS
// records for the given domain. Starting from the original qname, it
// iterates through every possible parent domain by removing 1 label at a time
// until it find a match, or there is no more labels.
// It returns a boolean if we have NS records, and if we are authoritative.
// And the packed zone cut for which we found NS/Auth.
//
// If `ns` is True and `auth` is False: this is a delegation.
// If `ns` and `auth` are True, we are authoritative.
func (r *DataReader) IsAuthoritative(q []byte, loc *Location) (ns bool, auth bool, zoneCut []byte, err error) {
	zoneCut = q

	parseResult := func(result []byte) error {
		rec, err := ExtractRRFromRow(result, false)
		if err != nil {
			// nolint: nilerr
			return nil
		}

		switch rec.Qtype {
		case dns.TypeSOA:
			auth = true
		case dns.TypeNS:
			ns = true
		}
		return nil
	}

	for {
		if loc.LocID != EmptyLocation.LocID {
			localQ := append(loc.LocID[:], zoneCut...)
			err := r.ForEach(localQ, parseResult)
			if err != nil {
				return false, false, zoneCut, err
			}
		}

		if !(auth && ns) {
			nonLocalQ := append(EmptyLocation.LocID[:], zoneCut...)
			err := r.ForEach(nonLocalQ, parseResult)
			if err != nil {
				return false, false, zoneCut, err
			}
		}
		if err != nil {
			return
		}
		// We found NS records. If we have a matching SOA, we are authoritative,
		// otherwise, this is a delegation.
		if ns {
			break
		}
		if zoneCut[0] == 0 {
			break
		}
		zoneCut = zoneCut[1+zoneCut[0]:]
	}
	return
}

// FindAnswer will find answers for a given query q
func (r *DataReader) FindAnswer(q []byte, packedControlName []byte, qname string, qtype uint16, loc *Location, a *dns.Msg, maxAnswer int) (bool, bool) {
	var (
		wrs         = Wrs{MaxAnswers: maxAnswer}
		err         error
		recordFound = false
		wildcard    = false
		// resource record pointer used during record lookups
		rec ResourceRecord
		// rr will be used to construct temporary ResourceRecords
		rr  dns.RR
		rrs []dns.RR
	)

	parseResult := func(result []byte) error {
		if errors.Is(err, io.EOF) {
			return nil
		}

		if rec, err = ExtractRRFromRow(result, wildcard); err != nil {
			// Not a location match
			// nolint:nilerr
			return nil
		}
		recordFound = true
		if rec.Qtype == dns.TypeCNAME || rec.Qtype == qtype || qtype == dns.TypeANY {
			// When dealing with A/AAAA we may have weighted round-robin records
			// Compute the weight and update wrr4/wrr6 with the current winner.
			// When we are done looping, we will add the record to the answer.
			if rec.Qtype == dns.TypeA || rec.Qtype == dns.TypeAAAA {
				if err := wrs.Add(rec, result); err != nil {
					glog.Errorf("Failed in adding record to WRS: %v", err)
				}
				// For other records, we append them to the answer.
			} else {
				hdr := dns.RR_Header{Name: qname, Rrtype: rec.Qtype, Class: dns.ClassINET, Ttl: rec.TTL, Rdlength: uint16(len(result[rec.Offset:]))}
				rr, _, err = dns.UnpackRRWithHeader(hdr, result, rec.Offset)
				if err != nil {
					glog.Errorf("Failed to convert from tinydns format %v %d, %d", err, hdr.Rdlength, len(result[rec.Offset:]))
					return err
				}
				a.Answer = append(a.Answer, rr)
			}
		}
		return nil
	}

	for {
		// Add location prefix to qname
		if loc.LocID != EmptyLocation.LocID {
			localQ := append(loc.LocID[:], q[:]...)
			err = r.ForEach(localQ, parseResult)
			if err != nil {
				glog.Errorf("%v", err)
			}
		}

		nonLocalQ := append(EmptyLocation.LocID[:], q[:]...)
		err = r.ForEach(nonLocalQ, parseResult)
		if err != nil {
			glog.Errorf("%v", err)
		}

		// append A/AAAA records with the selected RR record
		if rrs, err = wrs.ARecord(qname, dns.ClassINET); err != nil {
			glog.Errorf("%v", err)
		} else {
			a.Answer = append(a.Answer, rrs...)
		}
		if rrs, err = wrs.AAAARecord(qname, dns.ClassINET); err != nil {
			glog.Errorf("%v", err)
		} else {
			a.Answer = append(a.Answer, rrs...)
		}

		if recordFound {
			break
		}
		if bytes.Equal(q, packedControlName) {
			break
		}
		if q[0] == 0 {
			break
		}
		if !dnsLabelWildsafe(q[1 : q[0]+1]) {
			break
		}
		q = q[q[0]+1:]
		wildcard = true
	}
	return wrs.WeightedAnswer(), recordFound
}

// FindSOA find SOA record and set it into the Authority section of the message.
func FindSOA(r Reader, zoneCut []byte, zoneCutString string, loc *Location, a *dns.Msg) {
	var (
		// rr will be used to construct temporary ResourceRecords
		rr dns.RR
	)
	soa := false
	parseResult := func(result []byte) error {
		if rec, err := ExtractRRFromRow(result, false); err == nil {
			// rec is nil if not matching location
			if !soa && rec.Qtype == dns.TypeSOA {
				soa = true
				hdr := dns.RR_Header{Name: zoneCutString, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: rec.TTL, Rdlength: uint16(len(result[rec.Offset:]))}
				rr, _, err = dns.UnpackRRWithHeader(hdr, result, rec.Offset)
				if err != nil {
					glog.Errorf("%v", err)
				}
				a.Ns = append(a.Ns, rr)
			}
		}
		return nil
	}

	err := r.ForEachResourceRecord(zoneCut, loc, parseResult)
	if err != nil {
		glog.Errorf("%v", err)
	}
}
