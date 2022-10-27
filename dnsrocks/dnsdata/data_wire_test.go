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

package dnsdata

import (
	"encoding"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWireRecordIsImplemented(t *testing.T) {
	type testCase struct {
		in         string
		record     encoding.TextUnmarshaler
		wireType   WireType
		domainName string
		location   Loc
		ttl        uint32
	}

	tests := []testCase{
		{
			in:         "Zt.org,a.ns.t.org,dns.t.org,111,7201,1801,604801,121,119,,\004\010",
			record:     &Rsoa{},
			wireType:   TypeSOA,
			domainName: "t.org",
			location:   []byte("\004\010"),
			ttl:        119,
		},
		{
			in:         "+test.net,1.2.3.4,3600,,\001\002",
			record:     &Raddr{},
			wireType:   TypeA,
			domainName: "test.net",
			location:   []byte("\001\002"),
			ttl:        3600,
		},
		{
			in:         "+test.com,fc0a:14f5:dead:beef:1::37,1800,,\002\003",
			record:     &Raddr{},
			wireType:   TypeAAAA,
			domainName: "test.com",
			location:   []byte("\002\003"),
			ttl:        1800,
		},
		{
			in:         "&test.net,fd09:14f5:dead:beef:1::34,ns2.dot.com,3601,,\004\005",
			record:     &Rns1{},
			wireType:   TypeNS,
			domainName: "test.net",
			location:   []byte("\004\005"),
			ttl:        3601,
		},
		{
			in:         "Ctest.com,target.com,1801,,\005\006",
			record:     &Rcname{},
			wireType:   TypeCNAME,
			domainName: "test.com",
			location:   []byte("\005\006"),
			ttl:        1801,
		},
		{
			in:         "^168.192.in-addr.arpa,some.host.net,1802,,\006\007",
			record:     &Rptr{},
			wireType:   TypePTR,
			domainName: "168.192.in-addr.arpa",
			location:   []byte("\006\007"),
			ttl:        1802,
		},
		{
			in:         "@testmail.com,,mail.store.com,0,1803,,\007\010",
			record:     &Rmx1{},
			wireType:   TypeMX,
			domainName: "testmail.com",
			location:   []byte("\007\010"),
			ttl:        1803,
		},
		{
			in:         "'test.org,blah blah blah,1804,,\010\011",
			record:     &Rtxt{},
			wireType:   TypeTXT,
			domainName: "test.org",
			location:   []byte("\010\011"),
			ttl:        1804,
		},
		{
			in:         "Stest.com,1.2.3.45,srv.test.com,10,0,443,1805,,\011\012",
			record:     &Rsrv1{},
			wireType:   TypeSRV,
			domainName: "test.com",
			location:   []byte("\011\012"),
			ttl:        1805,
		},
		{
			in:         "Htest.com,www.test.com,1806,\002\003,0",
			record:     &Rhttps{},
			wireType:   TypeHTTPS,
			domainName: "test.com",
			location:   []byte("\002\003"),
			ttl:        1806,
		},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			err := tc.record.UnmarshalText([]byte(tc.in))
			assert.NoError(t, err)
			wireRecord, ok := tc.record.(WireRecord)

			assert.True(t, ok)

			assert.Equal(t, tc.wireType, wireRecord.WireType())
			assert.Equal(t, tc.domainName, wireRecord.DomainName())
			assert.Equal(t, tc.location, wireRecord.Location())
			assert.Equal(t, tc.ttl, wireRecord.TTL())
		})
	}
}
