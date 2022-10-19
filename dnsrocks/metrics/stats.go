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
	"fmt"
	"sort"
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
	values  *sync.Map
	windows *sync.Map
}

// NewStats creates a new stats counter.
// The entity parameter specifies the ODS entity.
// If entity is the empty string, the ODS entity is inferred from the
// Tupperware task name, or the hostname if the process doesn't
// run in Tupperware.
func NewStats() *Stats {
	stats := new(Stats)

	stats.values = &sync.Map{}
	stats.windows = &sync.Map{}
	return stats
}

// IncrementCounter increments the counter for key by 1.
// Implements dnsserver.IncrementCounter interface.
func (stats *Stats) IncrementCounter(key string) {
	val, loaded := stats.values.Load(key)
	if loaded {
		intval := val.(int64)
		stats.values.Store(key, intval+1)
	} else {
		stats.values.Store(key, int64(1))
	}
}

// IncrementCounterBy adds Value to the counter for key
// Implements dnsserver.IncrementCounterBy interface.
func (stats *Stats) IncrementCounterBy(key string, value int64) {
	val, loaded := stats.values.Load(key)
	if loaded {
		intval := val.(int64)
		stats.values.Store(key, intval+value)
	} else {
		stats.values.Store(key, value)
	}
}

// ResetCounter sets the counter for key to 0.
// Implements dnsserver.ResetCounter interface.
func (stats *Stats) ResetCounter(key string) {
	stats.values.Store(key, int64(0))
}

// ResetCounterTo sets the counter for key to the given value.
// Implements dnsserver.ResetCounterTo interface.
func (stats *Stats) ResetCounterTo(key string, value int64) {
	stats.values.Store(key, value)
}

// AddSample adds a sample to the sliding window identified by key
func (stats *Stats) AddSample(key string, value int64) {
	newWin, err := newSlidingWindow(60 * time.Second)
	if err != nil {
		glog.Errorf("failed to register new sliding window")
	}
	win, _ := stats.windows.LoadOrStore(key, newWin)
	win.(*slidingWindow).Add(value)
}

// Get implements export.Int interface
func (stats *Stats) Get() map[string]int64 {
	var ret = make(map[string]int64)
	stats.values.Range(func(key any, value any) bool {
		kstr := key.(string)
		val := value.(int64)
		ret[kstr] = val
		return true
	})
	stats.windows.Range(func(key any, value any) bool {
		kstr := key.(string)
		val := value.(*slidingWindow)
		samples := val.Samples()
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		if len(samples) > 0 {
			ret[fmt.Sprintf("%s.min", kstr)] = samples[0]
			ret[fmt.Sprintf("%s.max", kstr)] = samples[len(samples)-1]
			var sum int64
			for _, numb := range samples {
				sum += numb
			}
			ret[fmt.Sprintf("%s.avg", kstr)] = sum / int64(len(samples))
		} else {
			ret[fmt.Sprintf("%s.min", kstr)] = 0
			ret[fmt.Sprintf("%s.max", kstr)] = 0
			ret[fmt.Sprintf("%s.avg", kstr)] = 0
		}
		return true
	})
	return ret
}
