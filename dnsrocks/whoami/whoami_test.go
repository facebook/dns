/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package whoami

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/debuginfo"
	"github.com/facebook/dns/dnsrocks/dnsserver/test"
)

func makeWhoamiDomain(s string) string {
	return strings.ToLower(dns.Fqdn(s))
}

// nxdomainNext is a mock downstream handler that replies with NXDOMAIN and an
// SOA in the authority section, mimicking an authoritative backend.
type nxdomainNext struct{}

func (nxdomainNext) Name() string { return "nxdomainNext" }

func (nxdomainNext) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(r, dns.RcodeNameError)
	m.Authoritative = true
	m.Ns = append(m.Ns, &dns.SOA{
		Hdr:     dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60},
		Ns:      "ns.example.com.",
		Mbox:    "hostmaster.example.com.",
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  60,
	})
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

// noerrorNext is a mock downstream handler that replies with a non-NXDOMAIN
// (NOERROR) response.
type noerrorNext struct{}

func (noerrorNext) Name() string { return "noerrorNext" }

func (noerrorNext) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

// TestHandlerNonTxtNodata checks that a non-TXT query to the whoami domain is
// passed down the chain to obtain the SOA, and the resulting NXDOMAIN is
// rewritten to a noerror/nodata reply with the SOA retained.
func TestHandlerNonTxtNodata(t *testing.T) {
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeAAAA)
	rec := dnstest.NewRecorder(w)
	wh, err := NewWhoami("example.com", false)
	require.NoError(t, err)
	wh.Next = nxdomainNext{}

	rc, err := wh.ServeDNS(context.TODO(), rec, req)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, rc)
	require.Equal(t, dns.RcodeSuccess, rec.Rcode, "NXDOMAIN should be rewritten to RcodeSuccess.")
	require.Empty(t, rec.Msg.Answer, "no records in the answer section")
	require.Len(t, rec.Msg.Ns, 1, "SOA from downstream should be retained")
	require.IsType(t, &dns.SOA{}, rec.Msg.Ns[0], "authority record should be an SOA")
	require.Equal(t, req.Id, rec.Msg.Id, "request and response IDs should match")
}

// TestHandlerNonTxtNonNxdomain checks that when the downstream response to a
// non-TXT query is not NXDOMAIN, the whoami handler fails rather than rewriting
// the RCODE.
func TestHandlerNonTxtNonNxdomain(t *testing.T) {
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeAAAA)
	rec := dnstest.NewRecorder(w)
	wh, err := NewWhoami("example.com", false)
	require.NoError(t, err)
	wh.Next = noerrorNext{}

	rc, err := wh.ServeDNS(context.TODO(), rec, req)
	require.Error(t, err)
	require.Equal(t, dns.RcodeServerFailure, rc)
}

// TestHandlerTxtInfo checks that TXT and ANY queries are both answered with the
// whoami info as TXT records.
func TestHandlerTxtInfo(t *testing.T) {
	expectedAnswers := []debuginfo.Pair{
		{Key: "foo1", Val: "bar1"},
		{Key: "foo2", Val: "bar2"},
	}

	for _, qtype := range []uint16{dns.TypeTXT, dns.TypeANY} {
		t.Run(dns.TypeToString[qtype], func(t *testing.T) {
			w := &test.ResponseWriter{}
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn("example.com."), qtype)
			rec := dnstest.NewRecorder(w)
			wh := &Handler{whoamiDomain: makeWhoamiDomain("example.com")}

			var creationTime time.Time
			wh.infoGen = func() debuginfo.InfoSrc {
				creationTime = time.Now()
				src := debuginfo.MockInfoSrc(expectedAnswers)
				return &src
			}

			before := time.Now()
			rcode, err := wh.ServeDNS(context.TODO(), rec, req)
			require.NoError(t, err)
			require.Equal(t, dns.RcodeSuccess, rcode)
			require.Equal(t, dns.RcodeSuccess, rec.Rcode, "RcodeSuccess was expected to be returned.")
			require.Len(t, rec.Msg.Answer, len(expectedAnswers), "Number of answers should be %d", len(expectedAnswers))
			require.Equal(t, uint64(1), w.GetWriteMsgCallCount(), "WriteMsg was called")
			require.False(t, before.After(creationTime), "unexpected creation time")
			for i, pair := range expectedAnswers {
				want := []string{fmt.Sprintf("%s %s", pair.Key, pair.Val)}
				require.Equal(t, want, rec.Msg.Answer[i].(*dns.TXT).Txt, "answer %d is wrong", i)
			}
		})
	}
}
