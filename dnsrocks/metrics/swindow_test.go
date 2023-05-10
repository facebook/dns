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
	"time"

	"github.com/stretchr/testify/require"
)

func TestSlidewindow(t *testing.T) {
	sw, err := newSlidingWindow(time.Second * 2)
	require.NoError(t, err)
	sw.Add(1)
	sw.Add(1)
	sw.Add(2)
	time.Sleep(time.Second)
	samples := sw.Samples()
	require.Equal(t, 3, len(samples))
	time.Sleep(time.Second * 8)
	sw.Add(5)
	samples = sw.Samples()
	require.Equal(t, 1, len(samples))
}

func TestSlideWindowClean(t *testing.T) {
	// create window without background cleaning goroutine
	sw, err := newWindow(time.Second * 2)
	require.NoError(t, err)
	sw.samples = []sample{
		{
			Value:   1,
			expires: time.Unix(1683557425, 0),
		},
		{
			Value:   2,
			expires: time.Unix(1683557425, 0),
		},
		{
			Value:   3,
			expires: time.Unix(1683557425, 0),
		},
		{
			Value:   4,
			expires: time.Unix(1683557426, 0),
		},
		{
			Value:   5,
			expires: time.Unix(1683557429, 0),
		},
		{
			Value:   6,
			expires: time.Unix(1683557438, 0),
		},
	}

	sw.clean(time.Unix(1683557427, 0))
	want := []sample{
		{
			Value:   5,
			expires: time.Unix(1683557429, 0),
		},
		{
			Value:   6,
			expires: time.Unix(1683557438, 0),
		},
	}
	require.Equal(t, want, sw.samples)

	sw.clean(time.Unix(1683557447, 0))
	want = []sample{}
	require.Equal(t, want, sw.samples)
}

func BenchmarkClean(b *testing.B) {
	samples := []sample{}
	start := 1683557420
	// test against potential 60 sec * 80k QPS
	secs := 60
	qps := 80 * 1000
	for i := 0; i < secs; i++ {
		for j := 0; j < qps; j++ {
			samples = append(samples, sample{
				Value:   int64(j),
				expires: time.Unix(int64(start+i), 0),
			})
		}
	}
	wantSamples := secs * qps
	if len(samples) != wantSamples {
		b.Fatalf("unexpected number of initial samples, want %d, got %d", wantSamples, len(samples))
	}
	for n := 0; n < b.N; n++ {
		sw, err := newWindow(time.Second * 2)
		if err != nil {
			b.Fatalf("failed to create window: %v", err)
		}
		sw.samples = samples
		sw.clean(time.Unix(int64(start+secs/2), 0)) // 30 seconds from start
		wantSamples = 30 * qps
		if len(sw.samples) != wantSamples {
			b.Fatalf("unexpected number of remaining samples after half-cleanup, want %d, got %d", wantSamples, len(sw.samples))
		}
		sw.clean(time.Unix(int64(start+secs), 0)) // 60 seconds from start
		wantSamples = 0
		if len(sw.samples) != wantSamples {
			b.Fatalf("unexpected number of remaining samples after full cleanup, want %d, got %d", wantSamples, len(sw.samples))
		}
	}
}
