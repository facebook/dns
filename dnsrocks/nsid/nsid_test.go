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

package nsid

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/debuginfo"
)

type mockInfo []debuginfo.Pair

func (i mockInfo) GetInfo(_ request.Request) []debuginfo.Pair {
	return i
}

func TestNSID(t *testing.T) {
	w := &test.ResponseWriter{}

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	req.SetEdns0(1234, false)
	req.IsEdns0().Option = append(req.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})

	rec := dnstest.NewRecorder(w)
	h, err := NewHandler(false)
	require.NoError(t, err)
	var infoTime time.Time
	h.infoGen = func() debuginfo.InfoSrc {
		infoTime = time.Now()
		return mockInfo([]debuginfo.Pair{
			{Key: "foo1", Val: "bar1"},
			{Key: "foo2", Val: "bar2"},
		})
	}
	var responseTime time.Time
	h.Next = test.HandlerFunc(func(c context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		responseTime = time.Now()
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		m.SetEdns0(1234, false)
		err := w.WriteMsg(m)
		require.NoError(t, err)
		return dns.RcodeSuccess, nil
	})
	rc, err := h.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.NoError(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")

	nsid, ok := rec.Msg.IsEdns0().Option[0].(*dns.EDNS0_NSID)
	require.True(t, ok, "Didn't find NSID in the expected place")
	data, err := hex.DecodeString(nsid.Nsid)
	require.NoError(t, err, "hex decode failed")
	assert.False(t, infoTime.After(responseTime), "info time should be set before the response is created")
	assert.Equal(t, "foo1=bar1 foo2=bar2", string(data))
}

func TestNoNSIDRequested(t *testing.T) {
	w := &test.ResponseWriter{}
	w.TCP = true

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	req.SetEdns0(1234, false)
	// Request does not include NSID.

	rec := dnstest.NewRecorder(w)
	h, err := NewHandler(false)
	require.NoError(t, err)
	h.Next = test.HandlerFunc(func(c context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		m.SetEdns0(1234, false)
		err2 := w.WriteMsg(m)
		require.NoError(t, err2)
		return dns.RcodeSuccess, nil
	})
	rc, err := h.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.NoError(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")

	for _, opt := range rec.Msg.IsEdns0().Option {
		assert.NotEqual(t, dns.EDNS0NSID, opt.Option())
	}
}

func TestNoEDNSInQuery(t *testing.T) {
	w := &test.ResponseWriter{}
	w.TCP = true

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	// Request does not include EDNS.

	rec := dnstest.NewRecorder(w)
	h, err := NewHandler(false)
	require.NoError(t, err)
	h.Next = test.HandlerFunc(func(c context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		// Response does not include EDNS
		err2 := w.WriteMsg(m)
		require.NoError(t, err2)
		return dns.RcodeSuccess, nil
	})
	rc, err := h.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.NoError(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")

	// This handler doesn't add EDNS.
	assert.Nil(t, rec.Msg.IsEdns0())
}

func TestEDNSIsDisabled(t *testing.T) {
	w := &test.ResponseWriter{}
	w.TCP = true

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	req.SetEdns0(1234, false)
	req.IsEdns0().Option = append(req.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})

	rec := dnstest.NewRecorder(w)
	h, err := NewHandler(false)
	require.NoError(t, err)
	h.Next = test.HandlerFunc(func(c context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		// Next handler doesn't add EDNS even though it was requested.
		err2 := w.WriteMsg(m)
		require.NoError(t, err2)
		return dns.RcodeSuccess, nil
	})
	rc, err := h.ServeDNS(context.TODO(), rec, req)

	assert.Equal(t, dns.RcodeSuccess, rc)
	assert.NoError(t, err)
	assert.Equal(t, rec.Rcode, dns.RcodeSuccess, "RcodeSuccess was expected to be returned.")

	// This handler doesn't add EDNS.
	assert.Nil(t, rec.Msg.IsEdns0())
}
