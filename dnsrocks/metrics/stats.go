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
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
)

// Stats implements dnsserver.stats.Stats. It also implements export.Int interface,
// allowing the counters to be exported.
// * NewStats to create it,
// * IncrementCounter increments the counter by 1
// * IncrementCounterBy increments the counter by `value`
// * ResetCounter resets the counter to 0
// * ResetCounterTo resets the counter to `value`
// * Get to export them.
type Stats struct {
	vlock   sync.RWMutex
	wlock   sync.RWMutex
	values  map[string]int64
	windows map[string]*slidingWindow
}

// NewStats creates a new stats counter.
// The entity parameter specifies the ODS entity.
// If entity is the empty string, the ODS entity is inferred from the
// Tupperware task name, or the hostname if the process doesn't
// run in Tupperware.
func NewStats() *Stats {
	stats := new(Stats)

	stats.values = make(map[string]int64)
	stats.windows = make(map[string]*slidingWindow)
	return stats
}

// IncrementCounter increments the counter for key by 1.
// Implements dnsserver.IncrementCounter interface.
func (stats *Stats) IncrementCounter(key string) {
	stats.vlock.Lock()
	stats.values[key]++
	stats.vlock.Unlock()
}

// IncrementCounterBy adds Value to the counter for key
// Implements dnsserver.IncrementCounterBy interface.
func (stats *Stats) IncrementCounterBy(key string, value int64) {
	stats.vlock.Lock()
	stats.values[key] += value
	stats.vlock.Unlock()
}

// ResetCounter sets the counter for key to 0.
// Implements dnsserver.ResetCounter interface.
func (stats *Stats) ResetCounter(key string) {
	stats.vlock.Lock()
	stats.values[key] = 0
	stats.vlock.Unlock()
}

// ResetCounterTo sets the counter for key to the given value.
// Implements dnsserver.ResetCounterTo interface.
func (stats *Stats) ResetCounterTo(key string, value int64) {
	stats.vlock.Lock()
	stats.values[key] = value
	stats.vlock.Unlock()
}

// AddSample adds a sample to the sliding window identified by key
func (stats *Stats) AddSample(key string, value int64) {
	stats.wlock.Lock()
	defer stats.wlock.Unlock()
	var win *slidingWindow
	winfound, found := stats.windows[key]
	if !found {
		newwin, err := newSlidingWindow(60 * time.Second)
		if err != nil {
			glog.Errorf("failed to register new sliding window")
			return
		}
		stats.windows[key] = newwin
		win = newwin
	} else {
		win = winfound
	}
	win.Add(value)
}

// Get implements export.Int interface
func (stats *Stats) Get() map[string]int64 {
	var ret = make(map[string]int64)
	stats.vlock.Lock()
	for key, val := range stats.values {
		ret[key] = val
	}
	stats.vlock.Unlock()
	stats.wlock.Lock()
	for key, val := range stats.windows {
		s := val.Stats()
		ret[fmt.Sprintf("%s.min", key)] = s.min
		ret[fmt.Sprintf("%s.max", key)] = s.max
		ret[fmt.Sprintf("%s.avg", key)] = s.avg
	}
	stats.wlock.Unlock()
	return ret
}
