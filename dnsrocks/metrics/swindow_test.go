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
	got := sw.Stats()
	want := swindowStats{
		min:   1,
		max:   2,
		avg:   1,
		count: 3,
	}
	require.Equal(t, want, got)
	time.Sleep(time.Second * 8)
	sw.Add(5)
	got = sw.Stats()
	want = swindowStats{
		min:   5,
		max:   5,
		avg:   5,
		count: 1,
	}
	require.Equal(t, want, got)
}

func TestSlideWindowClean(t *testing.T) {
	// create window without background cleaning goroutine
	sw, err := newWindow(time.Second * 3)
	require.NoError(t, err)
	sw.Add(1)
	sw.Add(2)
	sw.refresh() // one second passed
	sw.Add(3)
	sw.Add(4)
	sw.Add(5)
	got := sw.Stats()
	want := swindowStats{
		min:   1,
		max:   5,
		avg:   3,
		count: 5,
	}
	require.Equal(t, want, got, "after one second")
	sw.refresh() // two seconds passed, all values are still 'visible'
	got = sw.Stats()
	require.Equal(t, want, got, "after two seconds")
	sw.refresh() // three seconds passed
	got = sw.Stats()
	want = swindowStats{
		min:   3,
		max:   5,
		avg:   4,
		count: 3,
	}
	require.Equal(t, want, got, "after three seconds some values got dropped")
	sw.refresh() // four seconds passed
	got = sw.Stats()
	want = swindowStats{}
	require.Equal(t, want, got, "after four seconds")
}

func BenchmarkSlidingWindowAdd(b *testing.B) {
	// create window without background cleaning goroutine
	sw, err := newWindow(time.Second * 2)
	if err != nil {
		b.Fatalf("failed to create window: %v", err)
	}
	for n := 0; n < b.N; n++ {
		sw.Add(int64(n))
	}
}
