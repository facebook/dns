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

	"github.com/facebook/dns/dnsrocks/dnsdata"

	"github.com/golang/glog"
	"github.com/miekg/dns"
)

func (r *sortedDataReader) FindAnswer(q []byte, packedControlName []byte, qname string, qtype uint16, locID ID, a *dns.Msg, maxAnswer int) (bool, int) {
	var (
		rrs []dns.RR
		err error
		rp  = &recordProcessor{
			msg:   a,
			wrs:   Wrs{MaxAnswers: maxAnswer},
			qname: qname,
			qtype: qtype,
		}
	)

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

		if rp.recordFound {
			return false
		}

		rp.wildcard = true

		return true
	}

	err = r.find(q, locID, rp.parseResult, preIterationCheck, postIterationCheck)
	if err != nil {
		rp.seenError = true
	}

	return rp.wrs.WeightedAnswer(), rp.responseCode()
}

func (r *sortedDataReader) IsAuthoritative(q []byte, locID ID) (ns bool, auth bool, zoneCut []byte, err error) {
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

	preIterationCheck := func(_ []byte, qLength int) bool {
		zoneCutLength = qLength

		return true
	}

	postIterationCheck := func() bool {
		return !ns
	}

	_ = r.find(q, locID, parseResult, preIterationCheck, postIterationCheck)

	zoneCut = q[len(q)-zoneCutLength:]

	return
}

func (r *sortedDataReader) find(
	q []byte,
	locID ID,
	parseResult func(value []byte) error,
	preIterationCheck func(qName []byte, currentLength int) bool,
	postIterationCheck func() bool,
) error {
	var err error

	reversedQName := reverseZoneName(q)

	qLength := len(reversedQName)

	key := make([]byte, len(q)+max(len(locID), len(ZeroID))+len(dnsdata.ResourceRecordsKeyMarker))
	copy(key, []byte(dnsdata.ResourceRecordsKeyMarker))

	domainNameStart := len(dnsdata.ResourceRecordsKeyMarker)

	copy(key[domainNameStart:], reversedQName)

	// intentionally passing untouched reversed QName so client can examine length changes
	// for example for wildcard safety
	for preIterationCheck(reversedQName, qLength) {
		locationStart := domainNameStart + qLength
		// mark domain name end. we can avoid re-copying as we are cutting labels from the end
		key[locationStart-1] = 0x00

		key = append(key[:locationStart], locID...)

		// new key that is equal to what we asked for, or less than it.
		// This new key can be very different from what we requested, i.e.
		// for com.example.foo (if exact match is not found) previous key will be returned,
		// which doesn't guaranteed to even start with com.example, it can be com.examnle.foo for what we know
		var k []byte
		k, err = r.TryForEach(key, parseResult)
		if err != nil {
			break
		}

		// zone cut for different location exists -> check default location
		if !locID.IsZero() &&
			// empty location override might exists as only location part is different in found key
			bytes.HasPrefix(k, key[:locationStart]) {
			key = append(key[:locationStart], ZeroID...)

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

		// Peel off one or more labels from reversedQName until
		// the remainder matches k.
		priorQLength := qLength
		qLength = findCommonLongestPrefix(reversedQName[:qLength-1], k[domainNameStart:]) + 1
		if qLength == priorQLength {
			qLength = getLengthWithoutLastLabel(reversedQName, qLength)
		}
	}
	return err
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
			if j >= len(str1) || j >= len(str2) || str1[j] != str2[j] {
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
