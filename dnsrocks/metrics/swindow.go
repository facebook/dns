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
	"errors"
	"sync"
	"time"
)

type sample struct {
	Value   int64
	expires time.Time
}

// slidingWindow defines a sliding window for use to count averages over time.
type slidingWindow struct {
	mutex          sync.RWMutex
	sampleLifetime time.Duration
	samples        []sample
	stopping       chan struct{}
}

func newWindow(sampleLifetime time.Duration) (*slidingWindow, error) {
	if sampleLifetime == 0 {
		return nil, errors.New("sliding window cannot be zero")
	}

	sw := &slidingWindow{
		sampleLifetime: sampleLifetime,
		samples:        make([]sample, 0),
		stopping:       make(chan struct{}, 1),
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

func (sw *slidingWindow) cleaner() {
	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-ticker.C:
			sw.mutex.Lock()
			newstartidx := 0
			for idx, val := range sw.samples {
				if val.expires.Before(time.Now()) {
					newstartidx = idx + 1
				} else {
					break
				}
				if len(sw.samples) > newstartidx {
					newsamples := make([]sample, len(sw.samples)-newstartidx)
					copy(sw.samples[newstartidx:], newsamples)
					sw.samples = newsamples
				} else {
					sw.samples = make([]sample, 0)
				}
			}
			sw.mutex.Unlock()

		case <-sw.stopping:
			return
		}
	}
}

// Add adds a new sample into the sliding window
func (sw *slidingWindow) Add(v int64) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	sw.samples = append(sw.samples, sample{v, time.Now().Add(sw.sampleLifetime)})
}

// Samples returns current samples from the sliding window
func (sw *slidingWindow) Samples() []int64 {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	samples := make([]int64, len(sw.samples))
	for idx, sws := range sw.samples {
		samples[idx] = sws.Value
	}
	return samples

}
