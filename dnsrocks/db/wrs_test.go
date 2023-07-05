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
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestWeightedAnswer1(t *testing.T) {
	wrs := Wrs{MaxAnswers: 1}
	err := wrs.Add(ResourceRecord{Weight: 1, Qtype: dns.TypeA}, nil)
	require.NoError(t, err)
	// One answer in, one answer out: not weighted.
	require.False(t, wrs.WeightedAnswer())
	// Add a second answer.
	err = wrs.Add(ResourceRecord{Weight: 1, Qtype: dns.TypeA}, nil)
	require.NoError(t, err)
	// The answer is now marked as weighted.
	require.True(t, wrs.WeightedAnswer())
}

func TestWeightedAnswer2(t *testing.T) {
	wrs := Wrs{MaxAnswers: 2}
	err := wrs.Add(ResourceRecord{Weight: 1, Qtype: dns.TypeA}, nil)
	require.NoError(t, err)
	// One answer in, one answer out: not weighted.
	require.False(t, wrs.WeightedAnswer())
	// Add a second answer
	err = wrs.Add(ResourceRecord{Weight: 1, Qtype: dns.TypeA}, nil)
	require.NoError(t, err)
	// Two answers in, two answers out: not weighted.
	require.False(t, wrs.WeightedAnswer())
	// Add a third answer
	err = wrs.Add(ResourceRecord{Weight: 1, Qtype: dns.TypeA}, nil)
	require.NoError(t, err)
	// The answer is now marked as weighted.
	require.True(t, wrs.WeightedAnswer())
}

func BenchmarkAdd(b *testing.B) {
	testCases := []struct {
		numAnswers int
		maxAnswers int
		weight     int
	}{{
		numAnswers: 1,
		maxAnswers: 1,
	}, {
		numAnswers: 4,
		maxAnswers: 1,
	}, {
		numAnswers: 100,
		maxAnswers: 1,
	}, {
		numAnswers: 100,
		maxAnswers: 3,
	}, {
		numAnswers: 100,
		maxAnswers: 8,
	}}

	for _, tc := range testCases {
		for _, weighted := range []bool{false, true} {
			benchname := fmt.Sprintf("%d-%d-%v", tc.numAnswers, tc.maxAnswers, weighted)
			b.Run(benchname, func(b *testing.B) {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						w := Wrs{MaxAnswers: tc.maxAnswers}
						for i := 0; i < tc.numAnswers; i++ {
							var weight uint32 = 1
							if weighted {
								weight = 10 // Bypass the special case for weight=1
							}
							err := w.Add(ResourceRecord{Weight: weight, Qtype: dns.TypeA}, nil)
							require.NoError(b, err)
						}
					}
				})
			})
		}
	}
}
