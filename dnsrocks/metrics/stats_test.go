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

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatsGet(t *testing.T) {
	inputs := map[string][]int64{
		"foo":             {1, 2, 3, 4, 5, 6},
		"bar":             {19999, -777, 28888},
		"nope":            {},
		"some.long.stuff": {1, 2, 3, 4, 5, 6},
	}

	s := NewStats()

	for k, vv := range inputs {
		for _, v := range vv {
			s.AddSample(k, v)
		}
	}

	got := s.Get()
	want := map[string]int64{
		"foo.min":             1,
		"foo.max":             6,
		"foo.avg":             3,
		"bar.min":             -777,
		"bar.max":             28888,
		"bar.avg":             16036,
		"some.long.stuff.min": 1,
		"some.long.stuff.max": 6,
		"some.long.stuff.avg": 3,
	}
	require.Equal(t, want, got)
}
