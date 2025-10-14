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

package metrics

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/eclesh/welford"
)

// slidingWindow defines a sliding window for use to count averages over time.
type slidingWindow struct {
	mutex          sync.RWMutex
	sampleLifetime time.Duration
	stats          []*welford.Stats
	pointer        int
	stopping       chan struct{}
}

type swindowStats struct {
	min   int64
	max   int64
	avg   int64
	count int64
}

func newWindow(sampleLifetime time.Duration) (*slidingWindow, error) {
	if sampleLifetime == 0 {
		return nil, errors.New("sliding window cannot be zero")
	}
	// how many seconds of data we store
	secs := int(math.Ceil(sampleLifetime.Seconds()))

	sw := &slidingWindow{
		sampleLifetime: sampleLifetime,
		stats:          make([]*welford.Stats, secs),
		stopping:       make(chan struct{}, 1),
	}
	// for N seconds, we store 60 aggregates, which allows us to
	// have a N-second 'lookback' with natural removal of 'outdated' samples
	for i := range secs {
		sw.stats[i] = welford.New()
	}

	return sw, nil
}

// newSlidingwindow creates a new slidingWindow and launches the cleaner coroutine
func newSlidingWindow(sampleLifetime time.Duration) (*slidingWindow, error) {
	w, err := newWindow(sampleLifetime)
	if err != nil {
		return nil, err
	}
	go w.cleaner()
	return w, nil
}

// refresh moves pointer to next stat cell and resets this cell content
func (sw *slidingWindow) refresh() {
	sw.mutex.Lock()
	sw.pointer++
	if sw.pointer >= len(sw.stats) {
		sw.pointer = 0
	}
	sw.stats[sw.pointer] = welford.New()

	sw.mutex.Unlock()
}

func (sw *slidingWindow) cleaner() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			sw.refresh()
		case <-sw.stopping:
			return
		}
	}
}

// Add adds a new sample into the sliding window
func (sw *slidingWindow) Add(v int64) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	for i := range len(sw.stats) {
		sw.stats[i].Add(float64(v))
	}
}

// Samples returns current samples from the sliding window
func (sw *slidingWindow) Stats() swindowStats {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	// report data from last cell (the one that will be overwritten on next refresh)
	// because it contains aggregates through last sampleLifetime seconds
	oldest := sw.pointer + 1
	if oldest >= len(sw.stats) {
		oldest = 0
	}
	return swindowStats{
		min:   int64(sw.stats[oldest].Min()),
		max:   int64(sw.stats[oldest].Max()),
		avg:   int64(sw.stats[oldest].Mean()),
		count: int64(sw.stats[oldest].Count()),
	}
}
