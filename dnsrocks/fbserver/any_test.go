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

package fbserver

import (
	"context"
	"testing"

	"github.com/facebook/dns/dnsrocks/dnsserver/test"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// TestAnyHandlerBadType tests that we continue to the next handler when a
// different dns type is queried
func TestAnyHandlerBadType(t *testing.T) {
	expectedError := "plugin/any: no next plugin found"
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	ah, err := newAnyHandler()
	require.Nil(t, err)
	rc, err := ah.ServeDNS(context.TODO(), rec, req)

	require.Equal(t, rc, dns.RcodeServerFailure)
	require.Equal(t, expectedError, err.Error())
}

// TestWhoamiHandlerCorrectType tests that we return "RFC 8482" ""
func TestAnyHandlerCorrectType(t *testing.T) {
	testCases := []struct {
		qname         string
		answerCPU     string
		answerOS      string
		expectedCount int
	}{
		{
			// example.com
			qname:         "example.com.",
			answerCPU:     "RFC 8482",
			answerOS:      "",
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		w := &test.ResponseWriter{}
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), dns.TypeANY)
		rec := dnstest.NewRecorder(w)
		ah, err := newAnyHandler()
		require.Nil(t, err)
		rc, err := ah.ServeDNS(context.TODO(), rec, req)

		require.Equal(t, dns.RcodeSuccess, rc)
		require.Nil(t, err)
		require.Equal(t, dns.RcodeSuccess, rec.Rcode, "RcodeSuccess was expected to be returned.")
		require.Equal(t, tc.expectedCount, len(rec.Msg.Answer), "Number of answers should be %d", tc.expectedCount)
		require.Equal(t, tc.answerCPU, rec.Msg.Answer[0].(*dns.HINFO).Cpu, "Answer should be %s", tc.answerCPU)
		require.Equal(t, tc.answerOS, rec.Msg.Answer[0].(*dns.HINFO).Os, "Answer should be %s", tc.answerOS)
	}
}
