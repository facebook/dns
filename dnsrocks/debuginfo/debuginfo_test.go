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

package debuginfo

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/test"
)

// TestWithoutECS checks that we extract the expected info
// and that we format the client IP and server IP properly.
func TestWithoutECS(t *testing.T) {
	remotePort := ":40212"
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
			state := request.Request{W: w, Req: req}

			info := GetInfo(state)

			expected := []Pair{
				{"protocol", "UDP"},
				{"source", tc.remoteIPResp},
				{"destination", w.LocalAddr().String()},
			}
			require.Equal(t, info[:len(expected)], expected, "wrong info output")
		})
	}
}

// TestWithECS checks that we include ECS and that
// we format the client IP and server IP properly.
func TestWithECS(t *testing.T) {
	remotePort := ":40212"

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
			state := request.Request{W: w, Req: req}

			info := GetInfo(state)

			expected := []Pair{
				{"protocol", "UDP"},
				{"source", tc.remoteIPResp},
				{"destination", w.LocalAddr().String()},
				{"ecs", tc.ecsResp},
			}
			require.Equal(t, info[:len(expected)], expected, "wrong info output")
		})
	}
}
