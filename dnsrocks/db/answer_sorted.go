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
	"errors"
	"io"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"

	"github.com/golang/glog"
	"github.com/miekg/dns"
)

func (r *sortedDataReader) FindAnswer(q []byte, packedControlName []byte, qname string, qtype uint16, loc *Location, a *dns.Msg, maxAnswer int) (bool, bool) {
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
			// nolint: nilerr
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

	var lastLength = len(q)

	preIterationCheck := func(q []byte, length int) bool {
		// we iterate on labels of initial qName, so length check should be sufficient to determine
		// whether we passed zone border
		if length < len(packedControlName) {
			return false
		}
		// we can also check if reversed control name is part of new q
		// (which can be any key less than what we look for),
		// but that will require another allocation to actually store this reversed control name,
		// plus `bytes.Equal` call

		// i points to the first character of the label
		// i-1 is length of the label
		i := length

		for i < lastLength {
			labelLength := int(q[i-1])
			label := q[i : i+labelLength]

			if !dnsLabelWildsafe(label) {
				return false
			}

			i += labelLength + 1
		}

		lastLength = length

		return true
	}

	postIterationCheck := func() bool {
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
			return false
		}

		wildcard = true

		return true
	}

	r.find(q, loc, parseResult, preIterationCheck, postIterationCheck)

	return wrs.WeightedAnswer(), recordFound
}

func (r *sortedDataReader) IsAuthoritative(q []byte, loc *Location) (ns bool, auth bool, zoneCut []byte, err error) {
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

	var zoneCutLength int

	preIterationCheck := func(q []byte, qLength int) bool {
		zoneCutLength = qLength

		return true
	}

	postIterationCheck := func() bool {
		return !ns
	}

	r.find(q, loc, parseResult, preIterationCheck, postIterationCheck)

	zoneCut = q[len(q)-zoneCutLength:]

	return
}

func (r *sortedDataReader) find(
	q []byte,
	loc *Location,
	parseResult func(value []byte) error,
	preIterationCheck func(qName []byte, currentLength int) bool,
	postIterationCheck func() bool,
) {
	var err error

	reversedQName := reverseZoneName(q)

	qLength := len(reversedQName)

	key := make([]byte, len(q)+len(loc.LocID)+len(dnsdata.ResourceRecordsKeyMarker))
	copy(key, []byte(dnsdata.ResourceRecordsKeyMarker))

	// assumption is that both provided location AND empty location have same length
	locationLength := len(loc.LocID)
	domainNameStart := len(dnsdata.ResourceRecordsKeyMarker)

	copy(key[domainNameStart:], reversedQName)

	for {
		// intentionally passing untouched reversed QName so client can examine length changes
		// for example for wildcard safety
		if !preIterationCheck(reversedQName, qLength) {
			break
		}

		locationStart := domainNameStart + qLength
		// mark domain name end. we can avoid re-copying as we are cutting labels from the end
		key[locationStart-1] = 0x00

		copy(key[locationStart:], loc.LocID[:])
		key = key[:locationStart+locationLength]

		// new key that is equal to what we asked for, or less than it.
		// This new key can be very different from what we requested, i.e.
		// for com.example.foo (if exact match is not found) previous key will be returned,
		// which doesn't guaranteed to even start with com.example, it can be com.example.foo for what we know
		var k []byte
		k, err = r.TryForEach(key, parseResult)

		if err != nil {
			break
		}

		// zone cut for different location exists -> check default location
		if loc.LocID != EmptyLocation.LocID &&
			// empty location override might exists as only location part is different in found key
			len(key) == len(k) &&
			bytes.Equal(
				key[:len(key)-locationLength], k[:len(key)-locationLength],
			) {
			copy(key[locationStart:], EmptyLocation.LocID[:])

			k, err = r.TryForEach(key, parseResult)
			if err != nil {
				break
			}
		}

		if !postIterationCheck() {
			break
		}

		if len(k) < len(dnsdata.ResourceRecordsKeyMarker) ||
			!bytes.Equal(k[:len(dnsdata.ResourceRecordsKeyMarker)], []byte(dnsdata.ResourceRecordsKeyMarker)) {
			// reached border of data
			break
		}

		// reached root zone
		if qLength == 1 {
			break
		}

		// strip out prefix and location
		foundLabel := k[domainNameStart : len(k)-locationLength]

		// ignore end length as it will be \0 for foundLabel and most probably non \0 for current name
		if bytes.Equal(reversedQName[:qLength-1], foundLabel[:len(foundLabel)-1]) {
			qLength = getLengthWithoutLastLabel(reversedQName, qLength)
		} else {
			qLength = findCommonLongestPrefix(reversedQName, foundLabel) + 1 // +1 for terminating \0
		}
	}
}

// cut out exactly one label from the end
func getLengthWithoutLastLabel(qName []byte, qLength int) int {
	var i byte

	lastLabelLengthIndex := 0
	for i < byte(qLength)-1 {
		lastLabelLengthIndex = int(i)
		i += qName[i] + 1
	}

	return lastLabelLengthIndex + 1
}

// TryForEach performs provided operation on each value in case exact key is found
// otherwise closest smaller key (previous) is returned
func (r *sortedDataReader) TryForEach(key []byte, f func(value []byte) error) (foundKey []byte, err error) {
	foundKey, err = r.closestKeyFinder.FindClosestKey(key, r.context)

	if err != nil {
		return nil, err
	}

	if bytes.Equal(key, foundKey) {
		err = r.ForEach(key, f)
	}

	return foundKey, err
}

// search for common labels at the beginning of 2 packed names
// returns length of the common part
func findCommonLongestPrefix(str1 []byte, str2 []byte) int {
	i := 0

	for i < len(str1) && i < len(str2) {
		if str1[i] != str2[i] {
			break
		}

		match := true

		for j := i + 1; j <= i+int(str1[i]); j++ {
			if str1[j] != str2[j] {
				match = false
				break
			}
		}

		if match {
			i = i + int(str1[i]) + 1
		} else {
			break
		}
	}

	return i
}

func reverseZoneName(qName []byte) []byte {
	reversedName := make([]byte, len(qName))

	reverseZoneNameToBuffer(qName, reversedName)
	return reversedName
}

func reverseZoneNameToBuffer(qName []byte, destination []byte) {
	i := byte(len(qName) - 1)

	destination[i] = 0

	for qName[0] > 0 {
		i -= qName[0]

		copy(destination[i:], qName[1:qName[0]+1])
		i--

		destination[i] = qName[0]

		qName = qName[qName[0]+1:]
	}
}
