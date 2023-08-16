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

// TestHandlerBadType tests that we return noerror/nodata
func TestHandlerBadType(t *testing.T) {
	expectedCount := 0
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	wh, err := NewWhoami("example.com", false)
	require.Nil(t, err)
	rc, err := wh.ServeDNS(context.TODO(), rec, req)

	require.Equal(t, dns.RcodeSuccess, rc)
	require.Nil(t, err)
	require.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	require.Equal(t, len(rec.Msg.Answer), expectedCount, "Number of answers should be %d", expectedCount)
}

// TestHandlerValidRequest check that we return the info in TXT records.
func TestHandlerValidRequest(t *testing.T) {
	expectedAnswers := []debuginfo.Pair{
		{Key: "foo1", Val: "bar1"},
		{Key: "foo2", Val: "bar2"},
	}

	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeTXT)
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
	require.Equal(t, rcode, dns.RcodeSuccess)
	require.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	require.Equal(t, len(rec.Msg.Answer), len(expectedAnswers), "Number of answers should be %d", len(expectedAnswers))
	require.Equal(t, w.GetWriteMsgCallCount(), uint64(1), "WriteMsg was called")

	require.False(t, before.After(creationTime), "unexpected creation time")
	require.Equal(t, rec.Msg.Answer[0].(*dns.TXT).Txt, []string{"foo1 bar1"}, "first message is wrong")
	require.Equal(t, rec.Msg.Answer[1].(*dns.TXT).Txt, []string{"foo2 bar2"}, "second message is wrong")
}

// TestHandlerValidRequestNonTxt checks that we return a noerror/nodata
// reply if the user queries for whoami without setting type of TXT.
func TestHandlerValidRequestNonTxt(t *testing.T) {
	remoteIP := "198.51.100.10"
	w := &test.ResponseWriterCustomRemote{RemoteIP: remoteIP}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeAAAA)
	rec := dnstest.NewRecorder(w)
	wh := &Handler{whoamiDomain: makeWhoamiDomain("example.com")}
	rc, err := wh.ServeDNS(context.TODO(), rec, req)

	require.Equal(t, dns.RcodeSuccess, rc)
	require.Nil(t, err)
	require.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")
	require.Equal(t, len(rec.Msg.Answer), 0, "No answers in the answer section.")
	require.Equal(t, req.MsgHdr.Id, rec.Msg.MsgHdr.Id, "Request and response IDs should match.")
}
