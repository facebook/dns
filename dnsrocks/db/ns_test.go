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

// q []byte, zonename string, class uint16, loc *Location
func BenchmarkGetNs(b *testing.B) {
	var packedQName = make([]byte, 255)
	var loc = new(Location)
	var db *DB
	var err error

	benchmarks := []string{
		"example.com.",
		"nonauth.example.com.",
		"abc.example.com.", // nonexistent name
	}

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nilf(b, err, "Could not open fixture database")
		r, err := NewReader(db)
		require.Nilf(b, err, "Could not acquire new reader")

		for _, bm := range benchmarks {
			b.Run(fmt.Sprintf("%s/%s", config.Driver, bm), func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					offset, err := dns.PackDomainName(bm, packedQName, 0, nil, false)
					require.Nilf(b, err, "Could not pack domain %s", bm)
					_, err = GetNs(r, packedQName[:offset], bm, dns.ClassINET, loc)
					if err != nil {
						b.Fatalf("%v", err)
					}
				}
			})
		}
	}
}

func TestGetNs(t *testing.T) {
	var db *DB
	var err error
	var q = make([]byte, 255)

	testCases := []struct {
		qname         string
		location      Location
		expectedCount int
	}{
		{
			// example.com has NS records
			qname:         "example.com.",
			location:      Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
			expectedCount: 2,
		},
		{
			// nonauth.example.com has NS records
			qname:         "nonauth.example.com.",
			location:      Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
			expectedCount: 2,
		},
		{
			// doesnotexist.example.com does not have ns records but matches wildcard
			qname:         "doesnotexist.example.com.",
			location:      Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
			expectedCount: 0,
		},
		{
			// example.com has NS records but no specific location. We expect to
			// return the non localized records when we provide a location and there
			// is a location matching the client.
			qname:         "example.com.",
			location:      Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 2}},
			expectedCount: 2,
		},
	}

	for _, config := range testaid.TestDBs {
		db, err = Open(config.Path, config.Driver)
		require.Nilf(t, err, "Could not open fixture database")
		r, err := NewReader(db)
		require.Nilf(t, err, "Could not acquire new reader")

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", config.Driver, tc), func(t *testing.T) {
				offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
				require.Nilf(t, err, "Failed at packing domain %s: %v", tc.qname)
				rrs, err := GetNs(r, q[:offset], tc.qname, dns.ClassINET, &tc.location)
				require.Nil(t, err)
				require.Equal(t, tc.expectedCount, len(rrs))
			})
		}
	}
}
