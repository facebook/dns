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
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/dns/dnsrocks/testaid"
)

type isAuthTestCase struct {
	qname             string
	location          *Location
	flagns            bool
	flagauthoritative bool
	authdomain        string
	expectedErr       error
}

func getAuthTestCases() []isAuthTestCase {
	return []isAuthTestCase{
		{
			qname:             "www.example.com.", // This is a CNAME
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "wildcard.example.com.", // This is a CNAME
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "foo.example.com.", // This is a A Record
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "example.com.",
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            true,
			flagauthoritative: true,
			authdomain:        "example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "foo.nonauth.example.com.",
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            true,
			flagauthoritative: false,
			authdomain:        "nonauth.example.com.",
			expectedErr:       nil,
		},
		{
			qname:             "badexample.com.",
			location:          &Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
			flagns:            false,
			flagauthoritative: false,
			authdomain:        ".", // Not authoritative
			expectedErr:       nil,
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
			benchname := fmt.Sprintf("%s/%s-%v", config.Driver, bm.qname, bm.location.LocID)
			b.Run(benchname, func(b *testing.B) {
				offset, _ := dns.PackDomainName(bm.qname, packedQName, 0, nil, false)
				for i := 0; i < b.N; i++ {
					_, _, _, err = r.IsAuthoritative(packedQName[:offset], bm.location)
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
			t.Run(fmt.Sprintf("%s-%v", tc.qname, tc.location.LocID), func(t *testing.T) {
				offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
				require.Nilf(t, err, "failed at packing domain %s", tc.qname)

				flagns, flagauthoritative, domain, err := r.IsAuthoritative(q[:offset], tc.location)
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

	loc := &Location{}

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nil(t, err, "could not open fixture database")
		r, err := NewReader(db)
		require.Nil(t, err, "could not open db file")

		for _, tc := range testCases {
			t.Run(tc.zoneCutString, func(t *testing.T) {
				a := new(dns.Msg)
				offset, err := dns.PackDomainName(tc.zoneCutString, zoneCut, 0, nil, false)
				require.Nilf(t, err, "Failed at packing zoneCut %s", tc.zoneCutString)
				FindSOA(r, zoneCut[:offset], tc.zoneCutString, loc, a)
				require.Equalf(t, tc.expectedLength, len(a.Ns),
					"Expected authoritative section length %d, got %d",
					tc.expectedLength, len(a.Ns))
			})
		}
	}
}
