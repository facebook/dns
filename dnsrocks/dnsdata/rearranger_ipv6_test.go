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

package dnsdata

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPv6StringConversion(t *testing.T) {
	testCases := []string{
		"0.0.0.0",
		"1.1.1.1",
		"1.1.1.255",
		"2a00:1fa0:42d8::1",
		"2a00:1fa0:42d8::ffff",
		"2a00:1fa0:42d8::ffff:ffff",
		"::",
		"2a00:1fa0:42d8::",
	}

	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%v", nt, tc), func(t *testing.T) {
			ip := ParseIP(tc)
			str := ip.String()
			require.Equal(t, tc, str)
		})
	}
}

func TestEquality(t *testing.T) {
	type testCase struct {
		ip1    string
		ip2    string
		result bool
	}

	testCases := []testCase{
		{
			ip1:    "0.0.0.0",
			ip2:    "0.0.0.0",
			result: true,
		},
		{
			ip1:    "1.2.3.4",
			ip2:    "1.2.3.4",
			result: true,
		},
		{
			ip1:    "0.0.0.0",
			ip2:    "1.2.3.4",
			result: false,
		},
		{
			ip1:    "1.2.3.4",
			ip2:    "4.3.2.1",
			result: false,
		},
		{
			ip1:    "2a00:1fa0:42d8::1",
			ip2:    "2a00:1fa0:42d8::1",
			result: true,
		},
		{
			ip1:    "::",
			ip2:    "0::0000:0",
			result: true,
		},
		{
			ip1:    "::",
			ip2:    "2a00:1fa0:42d8::1",
			result: false,
		},
		{
			ip1:    "2a00:1fa0:42d8::ffff:ffff",
			ip2:    "2a00:1fa0:42d8::1",
			result: false,
		},
	}

	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%v", nt, tc), func(t *testing.T) {
			ip1 := ParseIP(tc.ip1)
			ip2 := ParseIP(tc.ip2)
			netIP := net.ParseIP(tc.ip2)

			require.Equal(t, tc.result, ip1.Equal(ip2))
			require.Equal(t, tc.result, ip2.Equal(ip1))
			require.Equal(t, tc.result, ip1.EqualToNetIP(netIP))
		})
	}
}
