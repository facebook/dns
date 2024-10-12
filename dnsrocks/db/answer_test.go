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
	"errors"
	"fmt"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/testaid"
)

type isAuthTestCase struct {
	qname             string
	locID             ID
	flagns            bool
	flagauthoritative bool
	authdomain        string
	expectedErr       error
}

func getAuthTestCases() []isAuthTestCase {
	return []isAuthTestCase{
		{
			qname:             "www.example.com.", // This is a CNAME
			locID:             []byte{0, 1},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "wildcard.example.com.", // This is a CNAME
			locID:             []byte{0, 1},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "foo.example.com.", // This is a A Record
			locID:             []byte{0, 1},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "long.example.com.", // This is a A Record
			locID:             []byte("\xff\x05other"),
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "example.com.",
			locID:             []byte{0, 1},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "foo.nonauth.example.com.",
			locID:             []byte{0, 1},
			flagns:            true,
			flagauthoritative: false,
			authdomain:        "nonauth.example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "badexample.com.",
			locID:             []byte{0, 1},
			flagns:            false,
			flagauthoritative: false,
			authdomain:        ".", // Not authoritative
			expectedErr:       nil,
		},
	}
}

type findAnswerTestCase struct {
	qname            string
	qtype            uint16
	locID            ID
	authdomain       string
	expectedRecords  bool
	expectedNXDomain bool
}

func getFindAnswerTestCases() []findAnswerTestCase {
	return []findAnswerTestCase{
		{
			qname:            "www.example.com.", // This is a CNAME
			qtype:            dns.TypeCNAME,
			locID:            []byte{0, 1},
			authdomain:       "example.com.",
			expectedRecords:  true,
			expectedNXDomain: false,
		},
		{
			qname:            "wildcard.example.com.", // This is a CNAME
			qtype:            dns.TypeCNAME,
			locID:            []byte{0, 1},
			authdomain:       "example.com.",
			expectedRecords:  true,
			expectedNXDomain: false,
		},
		{
			qname:            "foo.example.com.", // This is a A Record
			qtype:            dns.TypeA,
			locID:            []byte{0, 1},
			authdomain:       "example.com.",
			expectedRecords:  true,
			expectedNXDomain: false,
		},
		{
			qname:            "example.com.",
			qtype:            dns.TypeMX,
			locID:            []byte{0, 1},
			authdomain:       "example.com.",
			expectedRecords:  true,
			expectedNXDomain: false,
		},
		{
			qname:            "example.com.",
			qtype:            dns.TypeA,
			locID:            []byte{0, 1},
			authdomain:       "example.com.",
			expectedRecords:  false,
			expectedNXDomain: false, // we have the records for other types (MX), so return NOERROR
		},
	}
}

func BenchmarkIsAuthoritative(b *testing.B) {
	var (
		packedQName = make([]byte, 255)
		db          *DB
		err         error
	)

	benchmarks := getAuthTestCases()

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			b.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			b.Fatalf("Could not open db file: %v", err)
		}

		for _, bm := range benchmarks {
			benchname := fmt.Sprintf("%s(%s)/%s-%v", config.Driver, config.Flavour, bm.qname, bm.locID)
			b.Run(benchname, func(b *testing.B) {
				offset, _ := dns.PackDomainName(bm.qname, packedQName, 0, nil, false)
				for i := 0; i < b.N; i++ {
					_, _, _, err = r.IsAuthoritative(packedQName[:offset], bm.locID)
					if err != nil {
						b.Fatalf("%v", err)
					}
				}
			})
		}
	}
}

func TestFBDNSDBDnsLabelWildsafe(t *testing.T) {
	testCases := []struct {
		label  []byte
		result bool
	}{
		{
			label:  []byte{'a', 'b', '0', '-', '_'},
			result: true,
		},
		{
			label:  []byte{'a', 'b', '/', '-', '_'},
			result: false,
		},
		{
			label:  []byte{'a', 'b', '\023', '-', '_'},
			result: false,
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
			result := dnsLabelWildsafe(tc.label)
			require.Equal(t, tc.result, result)
		})
	}
}

func TestDBAuthoritative(t *testing.T) {
	var db *DB
	var err error
	var q = make([]byte, 255)

	testCases := getAuthTestCases()

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nil(t, err, "could not open fixture database")
		r, err := NewReader(db)
		require.Nil(t, err, "could not open db file")

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s-%v", tc.qname, tc.locID), func(t *testing.T) {
				offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
				require.Nilf(t, err, "failed at packing domain %s", tc.qname)

				flagns, flagauthoritative, domain, err := r.IsAuthoritative(q[:offset], tc.locID)
				require.Equalf(t, tc.flagns, flagns, "expected flagns %t", tc.flagns)
				require.Equalf(
					t, tc.flagauthoritative, flagauthoritative,
					"expected flagauthoritative %t", tc.flagauthoritative,
				)
				require.Equalf(t, err, tc.expectedErr, "expected error %v", tc.expectedErr)

				require.Nil(t, err)
				fqdn, _, err := dns.UnpackDomainName(domain, 0)
				if err == nil {
					require.Equalf(t, tc.authdomain, fqdn, "expected auth domain %s", tc.authdomain)
				}
			})
		}
	}
}

func TestDBFindAnswer(t *testing.T) {
	var db *DB
	var err error
	var q = make([]byte, 255)
	var controlName = make([]byte, 255)

	testCases := getFindAnswerTestCases()

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nil(t, err, "could not open fixture database")
		r, err := NewReader(db)
		require.Nil(t, err, "could not open db file")

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s-%v", config.Driver, tc.qname, tc.locID), func(t *testing.T) {
				offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
				require.Nilf(t, err, "failed at packing domain %s", tc.qname)
				controlOffset, err := dns.PackDomainName(tc.authdomain, controlName, 0, nil, false)
				require.Nilf(t, err, "failed at packing domain %s", tc.qname)
				a := new(dns.Msg)
				a.Compress = true
				a.Authoritative = true

				weighted, recordFound := r.FindAnswer(q[:offset], controlName[:controlOffset], tc.qname, tc.qtype, tc.locID, a, 10)
				require.False(t, weighted)
				if tc.expectedNXDomain {
					require.False(t, recordFound)
				} else {
					require.True(t, recordFound)
				}

				if tc.expectedRecords {
					require.Equalf(t, 1, len(a.Answer), "expect %v to have at least one record", a.Answer)
					require.Equal(t, tc.qname, a.Answer[0].Header().Name)
				} else {
					require.Equalf(t, 0, len(a.Answer), "expect %v to have no records", a.Answer)
				}
			})
		}
	}
}

func BenchmarkFindAnswer(b *testing.B) {
	var (
		packedQName = make([]byte, 255)
		db          *DB
		err         error
		controlName = make([]byte, 255)
	)

	benchmarks := getFindAnswerTestCases()

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			b.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			b.Fatalf("Could not open db file: %v", err)
		}

		for _, bm := range benchmarks {
			benchname := fmt.Sprintf("%s(%s)/%s-%v", config.Driver, config.Flavour, bm.qname, bm.locID)
			b.Run(benchname, func(b *testing.B) {
				offset, err := dns.PackDomainName(bm.qname, packedQName, 0, nil, false)
				require.Nilf(b, err, "failed at packing domain %s", bm.qname)
				controlOffset, err := dns.PackDomainName(bm.authdomain, controlName, 0, nil, false)
				require.Nilf(b, err, "failed at packing domain %s", bm.qname)
				a := new(dns.Msg)
				a.Compress = true
				a.Authoritative = true
				for i := 0; i < b.N; i++ {
					_, recordFound := r.FindAnswer(packedQName[:offset], controlName[:controlOffset], bm.qname, bm.qtype, bm.locID, a, 10)
					if bm.expectedNXDomain && recordFound {
						b.Fatal("unexpectedly found missing record")
					}
				}
			})
		}
	}
}

func TestDBFindSOA(t *testing.T) {
	var db *DB
	var err error
	var zoneCut = make([]byte, 255)

	testCases := []struct {
		zoneCutString  string
		expectedLength int
	}{
		// matching SOA for zone cut
		{
			zoneCutString:  "example.com.",
			expectedLength: 1,
		},
		// matching SOA for zone cut but we assume all lowercase.
		{
			zoneCutString:  "eXample.com.",
			expectedLength: 0,
		},
		// no matching SOa for zone cut
		{
			zoneCutString:  "foo.example.com.",
			expectedLength: 0,
		},
	}

	locID := []byte{0, 0}

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nil(t, err, "could not open fixture database")
		r, err := NewReader(db)
		require.Nil(t, err, "could not open db file")
		t.Run(config.Driver, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.zoneCutString, func(t *testing.T) {
					a := new(dns.Msg)
					offset, err := dns.PackDomainName(tc.zoneCutString, zoneCut, 0, nil, false)
					require.Nilf(t, err, "Failed at packing zoneCut %s", tc.zoneCutString)
					FindSOA(r, zoneCut[:offset], tc.zoneCutString, locID, a)
					require.Equalf(t, tc.expectedLength, len(a.Ns),
						"Expected authoritative section length %d, got %d",
						tc.expectedLength, len(a.Ns))
				})
			}
		})
	}
}

func TestExtractRRFromRow(t *testing.T) {
	type testCase struct {
		name       string
		row        []byte
		wildcard   bool
		expectedRR ResourceRecord
	}
	testCases := []testCase{{
		name:     "A record with long ID",
		row:      []byte{0, 1, 62, 0xff, 6, 'a', 'b', 'c', '1', '2', '3', 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 192, 0, 2, 1},
		wildcard: false,
		expectedRR: ResourceRecord{
			Weight: 100,
			Qtype:  1,
			TTL:    60,
			Offset: 27,
		},
	}, {
		name:     "AAAA record with long ID",
		row:      []byte{0, 28, 62, 0xff, 6, 'a', 'b', 'c', '1', '2', '3', 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		wildcard: false,
		expectedRR: ResourceRecord{
			Weight: 100,
			Qtype:  28,
			TTL:    60,
			Offset: 27,
		},
	}, {
		name:     "A record with short ID",
		row:      []byte{0, 1, 62, 0xa, 0xb, 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 192, 0, 2, 1},
		wildcard: false,
		expectedRR: ResourceRecord{
			Weight: 100,
			Qtype:  1,
			TTL:    60,
			Offset: 21,
		},
	}, {
		name:     "AAAA record with short ID",
		row:      []byte{0, 28, 62, 0xa, 0xb, 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		wildcard: false,
		expectedRR: ResourceRecord{
			Weight: 100,
			Qtype:  28,
			TTL:    60,
			Offset: 21,
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rr, err := ExtractRRFromRow(tc.row, tc.wildcard)
			require.NoError(t, err)
			require.Equal(t, tc.expectedRR, rr)
		})
	}
}

func TestExtractRRFromRow_Invalid(t *testing.T) {
	// Valid row
	row := []byte{0, 1, 62, 0xff, 6, 'a', 'b', 'c', '1', '2', '3', 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 192, 0, 2, 1}

	// Meta-test: check that the valid row is permitted
	_, err := ExtractRRFromRow(row, false)
	require.NoError(t, err)
	// Remove RDATA.  Row is still valid
	_, err = ExtractRRFromRow(row[:len(row)-4], false)
	require.NoError(t, err)

	// Further truncations are all invalid.
	for i := 0; i < len(row)-4; i++ {
		_, err := ExtractRRFromRow(row[:i], false)
		require.Error(t, err)
	}
}

func TestParseResult(t *testing.T) {
	testCases := []struct {
		name        string
		result      []byte
		qtype       uint16
		answer      []dns.RR
		wrsV4Count  uint32
		recordFound bool
		err         error
	}{

		{
			name:        "Malformed input",
			result:      []byte{'e', 'r', 'r'},
			qtype:       dns.TypeA,
			answer:      []dns.RR(nil),
			wrsV4Count:  0,
			recordFound: false,
			err:         errors.New("EOF"),
		},
		{
			name:        "Integer overflow",
			result:      make([]byte, 65551),
			qtype:       dns.TypeANY,
			answer:      []dns.RR(nil),
			wrsV4Count:  0,
			recordFound: true,
			err:         errors.New("integer overflow for uint16 RR_Header.Rdlength"),
		},
		{
			name:        "Add to Wrs",
			result:      []byte{0, 1, 62, 0xa, 0xb, 0, 0, 0, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 192, 0, 2, 1},
			qtype:       dns.TypeA,
			answer:      []dns.RR(nil),
			wrsV4Count:  1,
			recordFound: true,
			err:         nil,
		},
		{
			name:   "Add to Answer",
			result: []byte{0, 5, 61, 0, 0, 14, 16, 0, 0, 0, 0, 0, 0, 0, 0, 3, 'w', 'w', 'w', 7, 'n', 'o', 'n', 'a', 'u', 't', 'h', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0},
			qtype:  dns.TypeCNAME,
			answer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:     "www.example.com.",
						Rrtype:   dns.TypeCNAME,
						Class:    dns.ClassINET,
						Ttl:      3600,
						Rdlength: 25,
					},
					Target: "www.nonauth.example.com.",
				},
			},
			wrsV4Count:  0,
			recordFound: true,
			err:         nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rp := &recordProcessor{
				msg:   new(dns.Msg),
				wrs:   Wrs{MaxAnswers: 10},
				qname: "www.example.com.",
				qtype: tc.qtype,
			}

			err := rp.parseResult(tc.result)
			require.Equal(t, tc.answer, rp.msg.Answer)
			require.Equal(t, tc.wrsV4Count, rp.wrs.V4Count)
			require.Equal(t, tc.recordFound, rp.recordFound)
			require.Equal(t, tc.err, err)
		})
	}
}
