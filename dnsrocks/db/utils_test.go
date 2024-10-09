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
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/testaid"
)

const (
	AnswerSection     = iota
	NsSection         = iota
	AdditionalSection = iota
)

func RRSliceMatchSubsetf(t *testing.T, list, subset []dns.RR, msg string, args ...interface{}) {
	a := make([]string, 8)
	b := make([]string, 8)
	for _, x := range list {
		a = append(a, x.String())
	}
	for _, x := range subset {
		b = append(b, x.String())
	}
	require.Subsetf(t, a, b, msg, args...)
}

func answerSkeleton() *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn("foo.example.org."), dns.TypeA)
	a := new(dns.Msg)
	a.SetReply(q)

	a.Answer = append(a.Answer, &dns.CNAME{
		Hdr: dns.RR_Header{
			Name:   "foo.example.org.",
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		Target: "bar.example.org.",
	})
	a.Answer = append(a.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "bar.example.org.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("10.0.0.1"),
	})

	a.Ns = append(a.Ns, &dns.NS{
		Hdr: dns.RR_Header{
			Name:   "example.org.",
			Rrtype: dns.TypeNS,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		Ns: "a.ns.example.org.",
	})
	a.Extra = append(a.Extra, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "a.ns.example.org.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("10.0.0.2"),
	})
	a.Extra = append(a.Extra, &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   "a.ns.example.org.",
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		AAAA: net.ParseIP("::1"),
	})
	return a
}

func answerSkeletonForAdditionalSectionTest(qname string, qtype uint16) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(qname), qtype)
	a := new(dns.Msg)
	a.SetReply(q)

	return a
}

// TestHasRecord test that we can properly detect if a qname/qtype tuple is in
// a dns message.
func TestHasRecord(t *testing.T) {
	a := answerSkeleton()

	testCases := []struct {
		qname    string
		qtype    uint16
		expected bool
	}{
		{
			// message has no example.com record
			qname:    "example.com.",
			qtype:    dns.TypeA,
			expected: false,
		},
		{
			// message has no foo.example.org/A record (only question)
			qname:    "foo.example.org.",
			qtype:    dns.TypeA,
			expected: false,
		},
		{
			// message has a foo.example.org/CNAME record in Answer section
			qname:    "foo.example.org.",
			qtype:    dns.TypeCNAME,
			expected: true,
		},
		{
			// message has a bar.example.org/A record in Answer section
			qname:    "bar.example.org.",
			qtype:    dns.TypeA,
			expected: true,
		},
		{
			// message has no ns.example.org/SOA record
			qname:    "example.org.",
			qtype:    dns.TypeSOA,
			expected: false,
		},
		{
			// message has a example.org/NS record in Ns section
			qname:    "example.org.",
			qtype:    dns.TypeNS,
			expected: true,
		},
		{
			// message has a a.ns.example.org/NA record in Extra section
			qname:    "a.ns.example.org.",
			qtype:    dns.TypeA,
			expected: true,
		},
		{
			// FIXME: we may want to detect this use case.
			// message has a A.ns.example.org/NA record in Extra section
			qname:    "A.ns.example.org.",
			qtype:    dns.TypeA,
			expected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
			r := HasRecord(a, tc.qname, tc.qtype)
			require.Equalf(t, tc.expected, r, "Failed at finding if %s/%s is in answer %v", tc.qname, dns.TypeToString[tc.qtype], a)
		})
	}
}

// TestHasRecord test that we can properly detect if a qname/qtype tuple is in
// a dns message.
func TestAdditionalSectionForRecord(t *testing.T) {
	var db *DB
	var err error

	for _, tdb := range testaid.TestDBs {
		if db, err = Open(tdb.Path, tdb.Driver); err != nil {
			t.Fatalf("Could not open fixture database: %v", err)
		}
		reader, err := NewReader(db)
		if err != nil {
			t.Fatalf("Could not open db file: %v", err)
		}

		testCases := []struct {
			qname         string
			qtype         uint16
			rr            []dns.RR
			section       int
			locID         ID
			expectInExtra []dns.RR
		}{
			{ // When we have a NS record in Ns section, we search A/AAAA when missing.
				qname: "foo.example.com.",
				qtype: dns.TypeA,
				rr: []dns.RR{
					&dns.NS{
						Hdr: dns.RR_Header{
							Name:   "example.com.",
							Rrtype: dns.TypeNS,
							Class:  dns.ClassINET,
							Ttl:    172800,
						},
						Ns: "b.ns.example.com.",
					},
				},
				section: NsSection,
				locID:   []byte{0, 3},
				expectInExtra: []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "b.ns.example.com.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    172800,
						},
						A: net.ParseIP("5.5.6.5"),
					},
					&dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   "b.ns.example.com.",
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    172800,
						},
						AAAA: net.ParseIP("fd09:14f5:dead:beef:2::35"),
					},
				},
			},
			{ // When we have an MX record in Answer section, we search for A/AAAA when
				// missing.
				qname: "example.net.",
				qtype: dns.TypeMX,
				rr: []dns.RR{
					&dns.MX{
						Hdr: dns.RR_Header{
							Name:   "example.net.",
							Rrtype: dns.TypeMX,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						Mx:         "foo.example.net.",
						Preference: 30,
					},
				},
				section: AnswerSection,
				locID:   []byte{0, 3},
				expectInExtra: []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "foo.example.net.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    180,
						},
						A: net.ParseIP("1.1.1.3"),
					},
					&dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   "foo.example.net.",
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    180,
						},
						AAAA: net.ParseIP("fd24:7859:f076:2a21::3"),
					},
				},
			},
			{ // When we have a MX record in Answer section, we search for A/AAAA. If
				// all we have is a cname, we do not return anything.
				qname: "example.com.",
				qtype: dns.TypeMX,
				rr: []dns.RR{
					&dns.MX{
						Hdr: dns.RR_Header{
							Name:   "example.com.",
							Rrtype: dns.TypeMX,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						Mx:         "www.example.com.",
						Preference: 30,
					},
				},
				section:       AnswerSection,
				locID:         []byte{0, 3},
				expectInExtra: []dns.RR{},
			},
			{ // When we have a CNAME record in Answer section, AdditionalSectionForRecords
				// will not add any entries. (it only details with NS and MX)
				qname: "www.example.com.",
				qtype: dns.TypeA,
				rr: []dns.RR{
					&dns.CNAME{
						Hdr: dns.RR_Header{
							Name:   "www.example.com.",
							Rrtype: dns.TypeCNAME,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						Target: "www.nonauth.example.com.",
					},
				},
				section:       AnswerSection,
				locID:         []byte{0, 3},
				expectInExtra: []dns.RR{},
			},
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
				// section is a pointer to the section we are adding elements to as well as
				// testing AdditionalSectionForRecords on.
				var section *[]dns.RR
				a := answerSkeletonForAdditionalSectionTest(tc.qname, tc.qtype)

				switch tc.section {
				case NsSection:
					section = &a.Ns
				case AnswerSection:
					section = &a.Answer
				}

				*section = append(*section, tc.rr...)
				w := AdditionalSectionForRecords(reader, a, tc.locID, dns.ClassINET, *section)
				require.Falsef(t, w, "Did not expect a weighted result")
				RRSliceMatchSubsetf(t, a.Extra, tc.expectInExtra, "Failed at finding %s in answer %v", tc.expectInExtra, a)
			})
		}
	}
}
