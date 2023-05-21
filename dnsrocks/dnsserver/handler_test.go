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

package dnsserver

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/miekg/dns"
	newCopy "github.com/otiai10/copy"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/dns/dnsrocks/db"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/stats"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/test"
	"github.com/facebookincubator/dns/dnsrocks/testaid"
)

func TestMain(m *testing.M) {
	os.Exit(testaid.Run(m, "../testdata/data"))
}

func OpenDbForTesting(t *testing.T, db *testaid.TestDB) (th *FBDNSDB) {
	dbConfig := DBConfig{Path: db.Path, Driver: db.Driver, ReloadInterval: 10}
	cacheConfig := CacheConfig{Enabled: false}
	handlerConfig := HandlerConfig{}

	th, err := NewFBDNSDBBasic(handlerConfig, dbConfig, cacheConfig, &TextLogger{IoWriter: os.Stdout}, &stats.DummyStats{})
	if err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
	if err = th.Load(); err != nil {
		t.Fatalf("Failed to load DB: %v from %s", err, dbConfig.Path)
	}
	return
}

// RRSliceMatch matches the RR in a slice with specific order.
func RRSliceMatch(t *testing.T, expected, actual []dns.RR) {
	a := make([]string, 8)
	b := make([]string, 8)
	for _, x := range expected {
		a = append(a, x.String())
	}
	for _, x := range actual {
		b = append(b, x.String())
	}
	require.Equal(t, a, b)
}

// RRSliceMatchNoOrder matches the RR in a slice without specific order.
func RRSliceMatchNoOrder(t *testing.T, expected, actual []dns.RR) {
	a := make([]string, 8)
	b := make([]string, 8)
	for _, x := range expected {
		a = append(a, x.String())
	}
	for _, x := range actual {
		b = append(b, x.String())
	}
	require.ElementsMatch(t, a, b)
}

func CreateTestContext(maxAnswer int) context.Context {
	ctx := context.TODO()
	ctx = WithMaxAnswer(ctx, maxAnswer)
	return ctx
}

// TestFBDNSDBBadPathDontWrite confirms that when the CDB path does not exist,
// we don't do not write any messages back.
func TestFBDNSDBBadPathDontWrite(t *testing.T) {
	dbConfig := DBConfig{Path: "bad/path", Driver: "cdb", ReloadInterval: 10}
	cacheConfig := CacheConfig{Enabled: false}
	handlerConfig := HandlerConfig{}
	// Force the DB creation
	th, _ := NewFBDNSDB(handlerConfig, dbConfig, cacheConfig, &TextLogger{IoWriter: os.Stdout}, &stats.DummyStats{})
	err := th.Load()
	require.Error(t, err)
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	ctx := CreateTestContext(1)
	code, _ := th.ServeDNSWithRCODE(ctx, rec, req)
	require.Equal(t, dns.RcodeServerFailure, code, "RcodeServerFailure was expected to be returned.")
	require.False(t, w.GetWriteMsgCallCount() > 0, "WriteMsg was called")
}

// TestFBDNSDBBadDBDontWrite confirms that when the CDB is bad, we don't
// write any messages back.
func TestFBDNSDBBadDBDontWrite(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDBBad)

	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	ctx := CreateTestContext(1)
	code, err := th.ServeDNSWithRCODE(ctx, rec, req)
	require.Nil(t, err)
	require.Equal(t, dns.RcodeServerFailure, code, "RcodeServerFailure was expected to be returned")
	require.False(t, w.GetWriteMsgCallCount() > 0, "WriteMsg was called")
}

// TestDNSDBOldFindLocation checks locations
func TestDNSDBOldFindLocation(t *testing.T) {
	_separateBitMap := db.SeparateBitMap
	db.SeparateBitMap = false
	defer func() {
		db.SeparateBitMap = _separateBitMap
	}()

	t.Run("cdb !SeparateBitMap", func(t *testing.T) {
		testAnyDBOldFindLocation(t)
	})

	t.Run("rocksdb !SeparateBitMap", func(t *testing.T) {
		testAnyDBOldFindLocation(t)
	})
	db.SeparateBitMap = true
	t.Run("cdb SeparateBitMap", func(t *testing.T) {
		testAnyDBOldFindLocation(t)
	})
	t.Run("rocksdb SeparateBitMap", func(t *testing.T) {
		testAnyDBOldFindLocation(t)
	})
}

func testAnyDBOldFindLocation(t *testing.T) {
	testCases := []struct {
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply []string // ownernames for the records in the additional section.
		expectedErr   error
		ecs           string
	}{
		{
			qname:         "www.example.com.",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"www.nonauth.example.com."},
			expectedErr:   nil,
			ecs:           "1.1.1.0/24",
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for i, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, i), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
				if tc.ecs != "" {
					o, err := MakeOPTWithECS(tc.ecs)

					require.Nilf(t, err, "failed to generate ECS option for %s", tc.ecs)
					req.Extra = []dns.RR{o}
				}

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equalf(t, tc.expectedErr, err, "test %d unexpected error", i)
				require.Equalf(t, tc.expectedCode, code, "test %d unexpected code", i)
				require.Equalf(t, len(tc.expectedReply), len(rec.Msg.Answer), "test %d wrong replies length", i)

				for i, expected := range tc.expectedReply {
					actual := rec.Msg.Answer[i].(*dns.CNAME).Target
					require.Equalf(t, expected, actual, "test %d unexpected answer", i)
				}
			})
		}
	}
}

func getNs(domain string) []dns.RR {
	return []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			Ns: "a.ns." + domain,
		},
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			Ns: "b.ns." + domain,
		},
	}
}

func getExtra(domain string) []dns.RR {
	return []dns.RR{
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "a.ns." + domain,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			AAAA: net.ParseIP("fd09:14f5:dead:beef:1::35"),
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "a.ns." + domain,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			A: net.ParseIP("5.5.5.5"),
		},

		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "b.ns." + domain,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			AAAA: net.ParseIP("fd09:14f5:dead:beef:2::35"),
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "b.ns." + domain,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			A: net.ParseIP("5.5.6.5"),
		},
	}
}

func getExtraNonAuth() []dns.RR {
	return []dns.RR{
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "a.ns.nonauth.example.com.",
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			AAAA: net.ParseIP("fd09:24f5:dead:beef:1::35"),
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "a.ns.nonauth.example.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			A: net.ParseIP("6.5.5.5"),
		},

		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "b.ns.nonauth.example.com.",
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			AAAA: net.ParseIP("fd09:24f5:dead:beef:2::35"),
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "b.ns.nonauth.example.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    172800,
			},
			A: net.ParseIP("6.5.6.5"),
		},
	}
}

func makeSOA(domain string) []dns.RR {
	return []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    120,
			},
			Ns:      "a.ns." + domain,
			Mbox:    "dns." + domain,
			Serial:  123,
			Refresh: 7200,
			Retry:   1800,
			Expire:  604800,
			Minttl:  120,
		},
	}
}

// TestDNSDBRoundRobinRecord tests round robin rec
func TestDNSDBRoundRobinRecord(t *testing.T) {
	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()

		for nt, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn("wrr.example.com."), qtype)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)
				require.Equal(t, dns.RcodeSuccess, code)
				require.Nil(t, err)
				require.Equal(t, 1, len(rec.Msg.Answer), "expected exactly 1 record")

				require.Equal(t, 0, len(rec.Msg.Ns))
				require.Equal(t, 0, len(rec.Msg.Extra))
			})
		}
	}
}

// TestDNSDBMultipleQuestions checks the behaviour when a DNS request is
// receive and contains multiple Questions. While it is legitimate, very little
// implementations support it in practice.
// This test sets a baseline for us. On 1 hand, it makes sure that we do not
// crash, on the other hand, it allows us to confirm the behaviour and detect
// behaviour changes in the future.
// At the time of this writing, the behaviour is to only handle the first
// question in the list. The answer that we return contains only the first
// question in the question section.
func TestDNSDBMultipleQuestions(t *testing.T) {
	testCases := []struct {
		questions        []dns.Question // The questions RRSets in the request.
		expectedQuestion []dns.Question // The questions RRSets in the question section of the answer.
		expectedCode     int            // The expected Code of the answer.
		expectedAnswer   []dns.RR       // The expected Answer section.
		expectedAuth     []dns.RR       // The expected Authoritative section.
		expectedExtra    []dns.RR       // The expected Extra section.
	}{
		{
			questions: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
			},
		},
		{
			questions: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
			},
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.Question = tc.questions

				rec := dnstest.NewRecorder(&test.ResponseWriterCustomRemote{})
				ctx := CreateTestContext(1)
				code, _ := th.ServeDNSWithRCODE(ctx, rec, req)
				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)
				if rec.Msg != nil {
					require.Equal(t, tc.expectedQuestion, rec.Msg.Question)
					RRSliceMatch(t, tc.expectedAnswer, rec.Msg.Answer)
				}
			})
		}
	}
}

// We are trying to return up to 8 answer for whatsapp query.
// As a result, we are adding logic to support multiple answers.
// The below test is to verify we correctly return multiple answers.
func TestDNSDBMultipleAnswers1(t *testing.T) {
	// this test case will have max answer number more than
	// available record number
	testCases1 := []struct {
		qname            string
		qtype            uint16
		expectedCode     int
		answerListLength int
		expectedAnswer   []dns.RR
		expectedAuth     []dns.RR
		expectedExtra    []dns.RR
		expectedErr      error
		ecs              string
	}{
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 3,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.2"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.3"),
				},
			},
			expectedErr: nil,
		},
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeAAAA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 3,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("fd24:7859:f076:2a21::2"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "wrr.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("fd24:7859:f076:2a21::3"),
				},
			},
			expectedErr: nil,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for _, tc := range testCases1 {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(8)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)
				require.Equal(t, err, tc.expectedErr)
				require.NotNil(t, rec.Msg)
				require.NotNil(t, rec.Msg.Answer)
				require.Equalf(t, tc.answerListLength, len(rec.Msg.Answer), "expected answer length %d", tc.expectedCode)
				RRSliceMatchNoOrder(t, tc.expectedAnswer, rec.Msg.Answer)
				RRSliceMatch(t, tc.expectedAuth, rec.Msg.Ns)
				RRSliceMatch(t, tc.expectedExtra, rec.Msg.Extra)
			})
		}
	}
}

func TestDNSDBMultipleAnswers2(t *testing.T) {
	// this test case will have max answer number less than
	// available record number
	testCases2 := []struct {
		qname            string
		qtype            uint16
		expectedCode     int
		answerListLength int
		expectedAuth     []dns.RR
		expectedExtra    []dns.RR
		expectedErr      error
		ecs              string
	}{
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 2,
			expectedErr:      nil,
		},
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeAAAA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 2,
			expectedErr:      nil,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for _, tc := range testCases2 {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(2)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)
				require.Equal(t, err, tc.expectedErr)
				require.NotNil(t, rec.Msg)
				require.NotNil(t, rec.Msg.Answer)
				require.Equalf(t, tc.answerListLength, len(rec.Msg.Answer), "expected answer length %d", tc.expectedCode)
				RRSliceMatch(t, tc.expectedAuth, rec.Msg.Ns)
				RRSliceMatch(t, tc.expectedExtra, rec.Msg.Extra)
			})
		}
	}
}

func TestDNSDBWithoutContextOfMaxAnswer(t *testing.T) {
	// this test case expect 1 answer for not setting max answer context
	testCases := []struct {
		qname            string
		qtype            uint16
		expectedCode     int
		answerListLength int
		expectedAuth     []dns.RR
		expectedExtra    []dns.RR
		expectedErr      error
		ecs              string
	}{
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 1,
			expectedErr:      nil,
		},
		{
			qname:            "wrr.example.com.",
			qtype:            dns.TypeAAAA,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 1,
			expectedErr:      nil,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})

				code, err := th.ServeDNSWithRCODE(context.TODO(), rec, req)

				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)
				require.Equal(t, err, tc.expectedErr)
				require.NotNil(t, rec.Msg)
				require.NotNil(t, rec.Msg.Answer)
				require.Equalf(t, tc.answerListLength, len(rec.Msg.Answer), "expected answer length %d", tc.expectedCode)
				RRSliceMatch(t, tc.expectedAuth, rec.Msg.Ns)
				RRSliceMatch(t, tc.expectedExtra, rec.Msg.Extra)
			})
		}
	}
}

func TestDNSDBForHTTPSRecord(t *testing.T) {
	// this test case expect 1 answer for not setting max answer context
	testCases := []struct {
		qname            string
		qtype            uint16
		expectedCode     int
		answerListLength int
		expectedErr      error
		expectedAnswer   []dns.RR
		expectedAuth     []dns.RR
		expectedExtra    []dns.RR
	}{
		{
			qname:            "foo.example.com.",
			qtype:            dns.TypeHTTPS,
			expectedCode:     dns.RcodeSuccess,
			answerListLength: 2,
			expectedErr:      nil,
			expectedAnswer: []dns.RR{
				&dns.HTTPS{
					SVCB: dns.SVCB{
						Hdr: dns.RR_Header{
							Name:   "foo.example.com.",
							Rrtype: dns.TypeHTTPS,
							Class:  dns.ClassINET,
							Ttl:    7200,
						},
						Priority: 1,
						Target:   ".",
						Value: []dns.SVCBKeyValue{
							&dns.SVCBAlpn{
								Alpn: []string{"h3", "h2", "http/1.1"},
							},
						},
					},
				},
				&dns.HTTPS{
					SVCB: dns.SVCB{
						Hdr: dns.RR_Header{
							Name:   "foo.example.com.",
							Rrtype: dns.TypeHTTPS,
							Class:  dns.ClassINET,
							Ttl:    7200,
						},
						Priority: 2,
						Target:   "fallback.foo.example.com.",
						Value: []dns.SVCBKeyValue{
							&dns.SVCBAlpn{
								Alpn: []string{"h3", "h2", "http/1.1"},
							},
						},
					},
				},
			},
			expectedAuth: nil,
			expectedExtra: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
			},
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
				rec := dnstest.NewRecorder(&test.ResponseWriter{})

				// context TODO does not provide value to limit or specify max answers
				code, err := th.ServeDNSWithRCODE(context.TODO(), rec, req)

				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)
				require.Equal(t, err, tc.expectedErr)
				require.NotNil(t, rec.Msg)
				require.NotNil(t, rec.Msg.Answer)
				require.Equalf(t, tc.answerListLength, len(rec.Msg.Answer), "expected answer length %d", tc.answerListLength)
				RRSliceMatch(t, tc.expectedAuth, rec.Msg.Ns)
				RRSliceMatch(t, tc.expectedExtra, rec.Msg.Extra)
				RRSliceMatchNoOrder(t, tc.expectedAnswer, rec.Msg.Answer)
			})
		}
	}
}

func TestResolverBasedResponse(t *testing.T) {
	testCases := []struct {
		qname          string
		qtype          uint16
		expectedCode   int
		expectedAnswer []dns.RR
		resolver       string
	}{
		{
			qname:        "cnamemap.example.org.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "cnamemap.example.org.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "foo.example.org.",
				},
			},
			resolver: "1.1.1.1", // resolver for locID 2
		},
		{
			qname:        "cnamemap.example.org.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "cnamemap.example.org.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "bar.example.org.",
				},
			},
			resolver: "1.1.0.0", // resolver for locID 1 (default)
		},
		{
			qname:        "cnamemap.example.org.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "cnamemap.example.org.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "foo.example.org.",
				},
			},
			resolver: "fd48:6525:66bd::1", // resolver for locID 5
		},
		{
			qname:        "cnamemap.example.org.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "cnamemap.example.org.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "bar.example.org.",
				},
			},
			resolver: "::1", // resolver for locID 1 (default)
		},
		{
			qname:        "nonlocationawarewithmap.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "nonlocationawarewithmap.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					Target: "1.1.1.10",
				},
			},
			resolver: "1.1.1.5", // resolver for locID 1 (default)
		},
		{
			qname:        "nonlocationawarewithmap.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "nonlocationawarewithmap.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					Target: "1.1.1.10",
				},
			},
			resolver: "2.2.2.2", // resolver for locID 3
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%v", db.Driver, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

				rec := dnstest.NewRecorder(&test.ResponseWriterCustomRemote{RemoteIP: tc.resolver})
				ctx := CreateTestContext(1)
				code, _ := th.ServeDNSWithRCODE(ctx, rec, req)
				require.Equalf(t, tc.expectedCode, code, "expected status code %d", tc.expectedCode)

				if tc.expectedAnswer != nil {
					require.NotNil(t, rec.Msg)
					require.NotNil(t, rec.Msg.Answer)
					RRSliceMatch(t, tc.expectedAnswer, rec.Msg.Answer)
				}
			})
		}
	}
}

func TestDNSDB(t *testing.T) {
	testCases := []struct {
		qname          string
		qtype          uint16
		expectedCode   int
		expectedAnswer []dns.RR
		expectedAuth   []dns.RR
		expectedExtra  []dns.RR
		expectedErr    error
		ecs            string
	}{
		{
			qname:        "www.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "www.example.com.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "www.nonauth.example.com.",
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		// Handling of DS RR
		{
			qname:          "nonauth.example.com.",
			qtype:          dns.TypeDS,
			expectedCode:   dns.RcodeSuccess,
			expectedAuth:   makeSOA("example.com."),
			expectedErr:    nil,
			expectedAnswer: nil,
			expectedExtra:  nil,
		},
		// Handling of DS RR noerror/nodata
		{
			qname:          "example.com.",
			qtype:          dns.TypeDS,
			expectedCode:   dns.RcodeSuccess,
			expectedAuth:   makeSOA("example.com."),
			expectedExtra:  nil,
			expectedErr:    nil,
			expectedAnswer: nil,
		},
		{
			qname:        "foo.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		{
			qname:        "foo.example.com.",
			qtype:        dns.TypeAAAA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		// When querying for example.com/NS, we expect the NS RRset in the Answer
		// section and optionally the A/AAAA records in Additional section.
		{
			qname:        "example.com.",
			qtype:        dns.TypeNS,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.NS{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeNS,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					Ns: "a.ns.example.com.",
				},
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
			expectedExtra: getExtra("example.com."),
			expectedErr:   nil,
		},
		// NOERROR, non-location aware RR with an associated map
		{
			qname:        "nonlocationawarewithmap.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "nonlocationawarewithmap.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.10"),
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		{
			qname:        "nonlocationawarewithmap.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "nonlocationawarewithmap.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.10"),
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		// NOERROR/NODATA
		{
			qname:          "bar.example.com.",
			qtype:          dns.TypeTXT,
			expectedCode:   dns.RcodeSuccess,
			expectedAnswer: nil,
			expectedAuth:   makeSOA("example.com."),
			expectedExtra:  nil,
			expectedErr:    nil,
		},
		{
			qname:        "www.notourdomain.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeRefused,
			expectedErr:  nil,
		},
		// Test wildcard lookups
		{
			qname:        "wildcard.test.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "wildcard.test.example.com.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    1800,
					},
					Target: "some-other.domain.",
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
		},
		// Test bad wildcard label (see i umlaut). Will return NXDOMAIN.
		{
			qname:        "badwïldcard.test.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeNameError,
			expectedAuth: makeSOA("example.com."),
		},
		{
			qname:        "www.nonauth.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAuth: getNs("nonauth.example.com."),
			expectedExtra: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "a.ns.nonauth.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					AAAA: net.ParseIP("fd09:24f5:dead:beef:1::35"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "a.ns.nonauth.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					A: net.ParseIP("6.5.5.5"),
				},

				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "b.ns.nonauth.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					AAAA: net.ParseIP("fd09:24f5:dead:beef:2::35"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "b.ns.nonauth.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					A: net.ParseIP("6.5.6.5"),
				},
			},
			expectedErr: nil,
		},
		// www.nonauth.example.com./NS: nodata/noerror. authority section
		// has NS RRset. Additional section has A/AAAA RRsets.
		{
			qname:        "www.nonauth.example.com.",
			qtype:        dns.TypeNS,
			expectedCode: dns.RcodeSuccess,
			expectedAuth: getNs("nonauth.example.com."),
			expectedExtra: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "a.ns.nonauth.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					AAAA: net.ParseIP("fd09:24f5:dead:beef:1::35"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "a.ns.nonauth.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					A: net.ParseIP("6.5.5.5"),
				},

				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "b.ns.nonauth.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					AAAA: net.ParseIP("fd09:24f5:dead:beef:2::35"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "b.ns.nonauth.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    172800,
					},
					A: net.ParseIP("6.5.6.5"),
				},
			},
			expectedErr: nil,
		},
		// NXDOMAIN
		{
			qname:        "nxdomain.example.org.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeNameError,
			expectedAuth: makeSOA("example.org."),
		},
		// Single MX to CNAME
		{
			qname:        "example.com.",
			qtype:        dns.TypeMX,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.MX{
					Hdr: dns.RR_Header{
						Name:   "example.com.",
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Preference: 10,
					Mx:         "www.example.com.",
				},
			},
			expectedAuth:  []dns.RR{},
			expectedExtra: []dns.RR{},
			expectedErr:   nil,
		},
		// Dual MX, 1 CNAME, 1 A/AAAA + location aware
		{
			qname:        "example.net.",
			qtype:        dns.TypeMX,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.MX{
					Hdr: dns.RR_Header{
						Name:   "example.net.",
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Preference: 10,
					Mx:         "www.example.net.",
				},
				&dns.MX{
					Hdr: dns.RR_Header{
						Name:   "example.net.",
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Preference: 30,
					Mx:         "foo.example.net.",
				},
			},
			expectedAuth: []dns.RR{},
			expectedExtra: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.net.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.net.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
			},
			expectedErr: nil,
		},
		// Dual MX, with location
		{
			qname:        "example.net.",
			qtype:        dns.TypeMX,
			expectedCode: dns.RcodeSuccess,
			ecs:          "1.1.1.0/24",
			expectedAnswer: []dns.RR{
				&dns.MX{
					Hdr: dns.RR_Header{
						Name:   "example.net.",
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Preference: 10,
					Mx:         "www.example.net.",
				},
				&dns.MX{
					Hdr: dns.RR_Header{
						Name:   "example.net.",
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					Preference: 30,
					Mx:         "foo.example.net.",
				},
			},
			expectedAuth: []dns.RR{},
			expectedExtra: append([]dns.RR{
				&dns.OPT{
					Hdr: dns.RR_Header{
						Name:   ".",
						Rrtype: dns.TypeOPT,
					},
					Option: []dns.EDNS0{
						&dns.EDNS0_SUBNET{
							Code:          dns.EDNS0SUBNET,
							Family:        1,
							Address:       net.ParseIP("1.1.1.0").To4(),
							SourceNetmask: 24,
							SourceScope:   24,
						},
					},
				},
			},
				[]dns.RR{
					&dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   "foo.example.net.",
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    180,
						},
						AAAA: net.ParseIP("fd24:7859:f076:2a21::2"),
					},
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "foo.example.net.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    180,
						},
						A: net.ParseIP("1.1.1.2"),
					},
				}...),
			expectedErr: nil,
		},
		// ANY request
		{
			qname:        "foo.example.com.",
			qtype:        dns.TypeANY,
			expectedCode: dns.RcodeSuccess,
			ecs:          "1.1.1.0/24",
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.2"),
				},
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::2"),
				},
				&dns.HTTPS{
					SVCB: dns.SVCB{
						Hdr: dns.RR_Header{
							Name:   "foo.example.com.",
							Rrtype: dns.TypeHTTPS,
							Class:  dns.ClassINET,
							Ttl:    7200,
						},
						Priority: 1,
						Target:   ".",
						Value: []dns.SVCBKeyValue{
							&dns.SVCBAlpn{
								Alpn: []string{"h3", "h2", "http/1.1"},
							},
						},
					},
				},
				&dns.HTTPS{
					SVCB: dns.SVCB{
						Hdr: dns.RR_Header{
							Name:   "foo.example.com.",
							Rrtype: dns.TypeHTTPS,
							Class:  dns.ClassINET,
							Ttl:    7200,
						},
						Priority: 2,
						Target:   "fallback.foo.example.com.",
						Value: []dns.SVCBKeyValue{
							&dns.SVCBAlpn{
								Alpn: []string{"h3", "h2", "http/1.1"},
							},
						},
					},
				},
			},
			expectedAuth: []dns.RR{},
			expectedExtra: []dns.RR{
				&dns.OPT{
					Hdr: dns.RR_Header{
						Name:   ".",
						Rrtype: dns.TypeOPT,
					},
					Option: []dns.EDNS0{
						&dns.EDNS0_SUBNET{
							Code:          dns.EDNS0SUBNET,
							Family:        1,
							Address:       net.ParseIP("1.1.1.0").To4(),
							SourceNetmask: 24,
							SourceScope:   24,
						},
					},
				},
			},
			expectedErr: nil,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
				if tc.ecs != "" {
					o, err := MakeOPTWithECS(tc.ecs)

					require.Nilf(t, err, "failed to generate ECS option for %s", tc.ecs)
					req.Extra = append(req.Extra, []dns.RR{o}...)
				}
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equal(t, tc.expectedErr, err)
				require.Equal(t, tc.expectedCode, code)

				if tc.expectedAnswer != nil {
					if len(tc.expectedAnswer) != 0 {
						RRSliceMatchNoOrder(t, tc.expectedAnswer, rec.Msg.Answer)
					}
				}
				if tc.expectedAuth != nil {
					if len(tc.expectedAuth) != 0 {
						RRSliceMatchNoOrder(t, tc.expectedAuth, rec.Msg.Ns)
					}
				}
				if tc.expectedExtra != nil {
					if len(tc.expectedExtra) != 0 {
						RRSliceMatchNoOrder(t, tc.expectedExtra, rec.Msg.Extra)
					}
				}
			})
		}
	}
}

func TestTypeToStatsKey(t *testing.T) {
	testCases := []struct {
		keyName string
		qType   uint16
	}{
		// This is a test that is suppose to hit the cached entry.
		{
			keyName: "DNS_query.A",
			qType:   dns.TypeA,
		},
		// This is a nonexistent type, will default to DNS_query.TYPE%d
		{
			keyName: "DNS_query.TYPE12345",
			qType:   12345,
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
			require.Equal(t, tc.keyName, typeToStatsKey(tc.qType))
		})
	}
}

func TestEDNSComplianceUnknownVersion(t *testing.T) {
	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		t.Run(db.Driver, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn("example.com."), dns.TypeSOA)

			rec := dnstest.NewRecorder(&test.ResponseWriter{})

			o := new(dns.OPT)
			o.Hdr.Name = "."
			o.Hdr.Rrtype = dns.TypeOPT
			o.SetVersion(1)
			req.Extra = []dns.RR{o}
			ctx := CreateTestContext(1)
			code, err := th.ServeDNSWithRCODE(ctx, rec, req)

			require.Nil(t, err)

			// First pass, check extended rcode is not set....
			opt := rec.Msg.IsEdns0()
			require.NotNil(t, opt)
			require.Equal(t, int(opt.Hdr.Ttl&0xFF000000>>24)<<4, 0)

			// Pack... to force actually setting the extended code. This is lame, but
			// SetExtendedRcode() gets called upon calling Pack()... which is not called
			// within the unit tests.
			_, err = rec.Msg.Pack()
			require.NoError(t, err)

			// Second pass, this time it should be set.
			opt = rec.Msg.IsEdns0()
			require.NotNil(t, opt)
			require.Equal(t, int(opt.Hdr.Ttl&0xFF000000>>24)<<4, dns.RcodeBadVers&0xFFFFFFF0)

			require.Equalf(t, dns.RcodeBadVers, code, "Expected BADVERS/BADSIG, got %s", dns.RcodeToString[code])
			o = rec.Msg.IsEdns0()
			require.Equalf(t, uint8(0), o.Version(), "Expected EDNS0 version to be set to 0, got %d", o.Version())

			require.Equalf(t, 0, len(rec.Msg.Answer), "Expected no answer set. Got %v", rec.Msg.Answer)
		})
	}
}

// TestPackExtendedBadCookie is taken from github.com/miekg/dns
// duplicating it here to make sure we catch this in the future on dependency
// update and don't use BADVERS which may have gone unnoticed until GH#791
func TestPackExtendedBadCookie(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com."), dns.TypeNS)
	a := new(dns.Msg)
	a.SetReply(m)
	o := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
	}
	o.SetUDPSize(dns.DefaultMsgSize)
	a.Extra = append(a.Extra, o)
	a.SetRcode(m, dns.RcodeBadCookie)
	edns0 := a.IsEdns0()
	if edns0 == nil {
		t.Fatal("Expected OPT RR")
	}
	// SetExtendedRcode is only called as part of `Pack()`, hence at this stage,
	// the OPT RR is not set yet.
	if edns0.ExtendedRcode() == dns.RcodeBadCookie&0xFFFFFFF0 {
		t.Errorf("ExtendedRcode is expected to not be BADCOOKIE before Pack")
	}
	_, err := a.Pack()
	require.NoError(t, err)
	edns0 = a.IsEdns0()
	if edns0 == nil {
		t.Fatal("Expected OPT RR")
	}
	if edns0.ExtendedRcode() != dns.RcodeBadCookie&0xFFFFFFF0 {
		t.Errorf("ExtendedRcode is expected to be BADCOOKIE after Pack")
	}
}

// TestEDNSComplianceUnknownOption tests that unknown options are not echoed
// back whether or not EDNS option is known.
func TestEDNSComplianceUnknownOption(t *testing.T) {
	testCases := []struct {
		ednsVersion         uint8
		expectedEDNSVersion uint8
		expectedRcode       int
		optionListLength    int
		answerListLength    int
	}{
		{
			ednsVersion:         0,
			expectedEDNSVersion: 0,                // EDNS0
			expectedRcode:       dns.RcodeSuccess, // no failure
			optionListLength:    0,                // do not echo EDNS0_LOCAL
			answerListLength:    1,                // SOA
		},
		{
			ednsVersion:         1,
			expectedEDNSVersion: 0,                // highest level implemented by server rfc6891#section-6.1.3
			expectedRcode:       dns.RcodeBadVers, // BADVERS
			optionListLength:    0,                // do not echo EDNS0_LOCAL
			answerListLength:    0,                // nothing returned
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d/%v", db.Driver, nt, tc), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn("example.com."), dns.TypeSOA)
				rec := dnstest.NewRecorder(&test.ResponseWriter{})

				o := new(dns.OPT)
				o.Hdr.Name = "."
				o.Hdr.Rrtype = dns.TypeOPT
				o.SetVersion(tc.ednsVersion)
				e := new(dns.EDNS0_LOCAL)
				e.Code = dns.EDNS0LOCALSTART
				o.Option = append(o.Option, e)
				req.Extra = []dns.RR{o}
				ctx := CreateTestContext(1)
				code, _ := th.ServeDNSWithRCODE(ctx, rec, req)
				require.Equalf(t, tc.expectedRcode, code, "Wrong RCODE, got %s, expected %s", dns.RcodeToString[code], dns.RcodeToString[tc.expectedRcode])
				o = rec.Msg.IsEdns0()
				require.Equal(t, tc.expectedEDNSVersion, o.Version(), "Wrong EDNS0 version")
				require.Equalf(t, tc.optionListLength, len(o.Option), "Wrong EDNS option list size. got: %v", o.Option)
				require.Equalf(t, tc.answerListLength, len(rec.Msg.Answer), "Expected no answer set. Got %v", rec.Msg.Answer)
			})
		}
	}
}

func createFBDNSDBWithCache(t *testing.T, counters stats.Stats) (th *FBDNSDB) {
	db := testaid.TestCDB
	dbConfig := DBConfig{Path: db.Path, Driver: db.Driver, ReloadInterval: 10}
	cacheConfig := CacheConfig{Enabled: true, LRUSize: 1024}
	handlerConfig := HandlerConfig{}

	th, err := NewFBDNSDB(handlerConfig, dbConfig, cacheConfig, &TextLogger{IoWriter: os.Stdout}, counters)
	require.Nil(t, err, "Failed to initialize CDB")

	err = th.Load()
	require.Nil(t, err, "Failed to load CDB")
	return
}

// TestHandlerCache tests that we exercise the caching path when caching is
// enabled.
func TestHandlerCache(t *testing.T) {
	ctr := stats.NewCounters()
	th := createFBDNSDBWithCache(t, ctr)
	require.NotZero(t, ctr["DNS_db.reload"])

	iterations := 10

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	for i := 0; i < iterations; i++ {
		ctr.ResetCounter("DNS_queries")
		ctr.ResetCounter("DNS_query.A")
		ctr.ResetCounter("DNS_location.resolver")
		ctr.ResetCounter("DNS_location.empty")
		ctr.ResetCounter("DNS_response.authoritative")
		ctr.ResetCounter("DNS_cache.missed")
		ctr.ResetCounter("DNS_cache.hit")

		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn("www.example.com."), dns.TypeA)
		ctx := CreateTestContext(1)
		rcode, err := th.ServeDNSWithRCODE(ctx, rec, req)
		require.NoError(t, err)
		require.Equal(t, rcode, dns.RcodeSuccess)

		require.NotZero(t, ctr["DNS_queries"])
		require.NotZero(t, ctr["DNS_query.A"])
		require.Zero(t, ctr["DNS_location.resolver"])
		require.NotZero(t, ctr["DNS_location.empty"])
		if i == 0 {
			// First query is a miss and we perform the resolution
			require.NotZero(t, ctr["DNS_cache.missed"])
			require.NotZero(t, ctr["DNS_response.authoritative"])
		} else {
			require.NotZero(t, ctr["DNS_cache.hit"])
		}
	}
}

// TestHandlerNoCache tests that we DO NOT exercise the caching path when
// caching is disabled.
func TestHandlerNoCache(t *testing.T) {
	ctr := stats.NewCounters()
	th := createFBDNSDBWithCache(t, ctr)
	require.NotZero(t, ctr["DNS_db.reload"])
	// disable cache
	th.cacheConfig.Enabled = false
	iterations := 10

	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	for i := 0; i < iterations; i++ {
		ctr.ResetCounter("DNS_queries")
		ctr.ResetCounter("DNS_query.A")
		ctr.ResetCounter("DNS_location.resolver")
		ctr.ResetCounter("DNS_location.empty")
		ctr.ResetCounter("DNS_response.authoritative")

		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn("www.example.com."), dns.TypeA)
		ctx := CreateTestContext(1)
		rcode, err := th.ServeDNSWithRCODE(ctx, rec, req)
		require.Equal(t, rcode, dns.RcodeSuccess)
		require.NoError(t, err)

		require.NotZero(t, ctr["DNS_queries"])
		require.NotZero(t, ctr["DNS_query.A"])
		require.Zero(t, ctr["DNS_location.resolver"])
		require.NotZero(t, ctr["DNS_location.empty"])
		require.NotZero(t, ctr["DNS_response.authoritative"])
		require.Zero(t, ctr["DNS_cache.hit"])
	}
}

func TestReloadPartial(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestRDB)
	ctr := stats.NewCounters()
	th.stats = ctr
	th.dbConfig.ReloadTimeout = 10 * time.Second
	signal := NewPartialReloadSignal()
	err := th.Reload(*signal)
	require.Nil(t, err)
	require.NotZero(t, ctr["DNS_db.reload"])
	require.Zero(t, ctr["DNS_db.ErrReloadTimeout"])
}

func TestReloadFull(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestRDB)
	ctr := stats.NewCounters()
	th.stats = ctr
	th.dbConfig.ReloadTimeout = 10 * time.Second
	rdbDir, err := os.MkdirTemp("", "reload-test")
	require.Nil(t, err)
	defer os.RemoveAll(rdbDir)
	// copy existing rdb to new path, so we can switch to it
	err = newCopy.Copy(
		testaid.TestRDB.Path,
		rdbDir,
	)
	require.Nil(t, err)
	signal := NewFullReloadSignal(rdbDir)
	err = th.Reload(*signal)
	require.Nil(t, err)
	require.NotZero(t, ctr["DNS_db.reload"])
	require.Zero(t, ctr["DNS_db.ErrReloadTimeout"])
}

func TestReloadFullTimeoutRDB(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestRDB)
	ctr := stats.NewCounters()
	th.stats = ctr
	// setting reload timeout to 0 to simulate timeout
	th.dbConfig.ReloadTimeout = 0 * time.Millisecond
	rdbDir, err := os.MkdirTemp("", "reload-test")
	require.Nil(t, err)
	defer os.RemoveAll(rdbDir)
	// we want directory to exist
	signal := NewFullReloadSignal(rdbDir)
	err = th.Reload(*signal)
	require.NotNil(t, err)
	require.Zero(t, ctr["DNS_db.reload"])
	require.NotZero(t, ctr["DNS_db.ErrReloadTimeout"])
}

func TestReloadFullTimeoutCDB(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDB)
	ctr := stats.NewCounters()
	th.stats = ctr
	th.dbConfig.ReloadTimeout = 0 * time.Millisecond
	signal := NewFullReloadSignal(testaid.TestCDBBad.Path)
	err := th.Reload(*signal)
	require.NotNil(t, err)
	require.Zero(t, ctr["DNS_db.reload"])
	require.NotZero(t, ctr["DNS_db.ErrReloadTimeout"])
}

func TestReloadFullBroken(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDB)
	ctr := stats.NewCounters()
	th.stats = ctr
	th.dbConfig.ReloadTimeout = 10 * time.Second
	th.dbConfig.ValidationKey = []byte(`\000\001\003www\010facebook\003com\000`)
	// this should never pass key validation test
	signal := NewFullReloadSignal(testaid.TestCDBBad.Path)
	err := th.Reload(*signal)
	require.NotNil(t, err)
	require.Zero(t, ctr["DNS_db.reload"])
	require.Zero(t, ctr["DNS_db.ErrReloadTimeout"])
}

func TestWatchDBAndReload(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDB)
	watcher, err := prepareDBWatcher(path.Dir(th.dbConfig.Path))
	if watcher != nil {
		defer watcher.Close()
	}
	require.NoError(t, err)
	go func() {
		err := th.watchDBAndReload(watcher)
		require.NoError(t, err)
	}()

	// Simulate touch of the file
	// mtime has nanoseconds precision, therefore we need to have this sleep
	// See http://man7.org/linux/man-pages/man2/stat.2.html
	time.Sleep(1 * time.Millisecond)
	currenttime := time.Now()
	err = os.Chtimes(testaid.TestCDB.Path, currenttime, currenttime)
	require.NoError(t, err)

	select {
	case reload := <-th.ReloadChan:
		expectedSignal := *NewPartialReloadSignal()
		require.Equal(t, expectedSignal, reload)
	case <-time.After(2 * time.Second):
		t.Errorf("Expected to receive PartialReloadSignal in ReloadChan, but did not")
	}
}

func TestWatchControlDirAndReloadPartial(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDB)
	ctlDir, err := os.MkdirTemp("", "ctl-test")
	if err != nil {
		require.Nil(t, err)
	}
	defer os.RemoveAll(ctlDir)
	th.dbConfig.ControlPath = ctlDir
	watcher, err := prepareDBWatcher(th.dbConfig.ControlPath)
	if watcher != nil {
		defer watcher.Close()
	}
	require.NoError(t, err)
	go func() {
		err := th.watchControlDirAndReload(watcher)
		require.NoError(t, err)
	}()

	// Simulate touch of the file
	time.Sleep(1 * time.Millisecond)
	filePath := path.Join(ctlDir, ControlFilePartialReload)
	emptyFile, err := os.Create(filePath)
	require.Nil(t, err)
	emptyFile.Close()

	select {
	case reload := <-th.ReloadChan:
		expectedSignal := *NewPartialReloadSignal()
		require.Equal(t, expectedSignal, reload)
	case <-time.After(2 * time.Second):
		t.Errorf("Expected to receive PartialReloadSignal in ReloadChan, but did not")
	}
}

func TestWatchControlDirAndReloadFull(t *testing.T) {
	th := OpenDbForTesting(t, &testaid.TestCDB)
	ctlDir, err := os.MkdirTemp("", "ctl-test")
	if err != nil {
		require.Nil(t, err)
	}
	defer os.RemoveAll(ctlDir)
	th.dbConfig.ControlPath = ctlDir
	watcher, err := prepareDBWatcher(th.dbConfig.ControlPath)
	if watcher != nil {
		defer watcher.Close()
	}
	require.NoError(t, err)
	go func() {
		err := th.watchControlDirAndReload(watcher)
		require.NoError(t, err)
	}()
	newPath, err := os.MkdirTemp("", "ctl-test-newdir")
	if err != nil {
		require.Nil(t, err)
	}
	defer os.RemoveAll(newPath)

	// Simulate touch of the file
	time.Sleep(1 * time.Millisecond)
	filePathTmp := path.Join(ctlDir, "."+ControlFileFullReload)
	filePath := path.Join(ctlDir, ControlFileFullReload)
	// write temp file, move it to proper path already with the content
	err = os.WriteFile(filePathTmp, []byte(newPath), 0644)
	require.Nil(t, err)
	err = os.Rename(filePathTmp, filePath)
	require.Nil(t, err)

	select {
	case reload := <-th.ReloadChan:
		expectedSignal := *NewFullReloadSignal(newPath)
		require.Equal(t, expectedSignal, reload)
	case <-time.After(2 * time.Second):
		t.Errorf("Expected to receive FullReloadSignal in ReloadChan, but did not")
	}
}

func TestDNSDBMinimalNSInAuth(t *testing.T) {
	testCases := []struct {
		qname          string
		qtype          uint16
		expectedCode   int
		expectedAnswer []dns.RR
		expectedAuth   []dns.RR
		expectedExtra  []dns.RR
		expectedErr    error
		ecs            string
	}{
		{
			qname:        "www.example.com.",
			qtype:        dns.TypeA,
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   "www.example.com.",
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Target: "www.nonauth.example.com.",
				},
			},
			expectedErr: nil,
		},
		// A/www.nonauth is handled by delegation nonauth.example.com
		{
			qname:         "www.nonauth.example.com.",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedAuth:  getNs("nonauth.example.com."),
			expectedExtra: getExtraNonAuth(),
		},
		// NS/nonauth is handled by delegation nonauth.example.com
		{
			qname:         "nonauth.example.com.",
			qtype:         dns.TypeNS,
			expectedCode:  dns.RcodeSuccess,
			expectedAuth:  getNs("nonauth.example.com."),
			expectedExtra: getExtraNonAuth(),
		},
		// DS/nonauth is the parent, we should not send delegation info
		{
			qname:        "nonauth.example.com.",
			qtype:        dns.TypeDS,
			expectedCode: dns.RcodeSuccess,
			expectedAuth: makeSOA("example.com."),
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()

		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)
				if tc.ecs != "" {
					o, err := MakeOPTWithECS(tc.ecs)

					require.Nilf(t, err, "failed to generate ECS option for %s", tc.ecs)
					req.Extra = append(req.Extra, []dns.RR{o}...)
				}
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equal(t, tc.expectedErr, err)
				require.Equal(t, tc.expectedCode, code)

				// Answer
				require.Equal(t, len(tc.expectedAnswer), len(rec.Msg.Answer), "Answer length")
				RRSliceMatch(t, rec.Msg.Answer, tc.expectedAnswer)

				// Auth
				require.Equal(t, len(tc.expectedAuth), len(rec.Msg.Ns), "Auth length")
				RRSliceMatchNoOrder(t, rec.Msg.Ns, tc.expectedAuth)
				// Extra
				require.Equal(t, len(tc.expectedExtra), len(rec.Msg.Extra), "Additional length")
				RRSliceMatchNoOrder(t, rec.Msg.Extra, tc.expectedExtra)
			})
		}
	}
}

// popEdns0 is like IsEdns0, but it removes the record from the message.
func popEdns0(msg *dns.Msg) *dns.OPT {
	// RFC 6891, Section 6.1.1 allows the OPT record to appear
	// anywhere in the additional record section, but it's usually at
	// the end so start there.
	for i := len(msg.Extra) - 1; i >= 0; i-- {
		if msg.Extra[i].Header().Rrtype == dns.TypeOPT {
			opt := msg.Extra[i].(*dns.OPT)
			msg.Extra = append(msg.Extra[:i], msg.Extra[i+1:]...)
			return opt
		}
	}
	return nil
}

func TestLotOfAdditionalAndTruncation(t *testing.T) {
	testCases := []struct {
		bufsize   uint16
		truncated bool
	}{
		{
			bufsize:   512,
			truncated: true,
		},
		{
			bufsize:   2048,
			truncated: false,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()

		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetQuestion(dns.Fqdn("lotofns.example.org."), dns.TypeNS)
				req.SetEdns0(tc.bufsize, true)

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equal(t, nil, err)
				require.Equal(t, dns.RcodeSuccess, code)
				// Remove EDNS to compare number of NS vs number of additional
				popEdns0(rec.Msg)
				if tc.truncated {
					require.Greater(t, len(rec.Msg.Ns), (len(rec.Msg.Extra))/2)
				} else {
					require.Equal(t, len(rec.Msg.Ns), (len(rec.Msg.Extra))/2)
				}
				require.Equal(t, tc.truncated, rec.Msg.Truncated)
			})
		}
	}
}

func TestDNSDBQuerySingle(t *testing.T) {
	testCases := []struct {
		record           string         // DNS record we want
		rtype            string         // Record type as a string (i.e. AAAA)
		from             string         // IP address as a string (i.e. 127.0.0.1)
		expectedQuestion []dns.Question // The questions RRSets in the question section of the answer.
		expectedCode     int            // The expected Code of the answer.
		expectedAnswer   []dns.RR       // The expected Answer section.
		expectedAuth     []dns.RR       // The expected Authoritative section.
		expectedExtra    []dns.RR       // The expected Extra section.
	}{
		{
			record: "foo.example.com.",
			rtype:  "A",
			from:   "127.0.0.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("1.1.1.1"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "AAAA",
			from:   "127.0.0.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("fd24:7859:f076:2a21::1"),
				},
			},
		},
		{
			record: "missing.example.org.",
			rtype:  "AAAA",
			from:   "127.0.0.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "missing.example.org.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode:   dns.RcodeNameError,
			expectedAnswer: []dns.RR{}, // not found
		},
		{
			record: "www.nothere.org.",
			rtype:  "AAAA",
			from:   "127.0.0.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "www.nothere.org.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode:   dns.RcodeRefused,
			expectedAnswer: []dns.RR{}, // not authoritative
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()
		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				rec, err := th.QuerySingle(tc.rtype, tc.record, tc.from, "", 1)
				require.Nil(t, err)
				require.Equalf(t, tc.expectedCode, rec.Rcode, "expected status code %d", tc.expectedCode)
				if rec.Msg != nil {
					require.Equal(t, tc.expectedQuestion, rec.Msg.Question)
					RRSliceMatch(t, tc.expectedAnswer, rec.Msg.Answer)
				}
			})
		}
	}
}

// This test is meant to exercise the handling of the location field,
// specifically with characters which, if interpreted incorrectly, could
// cause issues.
func TestSpecialCharactersInLocationFields(t *testing.T) {
	testCases := []struct {
		record           string         // DNS record we want
		rtype            string         // Record type as a string (i.e. AAAA)
		from             string         // IP address as a string (i.e. 127.0.0.1)
		expectedQuestion []dns.Question // The questions RRSets in the question section of the answer.
		expectedCode     int            // The expected Code of the answer.
		expectedAnswer   []dns.RR       // The expected Answer section.
	}{
		{
			record: "foo.example.com.",
			rtype:  "A",
			from:   "6.6.6.0",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("9.9.9.0"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "AAAA",
			from:   "6.6.6.0",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("9:9:9:9::0"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "A",
			from:   "6.6.6.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("9.9.9.1"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "AAAA",
			from:   "6.6.6.1",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("9:9:9:9::1"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "A",
			from:   "6.6.6.2",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("9.9.9.2"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "AAAA",
			from:   "6.6.6.2",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeAAAA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					AAAA: net.ParseIP("9:9:9:9::2"),
				},
			},
		},
		{
			record: "foo.example.com.",
			rtype:  "A",
			from:   "6.6.6.3",
			expectedQuestion: []dns.Question{
				{
					Name:   "foo.example.com.",
					Qtype:  dns.TypeA,
					Qclass: dns.ClassINET,
				},
			},
			expectedCode: dns.RcodeSuccess,
			expectedAnswer: []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "foo.example.com.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    180,
					},
					A: net.ParseIP("9.9.9.3"),
				},
			},
		},
	}
	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()

		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				rec, err := th.QuerySingle(tc.rtype, tc.record, tc.from, "", 1)
				require.Nil(t, err)
				require.Equalf(t, tc.expectedCode, rec.Rcode, "expected status code %d", tc.expectedCode)
				if rec.Msg != nil {
					require.Equal(t, tc.expectedQuestion, rec.Msg.Question)
					RRSliceMatch(t, tc.expectedAnswer, rec.Msg.Answer)
				}
			})
		}
	}
}

func TestCompressionEnforced(t *testing.T) {
	testCases := []struct {
		bigResponse    bool
		alwaysCompress bool
		compressed     bool
	}{
		{
			bigResponse:    true,
			alwaysCompress: false,
			compressed:     true,
		},
		{
			bigResponse:    true,
			alwaysCompress: true,
			compressed:     true,
		},
		{
			bigResponse:    false,
			alwaysCompress: false,
			compressed:     false,
		},
		{
			bigResponse:    false,
			alwaysCompress: true,
			compressed:     true,
		},
	}

	for _, db := range testaid.TestDBs {
		th := OpenDbForTesting(t, &db)
		defer th.Close()

		for nt, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%d", db.Driver, nt), func(t *testing.T) {
				req := new(dns.Msg)
				req.SetEdns0(512, true)
				if tc.bigResponse {
					req.SetQuestion(dns.Fqdn("lotofns.example.org."), dns.TypeNS)
				} else {
					req.SetQuestion(dns.Fqdn("foo.example.com."), dns.TypeA)
				}

				th.handlerConfig.AlwaysCompress = tc.alwaysCompress

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ctx := CreateTestContext(1)
				code, err := th.ServeDNSWithRCODE(ctx, rec, req)

				require.Equal(t, nil, err)
				require.Equal(t, dns.RcodeSuccess, code)
				require.Equal(t, tc.compressed, rec.Msg.Compress)
			})
		}
	}
}
