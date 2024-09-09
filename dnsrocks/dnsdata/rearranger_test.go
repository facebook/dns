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
	"bytes"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func strToNet(t *testing.T, s string) *net.IPNet {
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return net
}

func TestRearranger(t *testing.T) {
	type testLocation struct {
		network string
		locID   []byte
	}
	type testOutput struct {
		startIP   string
		maskLen   uint8
		locIsNull bool
		locID     []byte
	}
	type testCase struct {
		description string
		input       []testLocation
		output      []testOutput
	}
	testCases := []testCase{
		{
			description: "one /64 range without super-range (as in ECS map)",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 1},
				},
			},
			output: []testOutput{
				{
					startIP:   "0::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
			},
		},
		{
			description: "one /64 range and a super-range (as in resolver map)",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 1},
				},
				{
					network: "0::/0",
					locID:   []byte{0, 2},
				},
			},
			output: []testOutput{
				{
					startIP:   "0::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
			},
		},
		{
			description: "one /64 range and two default ranges (v4+v6)",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 1},
				},
				{
					network: "0::/0",
					locID:   []byte{0, 2},
				},
				{
					network: "0.0.0.0/0",
					locID:   []byte{0, 3},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
			},
		},
		{
			description: "two v4+v6 ranges and one v6 default range",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "10.0.0.0/8",
					locID:   []byte{0, 3},
				},
				{
					network: "0::/0",
					locID:   []byte{0, 1},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "10.0.0.0",
					maskLen:   8,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "11.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
			},
		},
		{
			description: "two v4+v6 ranges and one v4 default range",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "10.0.0.0/8",
					locID:   []byte{0, 3},
				},
				{
					network: "0.0.0.0/0",
					locID:   []byte{0, 1},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "10.0.0.0",
					maskLen:   8,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "11.0.0.0",
					maskLen:   0,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
			},
		},
		{
			description: "two v4+v6 ranges and no default range",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "10.0.0.0/8",
					locID:   []byte{0, 3},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "10.0.0.0",
					maskLen:   8,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "11.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
			},
		},
		{
			description: "two adjacent /64 ranges",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8:0::/64",
					locID:   []byte{0, 1},
				},
				{
					network: "2a00:1fa0:42d8:1::/64",
					locID:   []byte{0, 2},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:2::",
					maskLen:   0,
					locIsNull: true,
				},
			},
		},
		{
			description: "two non-adjacent /64 ranges",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8:0::/64",
					locID:   []byte{0, 1},
				},
				{
					network: "2a00:1fa0:42d8:2::/64",
					locID:   []byte{0, 2},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:2::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:3::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
			},
		},
		{
			description: "two non-adjacent /64 ranges inside a larger /48 range",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8:0::/64",
					locID:   []byte{0, 1},
				},
				{
					network: "2a00:1fa0:42d8:2::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "2a00:1fa0:42d8::/48",
					locID:   []byte{0, 3},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d8:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d8:2::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:3::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d9::",
					maskLen:   0,
					locIsNull: true,
				},
			},
		},
		{
			description: "three nested ranges that start at the same IP",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8:0:0::/80",
					locID:   []byte{0, 1},
				},
				{
					network: "2a00:1fa0:42d8:0::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "2a00:1fa0:42d8::/48",
					locID:   []byte{0, 3},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8:0:0::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d8:0:0::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:0:0::",
					maskLen:   80,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d8:0:1::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:1::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d9::",
					maskLen:   0,
					locIsNull: true,
					locID:     nil,
				},
			},
		},
		{
			description: "three nested ranges that end at the same IP",
			input: []testLocation{
				{
					network: "2a00:1fa0:42d8::/48",
					locID:   []byte{0, 3},
				},
				{
					network: "2a00:1fa0:42d8:ffff::/64",
					locID:   []byte{0, 2},
				},
				{
					network: "2a00:1fa0:42d8:ffff:ffff::/80",
					locID:   []byte{0, 1},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "2a00:1fa0:42d8::",
					maskLen:   48,
					locIsNull: false,
					locID:     []byte{0, 3},
				},
				{
					startIP:   "2a00:1fa0:42d8:ffff::",
					maskLen:   64,
					locIsNull: false,
					locID:     []byte{0, 2},
				},
				{
					startIP:   "2a00:1fa0:42d8:ffff:ffff::",
					maskLen:   80,
					locIsNull: false,
					locID:     []byte{0, 1},
				},
				{
					startIP:   "2a00:1fa0:42d9::",
					maskLen:   0,
					locIsNull: true,
				},
			},
		},
		{
			description: "ends of two ranges align with the beginning of the third one",
			input: []testLocation{
				{
					network: "125.6.160.0/20",
					locID:   []byte{0, 55},
				},
				{
					network: "125.6.175.0/24",
					locID:   []byte{0, 56},
				},
				{
					network: "125.6.176.0/20",
					locID:   []byte{0, 57},
				},
			},
			output: []testOutput{
				{
					startIP:   "::",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "0.0.0.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "125.6.160.0",
					maskLen:   20,
					locIsNull: false,
					locID:     []byte{0, 55},
				},
				{
					startIP:   "125.6.175.0",
					maskLen:   24,
					locIsNull: false,
					locID:     []byte{0, 56},
				},
				{
					startIP:   "125.6.176.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "125.6.176.0",
					maskLen:   20,
					locIsNull: false,
					locID:     []byte{0, 57},
				},
				{
					startIP:   "125.6.192.0",
					maskLen:   0,
					locIsNull: true,
				},
				{
					startIP:   "::1:0:0:0", // 255.255.255.255 + "ff:ff" prefix + 1
					maskLen:   0,
					locIsNull: true,
				},
			},
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%s", nt, tc.description), func(t *testing.T) {
			r := NewRearranger(len(tc.input))
			for _, in := range tc.input {
				err := r.AddLocation(strToNet(t, in.network), in.locID)
				require.NoError(t, err)
			}
			res := r.Rearrange()
			require.Equal(t, len(tc.output), len(res))
			for i, out := range tc.output {
				if i >= len(res) {
					break
				}
				assertEqual(t, net.ParseIP(out.startIP), res[i].To16())
				require.Equal(t, out.maskLen, res[i].MaskLen())
				require.Equal(t, out.locIsNull, res[i].LocIsNull())

				if !out.locIsNull {
					require.Equal(t, out.locID, res[i].LocID())
				}
			}
		})
	}
}

func TestAddSingleLocation(t *testing.T) {
	type testCase struct {
		network        string
		maskLen        uint8
		locID          []byte
		startIP        string
		startPointKind rangePointKind
		nextIP         string
		nextPointKind  rangePointKind
	}
	testCases := []testCase{
		{
			network:        "31.185.13.0/24",
			maskLen:        24,
			locID:          []byte{0, 6},
			startIP:        "31.185.13.0",
			startPointKind: pointKindStart,
			nextIP:         "31.185.14.0",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "81.181.202.0/23",
			maskLen:        23,
			locID:          []byte{0, 6},
			startIP:        "81.181.202.0",
			startPointKind: pointKindStart,
			nextIP:         "81.181.204.0",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "2a00:1fa0:42d8::/47",
			maskLen:        47,
			locID:          []byte{0, 6},
			startIP:        "2a00:1fa0:42d8:0:0:0:0:0",
			startPointKind: pointKindStart,
			nextIP:         "2a00:1fa0:42da::",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "2a00:4802:4800::/39",
			maskLen:        39,
			locID:          []byte{0, 6},
			startIP:        "2a00:4802:4800::",
			startPointKind: pointKindStart,
			nextIP:         "2a00:4802:4a00::",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "2607:fea8:5b20::/48",
			maskLen:        48,
			locID:          []byte{0, 106},
			startIP:        "2607:fea8:5b20::",
			startPointKind: pointKindStart,
			nextIP:         "2607:fea8:5b21::",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "0.0.0.0/0",
			maskLen:        0,
			locID:          []byte{0, 105},
			startIP:        "::0:ffff:0:0",
			startPointKind: pointKindStart,
			nextIP:         "::1:0:0:0",
			nextPointKind:  pointKindEnd,
		},
		{
			network:        "0::/0",
			maskLen:        0,
			locID:          []byte{0, 105},
			startIP:        "::",
			startPointKind: pointKindStart,
			nextIP:         "::1:0:0:0",
			nextPointKind:  pointKindStart,
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%s", nt, tc.network), func(t *testing.T) {
			r := NewRearranger(1) // we expect just one location
			err := r.AddLocation(strToNet(t, tc.network), tc.locID)

			require.Nil(t, err)

			require.Equal(t, 2, len(r.points))

			// the first RangePoint is the start of the range, has IP address of the beginning of this network, and known location
			assertEqual(t, net.ParseIP(tc.startIP), r.points[0].To16())
			require.Equal(t, tc.locID, r.points[0].LocID())
			require.False(t, r.points[0].LocIsNull())
			require.Equal(t, tc.startPointKind, r.points[0].pointKind)
			require.Equal(t, tc.maskLen, r.points[0].MaskLen())

			// the second RangePoint is end of the range, and has IP address that follows this network (last IP address of this network + 1),
			// the location is not known yet
			assertEqual(t, net.ParseIP(tc.nextIP), r.points[1].To16())
			require.Equal(t, tc.nextPointKind, r.points[1].pointKind)
			require.Equal(t, tc.maskLen, r.points[0].MaskLen())
		})
	}
}

func TestInvalidLocation(t *testing.T) {
	type testCase struct {
		network string
		locID   []byte
	}

	testCases := []testCase{
		// {
		// 	network: "31.185.13.0/24",
		// 	locID:   []byte{0, 1, 2, 3},
		// }, We can be longer than 2 bytes now, so this is a bad testcase.
		{
			network: "31.185.13.0/24",
			locID:   []byte{1},
		},
		{
			network: "31.185.13.0/24",
			locID:   []byte{0},
		},
	}

	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%s", nt, tc.network), func(t *testing.T) {
			r := NewRearranger(1) // we expect just one location
			err := r.AddLocation(strToNet(t, tc.network), tc.locID)

			require.NotNil(t, err)
			require.ErrorIs(t, err, ErrInvalidLocation)
		})
	}
}

func TestLpad(t *testing.T) {
	in := []byte{1, 2}
	npad := 4
	targ := []byte{0, 0, 1, 2}

	out := lpad(in, npad)
	if !bytes.Equal(out, targ) {
		t.Fatalf("expected %v, got %v", targ, out)
	}
}

func TestIPCleanMask(t *testing.T) {
	type testCase struct {
		ipaddr string
		mask   net.IPMask
		result string
	}
	testCases := []testCase{
		{
			ipaddr: "1.1.1.1",
			mask:   net.CIDRMask(24, 32),
			result: "1.1.1.0",
		},
		{
			ipaddr: "1.1.1.255",
			mask:   net.CIDRMask(24, 32),
			result: "1.1.1.0",
		},
		{
			ipaddr: "1.1.1.255",
			mask:   net.CIDRMask(23, 32),
			result: "1.1.0.0",
		},
		{
			ipaddr: "2a00:1fa0:42d8::1",
			mask:   net.CIDRMask(32, 128),
			result: "2a00:1fa0::",
		},
		{
			ipaddr: "2a00:1fa0:42d8::1",
			mask:   net.CIDRMask(128, 128),
			result: "2a00:1fa0:42d8::1",
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%v", nt, tc), func(t *testing.T) {
			ip := net.ParseIP(tc.ipaddr)
			ret := ipCleanMask(&ip, &tc.mask)
			res := net.ParseIP(tc.result)
			assertEqual(t, res, ret)
		})
	}
}

func TestIPFillUnmasked(t *testing.T) {
	type testCase struct {
		ipaddr string
		mask   net.IPMask
		result string
	}
	testCases := []testCase{
		{
			ipaddr: "1.1.1.1",
			mask:   net.CIDRMask(24, 32),
			result: "1.1.1.255",
		},
		{
			ipaddr: "1.1.1.0",
			mask:   net.CIDRMask(24, 32),
			result: "1.1.1.255",
		},
		{
			ipaddr: "1.1.0.0",
			mask:   net.CIDRMask(23, 32),
			result: "1.1.1.255",
		},
		{
			ipaddr: "2a00:1fa0:42d8::1",
			mask:   net.CIDRMask(32, 128),
			result: "2a00:1fa0:ffff:ffff:ffff:ffff:ffff:ffff",
		},
		{
			ipaddr: "2a00:1fa0:42d8::1",
			mask:   net.CIDRMask(128, 128),
			result: "2a00:1fa0:42d8::1",
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%v", nt, tc), func(t *testing.T) {
			ip := net.ParseIP(tc.ipaddr)
			ret := ipFillUnmasked(&ip, &tc.mask)
			res := net.ParseIP(tc.result)
			assertEqual(t, res, ret)
		})
	}
}

func TestIPIncrementByOne(t *testing.T) {
	type testCase struct {
		ipaddr string
		result string
	}
	testCases := []testCase{
		{
			ipaddr: "1.1.1.1",
			result: "1.1.1.2",
		},
		{
			ipaddr: "1.1.1.255",
			result: "1.1.2.0",
		},
		{
			ipaddr: "1.1.255.255",
			result: "1.2.0.0",
		},
		{
			ipaddr: "2a00:1fa0:42d8::1",
			result: "2a00:1fa0:42d8::2",
		},
		{
			ipaddr: "2a00:1fa0:42d8::ffff",
			result: "2a00:1fa0:42d8::0001:0000",
		},
		{
			ipaddr: "2a00:1fa0:42d8::ffff:ffff",
			result: "2a00:1fa0:42d8::0001:0000:0000",
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%v", nt, tc), func(t *testing.T) {
			ip := ParseIP(tc.ipaddr)
			result := ipIncrementByOne(ip)
			res := net.ParseIP(tc.result)
			assertEqual(t, res, result)
		})
	}
}

func TestRangePointMarshalTextForLmap(t *testing.T) {
	type testCase struct {
		p    RangePoint
		lmap Lmap
		want string
	}
	testCases := []testCase{
		{
			p: RangePoint{
				rangeStart: ParseIP("0.0.0.0"),
				location: rangeLocation{
					maskLen:     0,
					locIDIsNull: true,
				},
			},
			lmap: Lmap("\155\061"),
			want: "!\\155\\061,0.0.0.0",
		},
		{
			p: RangePoint{
				rangeStart: ParseIP("1.1.1.1"),
				location: rangeLocation{
					maskLen:     120,
					locID:       []byte{100, 101},
					locIDIsNull: false,
				},
			},
			lmap: Lmap("\155\061"),
			want: "!\\155\\061,1.1.1.1,24,\\144\\145",
		},
		{
			p: RangePoint{
				rangeStart: ParseIP("2a00:1fa0:42d8::"),
				location: rangeLocation{
					maskLen:     64,
					locID:       []byte{97, 1},
					locIDIsNull: false,
				},
			},
			lmap: Lmap("\105\027"),
			want: "!\\105\\027,2a00:1fa0:42d8::,64,\\141\\001",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got, err := tc.p.MarshalTextForLmap(tc.lmap)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}

func assertEqual(t *testing.T, expected net.IP, actual IPv6) {
	require.Equal(t, []byte(expected), actual[:])
}

func BenchmarkRearranger(b *testing.B) {
	type testLocation struct {
		network string
		locID   []byte
	}

	locations := []testLocation{}

	for i := 1; i <= 0xFEFE; i++ {
		location := []byte{byte(i / 0xFF), byte(i % 0xFF)}
		network := fmt.Sprintf("2a00:1fa0:%x::/64", i)

		locations = append(locations, testLocation{network, location})
	}

	// run the DiffWith function b.N times
	for n := 0; n < b.N; n++ {
		r := NewRearranger(1)
		for _, in := range locations {
			_, net, _ := net.ParseCIDR(in.network)
			err := r.AddLocation(net, in.locID)
			if err != nil {
				b.Fatalf("%v", err)
			}
		}
		r.Rearrange()
	}
}
