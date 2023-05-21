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
	"net"
	"os"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/dns/dnsrocks/testaid"
)

func TestMain(m *testing.M) {
	os.Exit(testaid.Run(m, "../testdata/data"))
}

func makeECSOption(s string) (*dns.OPT, error) {
	o := new(dns.OPT)
	o.Hdr.Name = "."
	o.Hdr.Rrtype = dns.TypeOPT
	e := new(dns.EDNS0_SUBNET)
	e.Code = dns.EDNS0SUBNET
	ipaddr, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}

	if ipaddr.To4() != nil {
		e.Family = 1
	} else {
		e.Family = 2
	}
	e.Address = ipaddr.To16()
	msize, _ := ipnet.Mask.Size()
	e.SourceNetmask = uint8(msize)
	o.Option = append(o.Option, e)
	return o, nil
}

func BenchmarkResolverLocation(b *testing.B) {
	var packedQName = make([]byte, 255)
	var db *DB
	var err error
	qname := "foo.example.org." // "example.com."
	benchmarks := []struct {
		name string
		ip   string
	}{
		{
			name: "v4 localhost (worst case)",
			ip:   "127.0.0.1",
		},
		{
			name: "v4 /32 match (best case)",
			ip:   "10.255.255.255",
		},
		{
			name: "v4 /16 match",
			ip:   "10.255.0.0",
		},
		{
			name: "v6 localhost (worst case)",
			ip:   "::1",
		},
		{
			name: "v6 /128 match (best case)",
			ip:   "fd76:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		},
		{
			name: "v6 /64 match",
			ip:   "fd76:ffff:ffff:ffff::",
		},
		{
			name: "v6 /56 match",
			ip:   "fd76:ffff:ffff:ff00::",
		},
		{
			name: "v6 /48 match",
			ip:   "fd76:ffff:ffff::",
		},
	}
	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			b.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			b.Fatalf("Could not open db file: %v", err)
		}
		offset, _ := dns.PackDomainName(qname, packedQName, 0, nil, false)
		_separateBitMap := SeparateBitMap
		for _, s := range []bool{true, false} {
			SeparateBitMap = s
			for _, bm := range benchmarks {
				b.Run(fmt.Sprintf("%s/%s SeparateBitMap %v", config.Driver, bm.name, s), func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						_, err := r.ResolverLocation(packedQName[:offset], bm.ip)
						if err != nil {
							b.Fatalf("%v", err)
						}
					}
				})
			}
		}
		SeparateBitMap = _separateBitMap
	}
}

func BenchmarkECSLocation(b *testing.B) {
	var packedQName = make([]byte, 255)
	var db *DB
	var err error
	qname := "foo.example.org." // "example.com."
	benchmarks := []struct {
		name   string
		subnet string
	}{
		{
			name:   "v4 localhost (worst case)",
			subnet: "127.0.0.1/32",
		},
		{
			name:   "v4 /32 match (best case)",
			subnet: "10.255.255.255/32",
		},
		{
			name:   "v4 /16 match",
			subnet: "10.255.0.0/24",
		},
		{
			name:   "v6 localhost (worst case)",
			subnet: "::1/128",
		},
		{
			name:   "v6 /128 match (best case)",
			subnet: "fd76:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128",
		},
		{
			name:   "v6 /64 match",
			subnet: "fd76:ffff:ffff:ffff::/128",
		},
		{
			name:   "v6 /56 match",
			subnet: "fd76:ffff:ffff:ff00::/128",
		},
		{
			name:   "v6 /48 match",
			subnet: "fd76:ffff:ffff::/128",
		},
	}

	// lifted from dnsserver/handler.go to avoid circular dependency
	MakeOPTWithECS := func(s string) (*dns.OPT, error) {
		o := new(dns.OPT)
		o.Hdr.Name = "."
		o.Hdr.Rrtype = dns.TypeOPT
		e := new(dns.EDNS0_SUBNET)
		e.Code = dns.EDNS0SUBNET
		ipaddr, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}

		if ipaddr.To4() != nil {
			e.Family = 1
			e.Address = ipaddr.To4()
		} else {
			e.Family = 2
			e.Address = ipaddr
		}
		msize, _ := ipnet.Mask.Size()
		e.SourceNetmask = uint8(msize)
		o.Option = append(o.Option, e)
		return o, nil
	}

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			b.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			b.Fatalf("Could not open db file: %v", err)
		}
		offset, _ := dns.PackDomainName(qname, packedQName, 0, nil, false)
		_separateBitMap := SeparateBitMap
		for _, s := range []bool{true, false} {
			SeparateBitMap = s
			for _, bm := range benchmarks {
				b.Run(fmt.Sprintf("%s/%s SeparateBitMap %v", config.Driver, bm.name, s), func(b *testing.B) {
					edns, _ := MakeOPTWithECS(bm.subnet)
					for n := 0; n < b.N; n++ {
						_, err := r.EcsLocation(packedQName[:offset], edns.Option[0].(*dns.EDNS0_SUBNET))
						if err != nil {
							b.Fatalf("%v", err)
						}
					}
				})
			}
		}
		SeparateBitMap = _separateBitMap
	}
}

func testDBFindLocationCustomBitmap(t *testing.T, separateBitmap bool) {
	var db *DB
	var err error
	var q = make([]byte, 255)

	SeparateBitMap = separateBitmap

	testCases := []struct {
		desc             string
		qname            string
		qmap             []byte
		ip               string
		mask             uint8
		expectedLocation Location
	}{
		/** Resolvers */
		{
			desc:             "no map assigned to www.example.com",
			qname:            "www.example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "1.1.1.1",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "not such resolver match, will be caught by default",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "1.1.1.0",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 96, LocID: [2]byte{0, 1}},
		},
		{
			desc:             "caught by resolver 1.1.1.1/32 map number 2",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "1.1.1.1",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 128, LocID: [2]byte{0, 2}},
		},
		{
			desc:             "caught by resolver 2.2.2.0/24 map number 3",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "2.2.2.5",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 120, LocID: [2]byte{0, 3}},
		},
		/** ECS */
		{
			desc:             "no map assigned to www.example.com",
			qname:            "www.example.com.",
			qmap:             []byte{0, '8'},
			ip:               "1.1.1.1",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "not such ECS subnet match, wont set LocID/mask",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "1.1.2.0",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "caught by ECS subnet 1.1.1.0/24 map number 2",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "1.1.1.1",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 2}},
		},
		{
			desc:             "caught by ECS subnet 2.2.2.0/24 map number 3",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "2.2.2.5",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 3}},
		},
		{
			desc:             "caught by ECS subnet 2.2.2.0/24 map number 3",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "2.2.2.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 3}},
		},
		{
			desc:             "caught by ECS subnet 2.2.3.0/24 map number 3",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "2.2.3.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 3}},
		},
		{
			desc:             "no such ECS subnet 2.2.2.0/23, this verifies that we wont be caught by 2.2.2.0/24 or 2.2.3.0/24",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "2.2.2.0",
			mask:             23,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		/*
			fd8f:a2ea:9f4b::
			fd48:6525:66bd
		*/
		{
			desc:             "no map assigned to www.example.com",
			qname:            "www.example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "fd8f:a2ea:9f4b::1",
			mask:             128,
			expectedLocation: Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "not such resolver match, will be caught by default",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "fdff::",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 0, LocID: [2]byte{0, 1}},
		},
		{
			desc:             "caught by resolver fd8f:a2ea:9f4b::/56 map number 4",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "fd8f:a2ea:9f4b::1",
			mask:             128,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 56, LocID: [2]byte{0, 4}},
		},
		{
			desc:             "caught by resolver fd48:6525:66bd::/56 map number 5",
			qname:            "example.com.",
			qmap:             []byte{0, 'M'},
			ip:               "fd48:6525:66bd:1::",
			mask:             128,
			expectedLocation: Location{MapID: [2]byte{'c', 0}, Mask: 56, LocID: [2]byte{0, 5}},
		},
		/** ECS */
		{
			desc:             "no map assigned to www.example.com",
			qname:            "www.example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fd8f:a2ea:9f4b:1::",
			mask:             64,
			expectedLocation: Location{MapID: [2]byte{0, 0}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "not such ECS subnet match, wont set LocID/mask",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fdff::",
			mask:             64,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "caught by ECS subnet fd8f:a2ea:9f4b::/56 map number 4",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fd8f:a2ea:9f4b:1::",
			mask:             64,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 56, LocID: [2]byte{0, 4}},
		},
		{
			desc:             "caught by ECS subnet fd48:6525:66bd::/56 map number 5",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fd48:6525:66bd:1::",
			mask:             64,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 56, LocID: [2]byte{0, 5}},
		},
		{
			desc:             "no such ECS subnet fd48:6525::/32, this verifies that we wont be caught by fd48:6525:66bd::/56",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fd48:6525::",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		{
			desc:             "no such ECS subnet fd48:6525:66bd::/48, this verifies that we wont be caught by fd48:6525:66bd::/56",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "fd48:6525:66bd::",
			mask:             48,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 0, LocID: [2]byte{0, 0}},
		},
		// the following ECS tests are supposed to confuse rearranger for RDB
		{
			desc:             "4.0.0.0/8 exists, and should be 6",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.0.0.0",
			mask:             8,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 104, LocID: [2]byte{0, 6}},
		},
		{
			desc:             "4.0.0.0/16 exists, and should be 7",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.0.0.0",
			mask:             16,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 112, LocID: [2]byte{0, 7}},
		},
		{
			desc:             "4.0.0.0/24 exists, and should be 8",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.0.0.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 8}},
		},
		{
			desc:             "4.4.4.0/24 should be 9",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.4.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 9}},
		},
		{
			desc:             "4.4.5.0/24 should be 10",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.5.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 10}},
		},
		{
			desc:             "4.4.0.0/16 should be caught by 4.0.0.0/8",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.0.0",
			mask:             16,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 104, LocID: [2]byte{0, 6}},
		},
		{
			desc:             "4.4.4.4/32 should be caught by 4.4.4.0/24",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.4.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 9}},
		},
		{
			desc:             "4.4.5.5/32 should be caught by 4.4.5.0/24",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.5.0",
			mask:             24,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 10}},
		},
		{
			desc:             "4.4.5.1/32 should be 11",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.5.1",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 128, LocID: [2]byte{0, 11}},
		},
		{
			desc:             "4.4.5.2/32 should be caught by 4.4.5.0/24",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.5.2",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 120, LocID: [2]byte{0, 10}},
		},
		{
			desc:             "4.4.5.3/32 should be 12",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.5.3",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 128, LocID: [2]byte{0, 12}},
		},
		{
			desc:             "4.4.6.0/32 should be caught by 4.0.0.0/8",
			qname:            "example.com.",
			qmap:             []byte{0, '8'},
			ip:               "4.4.6.0",
			mask:             32,
			expectedLocation: Location{MapID: [2]byte{'e', 'c'}, Mask: 104, LocID: [2]byte{0, 6}},
		},
	}
	for _, dbconfig := range testaid.TestDBs {
		if db, err = Open(dbconfig.Path, dbconfig.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			t.Fatalf("Could not open db file: %v", err)
		}

		for i, tc := range testCases {
			offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
			require.Nilf(t, err, "failed at packing domain %s", tc.qname)
			if err != nil {
				t.Fatalf("failed at packing domain %s: %v", tc.qname, err)
			}
			t.Run(fmt.Sprintf("%s/dbi.FindMap for test case %d/%s", dbconfig.Driver, i, tc.desc), func(t *testing.T) {
				var location Location
				id, err := db.dbi.FindMap(q[:offset], tc.qmap, db.dbi.NewContext())
				if id != nil {
					copy(location.MapID[:], id)
				}
				require.Nil(t, err)
				require.Equal(t, tc.expectedLocation.MapID, location.MapID, "MapID does not match.")
			})
			t.Run(fmt.Sprintf("%s/%d/%s", dbconfig.Driver, i, tc.desc), func(t *testing.T) {
				offset, err := dns.PackDomainName(tc.qname, q, 0, nil, false)
				require.NoError(t, err)
				_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", tc.ip, tc.mask))
				require.NoError(t, err)
				location, err := r.findLocation(q[:offset], tc.qmap, ipnet)
				require.Nil(t, err)
				if location != nil {
					require.Equal(t, tc.expectedLocation, *location, "Location does not match.")
				}
			})
		}
	}
}

// TestDBFindLocation checks locations
func TestDBFindLocation(t *testing.T) {
	_separateBitMap := SeparateBitMap
	defer func() {
		SeparateBitMap = _separateBitMap
	}()

	t.Run("TestDBFindLocationCustomBitmap !SeparateBitMap", func(t *testing.T) {
		testDBFindLocationCustomBitmap(t, false)
	})

	t.Run("TestDBFindLocationCustomBitmap SeparateBitMap", func(t *testing.T) {
		testDBFindLocationCustomBitmap(t, true)
	})
}

func testFindLocationForResolversCustomBitmap(t *testing.T, separateBitmap bool) {
	var db *DB
	var packedQName = make([]byte, 255)
	var err error
	SeparateBitMap = separateBitmap

	testCases := []struct {
		domain           string
		resolver         string
		expectedLocation Location
	}{
		{
			domain:           "cnamemap.example.com.",
			resolver:         "1.1.1.1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 128, LocID: [2]byte{0, 2}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "1.1.0.1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 96, LocID: [2]byte{0, 1}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "2.2.2.2",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 120, LocID: [2]byte{0, 3}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "fd8f:a2ea:9f4b::1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 56, LocID: [2]byte{0, 4}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "::1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 0, LocID: [2]byte{0, 1}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "fd58:6525:66bd:a::1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 64, LocID: [2]byte{0, 3}},
		},
		{
			domain:           "cnamemap.example.com.",
			resolver:         "fd58:6525:66bd::1",
			expectedLocation: Location{MapID: [2]byte{'c', '\000'}, Mask: 56, LocID: [2]byte{0, 2}},
		},
	}

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			t.Fatalf("Could not open db file: %v", err)
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/find location %v", config.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.domain), dns.TypeA)
				offset, _ := dns.PackDomainName(tc.domain, packedQName, 0, nil, false)
				ecs, loc, err := r.FindLocation(packedQName[:offset], req, tc.resolver)
				require.Nil(t, ecs)
				require.Nil(t, err)
				if loc != nil {
					require.Equal(t, tc.expectedLocation, *loc)
				}
			})
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/resolver location %v", config.Driver, tc), func(t *testing.T) {
				offset, _ := dns.PackDomainName(tc.domain, packedQName, 0, nil, false)
				loc, err := r.ResolverLocation(packedQName[:offset], tc.resolver)
				require.Nil(t, err)
				if loc != nil {
					require.Equal(t, tc.expectedLocation, *loc)
				}
			})
		}
	}
}

// TestFindLocationForResolvers checks locations
func TestFindLocationForResolvers(t *testing.T) {
	_separateBitMap := SeparateBitMap
	defer func() {
		SeparateBitMap = _separateBitMap
	}()
	t.Run("TestFindLocationForResolversCustomBitmap !SeparateBitMap", func(t *testing.T) {
		testFindLocationForResolversCustomBitmap(t, false)
	})

	t.Run("TestFindLocationForResolversCustomBitmap SeparateBitMap", func(t *testing.T) {
		testFindLocationForResolversCustomBitmap(t, true)
	})
}

func testDBEcsLocationCustomBitmap(t *testing.T, separateBitMap bool) {
	var db *DB
	var packedQName = make([]byte, 255)
	var err error
	SeparateBitMap = separateBitMap

	testCases := []struct {
		domain string
		ecs    string
		scope  uint8
	}{
		// We would need a more specific SourceNetmask
		{
			domain: "foo.example.org.",
			ecs:    "1.1.1.0/22",
			scope:  24,
		},
		// Matching scope
		{
			domain: "foo.example.org.",
			ecs:    "2.2.2.0/24",
			scope:  24,
		},
		// We don't need that specific, return our scope
		{
			domain: "foo.example.org.",
			ecs:    "2.2.2.0/32",
			scope:  24,
		},
		// We have no matching for this range, return out default scope.
		{
			domain: "foo.example.org.",
			ecs:    "3.3.3.0/32",
			scope:  24,
		},
		// There is no ECS for this domain. We should return 0 so it is cached for
		// all subnets.
		{
			domain: "bar.example.org.",
			ecs:    "1.1.1.0/24",
			scope:  0,
		},
	}

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		if err != nil {
			t.Fatalf("Could not open db file: %v", err)
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", config.Driver, tc), func(t *testing.T) {
				edns, err := makeECSOption(tc.ecs)
				require.Nilf(t, err, "Failed to generate ECS option for %s", tc.ecs)
				ecs := edns.Option[0].(*dns.EDNS0_SUBNET)
				offset, err := dns.PackDomainName(tc.domain, packedQName, 0, nil, false)
				require.NoError(t, err)
				_, err = r.EcsLocation(packedQName[:offset], ecs)
				require.Nil(t, err)
				require.Equal(t, tc.scope, ecs.SourceScope)
			})
		}
	}
}

// TestFindLocationForResolvers checks locations
func TestDBEcsLocation(t *testing.T) {
	_separateBitMap := SeparateBitMap
	defer func() {
		SeparateBitMap = _separateBitMap
	}()

	t.Run("TestDBEcsLocationCustomBitmap !SeparateBitMap", func(t *testing.T) {
		testDBEcsLocationCustomBitmap(t, false)
	})

	t.Run("TestDBEcsLocationCustomBitmap SeparateBitMap", func(t *testing.T) {
		testDBEcsLocationCustomBitmap(t, true)
	})
}

func testDBCorrectEcsAnswerCustomBitmap(t *testing.T, separateBitMap bool) {
	var db *DB
	var packedQName = make([]byte, 255)
	var err error
	SeparateBitMap = separateBitMap

	testCases := []struct {
		ecs         string
		expectedECS dns.EDNS0_SUBNET
	}{
		// We would need a more specific SourceNetmask
		{
			ecs: "3.3.3.1/32",
			expectedECS: dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 32,
				SourceScope:   32,
				Address:       net.ParseIP("3.3.3.1"),
			},
		},
		{
			ecs: "3.3.3.2/32",
			expectedECS: dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 32,
				SourceScope:   24,
				Address:       net.ParseIP("3.3.3.2"),
			},
		},
		// Matching scope
		{
			ecs: "fd8f:a2ea:9f4b::1/128",
			expectedECS: dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        2,
				SourceNetmask: 128,
				SourceScope:   56,
				Address:       net.ParseIP("fd8f:a2ea:9f4b::1"),
			},
		},
	}

	// TODO: aluck is rebuilding RDB maps (T41316106), testing only CDB for now
	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}
		r, err := NewReader(db)
		require.Nil(t, err, "Could not open db file")

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", config.Driver, tc), func(t *testing.T) {
				edns, err := makeECSOption(tc.ecs)
				require.Nilf(t, err, "Failed to generate ECS option for %s", tc.ecs)
				ecs := edns.Option[0].(*dns.EDNS0_SUBNET)
				offset, err := dns.PackDomainName("example.com.", packedQName, 0, nil, false)
				require.NoError(t, err)
				_, err = r.EcsLocation(packedQName[:offset], ecs)
				require.NoError(t, err)
				require.Equalf(t, tc.expectedECS, *ecs, "unexpected ECS response value")
			})
		}
	}
}

// TestDBCorrectEcsAnswer checks locations
func TestDBCorrectEcsAnswer(t *testing.T) {
	_separateBitMap := SeparateBitMap
	defer func() {
		SeparateBitMap = _separateBitMap
	}()

	t.Run("TestDBCorrectEcsAnswerCustomBitmap !SeparateBitMap", func(t *testing.T) {
		testDBCorrectEcsAnswerCustomBitmap(t, false)
	})

	t.Run("TestDBCorrectEcsAnswerCustomBitmap SeparateBitMap", func(t *testing.T) {
		testDBCorrectEcsAnswerCustomBitmap(t, true)
	})
}

// TestDBECSResolverFindMap tests that we can properly find a map ID for a given name.
// It test exact match as well as wildcard.
func TestDBECSResolverFindMap(t *testing.T) {
	var db *DB
	var packedQName = make([]byte, 255)
	var err error

	const (
		resolverMapType = iota
		ecsMapType
	)

	mapType := [][]byte{{0, 'M'}, {0, '8'}}

	testCases := []struct {
		domain  string
		mapID   []byte
		mapType int
	}{
		{
			domain: "cnamemap.example.org.",
			mapID:  []byte{'c', 0},
		},
		{
			domain: "cnamemap.example.org.",
			mapID:  []byte{'c', 0},
		},
		{
			domain: "blah.example.com",
			mapID:  nil,
		},
		{ // matches *.a.b.c.example.org (second wildcard search)
			domain: "foo.bar.a.b.c.example.org",
			mapID:  []byte{'M', 'a'},
		},
		{ // matches *.a.b.c.example.org (first wildcard search)
			domain: "d.a.b.c.example.org",
			mapID:  []byte{'M', 'a'},
		},
		{ // exact match d.a.b.c.d.example.org
			domain: "d.a.b.c.d.example.org",
			mapID:  []byte{'M', 'b'},
		},
		{
			domain:  "cnamemap.example.org.",
			mapID:   []byte{'e', 'c'},
			mapType: ecsMapType,
		},
		{
			domain:  "cnamemap.example.org.",
			mapID:   []byte{'e', 'c'},
			mapType: ecsMapType,
		},
		{
			domain:  "blah.example.com",
			mapID:   nil,
			mapType: ecsMapType,
		},
		{
			domain:  "foo.bar.a.b.c.example.org",
			mapID:   []byte{'8', 'a'},
			mapType: ecsMapType,
		},
		{
			domain:  "d.a.b.c.example.org",
			mapID:   []byte{'8', 'a'},
			mapType: ecsMapType,
		},
		{
			domain:  "d.a.b.c.d.example.org",
			mapID:   []byte{'8', 'b'},
			mapType: ecsMapType,
		},
	}

	for _, config := range testaid.TestDBs {
		if db, err = Open(config.Path, config.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", config.Driver, tc), func(t *testing.T) {
				offset, _ := dns.PackDomainName(dns.Fqdn(tc.domain), packedQName, 0, nil, false)
				mapID, err := db.dbi.FindMap(packedQName[:offset], mapType[tc.mapType], db.dbi.NewContext())
				require.Nilf(t, err, "Failed to fetch map for %#v", tc)
				require.Equal(t, tc.mapID, mapID)
			})
		}
	}
}
