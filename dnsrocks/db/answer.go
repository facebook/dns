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
	"math"

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
)

// recordProcessor holds the parameters for parseResult. It enables us to
// deduplicate code for parseResult without changing any function signatures.
type recordProcessor struct {
	msg         *dns.Msg
	seenError   bool
	wrs         Wrs
	recordFound bool
	wildcard    bool
	qname       string
	qtype       uint16
}

func (rp *recordProcessor) parseResult(result []byte) error {
	var (
		// resource record pointer used during record lookups
		rec ResourceRecord
		// rr will be used to construct temporary ResourceRecords
		rr  dns.RR
		err error
	)

	if rec, err = ExtractRRFromRow(result, rp.wildcard); err != nil {
		if errors.Is(err, ErrWildcardMismatch) {
			// ignore wildcard mismatch errors to stay consistent with existing system behavior
			// nolint: nilerr
			return nil
		}
		glog.Errorf("Failed to extract rr from row: %v", err)
		return err
	}
	rp.recordFound = true
	if rec.Qtype == dns.TypeCNAME || rec.Qtype == rp.qtype || rp.qtype == dns.TypeANY {
		// When dealing with A/AAAA we may have weighted round-robin records
		// Compute the weight and update wrr4/wrr6 with the current winner.
		// When we are done looping, we will add the record to the answer.
		if rec.Qtype == dns.TypeA || rec.Qtype == dns.TypeAAAA {
			if err = rp.wrs.Add(rec, result); err != nil {
				rp.seenError = true
				glog.Errorf("Failed in adding record to WRS: %v", err)
			}
			// For other records, we append them to the answer.
		} else {
			rdlength := len(result[rec.Offset:])
			if rdlength > math.MaxUint16 {
				err = errors.New("integer overflow for uint16 RR_Header.Rdlength")
				glog.Errorf("Failed to create RR_Header: %v, max value is %d, tried assigning %d", err, math.MaxUint16, rdlength)
				return err
			}
			hdr := dns.RR_Header{Name: rp.qname, Rrtype: rec.Qtype, Class: dns.ClassINET, Ttl: rec.TTL, Rdlength: uint16(rdlength)}
			rr, _, err = dns.UnpackRRWithHeader(hdr, result, rec.Offset)
			if err != nil {
				glog.Errorf("Failed to create resource record %v %d, %d, qname: %s", err, hdr.Rdlength, len(result[rec.Offset:]), rp.qname)
				return err
			}
			rp.msg.Answer = append(rp.msg.Answer, rr)
		}
	}
	return nil
}

func (rp *recordProcessor) responseCode() int {
	// If any records are returned, we suppress most errors. This ensures that we don't
	// drop a potentially useful response. However, if no records are returned and an error
	// occurred, we return SERVFAIL. This increases the chances that clients can recover
	// from localized errors by retrying to a working instance.
	if len(rp.msg.Answer) == 0 && rp.seenError {
		return dns.RcodeServerFailure
	}

	if !rp.recordFound {
		return dns.RcodeNameError
	}

	return dns.RcodeSuccess
}

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
// qtype (2) ch (1) recordloc (2+)? ttl (4) ttd (8) weight (4)? rdata (vlen)
// recordloc is present only if ch == '+' + 1 or ch == '*' + 1
// weight is only present if qtype is A or AAAA
// It returns a pointer to a ResourceRecord, nil when not a proper match.
// nil is returned when the row is not matching the specific filters (e.g
// Location or wildcard).
func ExtractRRFromRow(row []byte, wildcard bool) (rr ResourceRecord, err error) {
	r := bytes.NewReader(row)
	if err = binary.Read(r, binary.BigEndian, &rr.Qtype); err != nil {
		return
	}
	ch, err := r.ReadByte()
	if err != nil {
		return
	}

	// Only handle wildcard records when in wildcard mode.
	if wildcard != (ch == '*' || ch == '*'+1) {
		return rr, ErrWildcardMismatch
	}

	// If location based record, skip location ID.
	if (ch == '='+1) || (ch == '*'+1) {
		firstLocByte, err := r.ReadByte()
		if err != nil {
			return rr, err
		}
		secondLocByte, err := r.ReadByte()
		if err != nil {
			return rr, err
		}
		if firstLocByte == 0xff {
			// Long ID.  Skip the remainder.
			if _, err = r.Seek(int64(secondLocByte), io.SeekCurrent); err != nil {
				return rr, err
			}
		}
	}

	if err = binary.Read(r, binary.BigEndian, &rr.TTL); err != nil {
		return
	}

	// the next 8 bytes contains `ttd` TAI timestamp, which we do not use... skip.
	if _, err = r.Seek(8, io.SeekCurrent); err != nil {
		return
	}
	// Only A and AAAA records have a weight.
	if rr.Qtype == dns.TypeAAAA || rr.Qtype == dns.TypeA {
		if err = binary.Read(r, binary.BigEndian, &rr.Weight); err != nil {
			return
		}
	}
	rr.Offset = int(r.Size()) - r.Len()

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
func (r *DataReader) IsAuthoritative(q []byte, locID ID) (ns bool, auth bool, zoneCut []byte, err error) {
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

	key := make([]byte, len(locID)+len(q))

	for {
		if !locID.IsZero() {
			key = append(key[:0], locID...)
			key = append(key, zoneCut...)
			err := r.ForEach(key, parseResult)
			if err != nil {
				return false, false, zoneCut, err
			}
		}

		if !(auth && ns) {
			key = append(key[:0], ZeroID...)
			key = append(key, zoneCut...)
			err := r.ForEach(key, parseResult)
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
func (r *DataReader) FindAnswer(q []byte, packedControlName []byte, qname string, qtype uint16, locID ID, a *dns.Msg, maxAnswer int) (bool, int) {
	var (
		rrs []dns.RR
		err error
		key = make([]byte, len(q)+len(locID))
		rp  = &recordProcessor{
			msg:   a,
			wrs:   Wrs{MaxAnswers: maxAnswer},
			qname: qname,
			qtype: qtype,
		}
	)

	for {
		// Add location prefix to qname
		if !locID.IsZero() {
			key = append(key[:0], locID...)
			key = append(key[:len(locID)], q...)
			err = r.ForEach(key, rp.parseResult)
			if err != nil {
				rp.seenError = true
				glog.Errorf("%v", err)
			}
		}

		key = append(key[:0], ZeroID...)
		key = append(key, q...)
		err = r.ForEach(key, rp.parseResult)
		if err != nil {
			rp.seenError = true
			glog.Errorf("%v", err)
		}

		// append A/AAAA records with the selected RR record
		if rrs, err = rp.wrs.ARecord(qname, dns.ClassINET); err != nil {
			rp.seenError = true
			glog.Errorf("%v", err)
		} else {
			a.Answer = append(a.Answer, rrs...)
		}
		if rrs, err = rp.wrs.AAAARecord(qname, dns.ClassINET); err != nil {
			rp.seenError = true
			glog.Errorf("%v", err)
		} else {
			a.Answer = append(a.Answer, rrs...)
		}

		// recordFound is modified by recordProcessor's parseResult
		if rp.recordFound {
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
		rp.wildcard = true
	}

	return rp.wrs.WeightedAnswer(), rp.responseCode()
}

// FindSOA find SOA record and set it into the Authority section of the message.
func FindSOA(r Reader, zoneCut []byte, zoneCutString string, locID ID, a *dns.Msg) {
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

	err := r.ForEachResourceRecord(zoneCut, locID, parseResult)
	if err != nil {
		glog.Errorf("%v", err)
	}
}
