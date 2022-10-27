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

package whoami

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/test"
)

func makeWhoamiDomain(s string) string {
	return strings.ToLower(dns.Fqdn(s))
}

// TestHandlerBadType tests that we return noerror/nodata
func TestHandlerBadType(t *testing.T) {
	expectedCount := 0
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	wh, err := NewWhoami("example.com")
	assert.Nil(t, err)
	rc, err := wh.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.Nil(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	assert.Equal(t, len(rec.Msg.Answer), expectedCount, "Number of answers should be %d", expectedCount)
}

// TestHandlerValidRequestNoECS checks that we return 1 record per message
// and that we format the client IP and server IP properly.
func TestHandlerValidRequestNoECS(t *testing.T) {
	expectedCount := 4

	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeTXT)
	rec := dnstest.NewRecorder(w)
	wh := &Handler{whoamiDomain: makeWhoamiDomain("example.com")}
	rcode, err := wh.ServeDNS(context.TODO(), rec, req)
	assert.NoError(t, err)
	assert.Equal(t, rcode, dns.RcodeSuccess)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	assert.Equal(t, len(rec.Msg.Answer), expectedCount, "Number of answers should be %d", expectedCount)
	assert.Equal(t, w.GetWriteMsgCallCount(), uint64(1), "WriteMsg was called")

	remotePort := ":40212"
	cluster := "foo1c01"
	testCases := []struct {
		remoteIP     string
		remoteIPResp string
	}{
		{
			remoteIP:     "198.51.100.10",
			remoteIPResp: "198.51.100.10" + remotePort,
		},
		{
			remoteIP:     "2001:db8::1",
			remoteIPResp: "[2001:db8::1]" + remotePort,
		},
		{
			remoteIP:     "2001:db8:0:0::1",
			remoteIPResp: "[2001:db8::1]" + remotePort,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
			w := &test.ResponseWriterCustomRemote{RemoteIP: tc.remoteIP}
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn("example.com."), dns.TypeTXT)
			rec := dnstest.NewRecorder(w)
			wh := &Handler{cluster: cluster, whoamiDomain: makeWhoamiDomain("example.com")}
			rc, err := wh.ServeDNS(context.TODO(), rec, req)

			assert.Equal(t, dns.RcodeSuccess, rc)
			assert.Nil(t, err)
			assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
			assert.Equal(t, len(rec.Msg.Answer), expectedCount, "Number of answers should be %d", expectedCount)
			assert.Equal(t, rec.Msg.Answer[0].(*dns.TXT).Txt, []string{"cluster " + cluster}, "Cluster mismatch")
			assert.Equal(t, rec.Msg.Answer[1].(*dns.TXT).Txt, []string{"protocol UDP"}, "Protocol mismatch")
			assert.Equal(t, rec.Msg.Answer[2].(*dns.TXT).Txt, []string{"source " + tc.remoteIPResp}, "Client IP mismatch")
			assert.Equal(t, rec.Msg.Answer[3].(*dns.TXT).Txt, []string{"destination " + w.LocalAddr().String()}, "Destination mismatch")
		})
	}
}

// TestHandlerValidRequestNonTxt checks that we return a noerror/nodata
// reply if the user queries for whoami without setting type of TXT.
func TestHandlerValidRequestNonTxt(t *testing.T) {
	remoteIP := "198.51.100.10"
	cluster := "foo1c01"
	w := &test.ResponseWriterCustomRemote{RemoteIP: remoteIP}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeAAAA)
	rec := dnstest.NewRecorder(w)
	wh := &Handler{cluster: cluster, whoamiDomain: makeWhoamiDomain("example.com")}
	rc, err := wh.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.Nil(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	assert.Equal(t, len(rec.Msg.Answer), 0, "No answers in the answer section.")
	assert.Equal(t, req.MsgHdr.Id, rec.Msg.MsgHdr.Id, "Request and response IDs should match.")
}

// TestHandlerValidRequestWithECS checks that we return 1 record per
// message, that we include ECS and that we format the client IP and server IP properly.
func TestHandlerValidRequestWithECS(t *testing.T) {
	remotePort := ":40212"
	cluster := "foo1c01"
	expectedCount := 5

	testCases := []struct {
		ecs          string
		remoteIP     string
		ecsResp      string
		remoteIPResp string
	}{
		{
			ecs:          "192.0.2.0/24",
			ecsResp:      "192.0.2.0/24/0",
			remoteIP:     "198.51.100.10",
			remoteIPResp: "198.51.100.10" + remotePort,
		},
		{
			ecs:          "192.0.2.0/24",
			ecsResp:      "192.0.2.0/24/0",
			remoteIP:     "2001:db8::1",
			remoteIPResp: "[2001:db8::1]" + remotePort,
		},
		{
			ecs:          "2001:db8:c::/64",
			ecsResp:      "[2001:db8:c::]/64/0",
			remoteIP:     "198.51.100.10",
			remoteIPResp: "198.51.100.10" + remotePort,
		},
		{
			ecs:          "2001:db8:c::/64",
			ecsResp:      "[2001:db8:c::]/64/0",
			remoteIP:     "2001:db8::1",
			remoteIPResp: "[2001:db8::1]" + remotePort,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc), func(t *testing.T) {
			w := &test.ResponseWriterCustomRemote{RemoteIP: tc.remoteIP}
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn("example.com."), dns.TypeTXT)
			o, _ := dnsserver.MakeOPTWithECS(tc.ecs)
			req.Extra = []dns.RR{o}
			rec := dnstest.NewRecorder(w)
			wh := &Handler{cluster: cluster, whoamiDomain: makeWhoamiDomain("example.com")}
			rc, err := wh.ServeDNS(context.TODO(), rec, req)

			assert.Equal(t, dns.RcodeSuccess, rc)
			assert.Nil(t, err)
			assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
			assert.Equalf(t, len(rec.Msg.Answer), expectedCount, "Number of answers should be %d", expectedCount)
			assert.Equal(t, rec.Msg.Answer[0].(*dns.TXT).Txt, []string{"cluster " + cluster}, "Cluster mismatch")
			assert.Equal(t, rec.Msg.Answer[1].(*dns.TXT).Txt, []string{"protocol UDP"}, "Protocol mismatch")
			assert.Equal(t, rec.Msg.Answer[2].(*dns.TXT).Txt, []string{"source " + tc.remoteIPResp}, "Client IP mismatch")
			assert.Equal(t, rec.Msg.Answer[3].(*dns.TXT).Txt, []string{"destination " + w.LocalAddr().String()}, "Destination mismatch")
			assert.Equal(t, rec.Msg.Answer[4].(*dns.TXT).Txt, []string{"ecs " + tc.ecsResp}, "ECS subnet mismatch")
		})
	}
}
