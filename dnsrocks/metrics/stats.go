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
	"sync"
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
	lock   sync.RWMutex
	values map[string]int64
}

// NewStats creates a new stats counter.
// The entity parameter specifies the ODS entity.
// If entity is the empty string, the ODS entity is inferred from the
// Tupperware task name, or the hostname if the process doesn't
// run in Tupperware.
func NewStats() *Stats {
	stats := new(Stats)

	stats.values = make(map[string]int64)

	return stats
}

// IncrementCounter increments the counter for Key by 1.
// Implements dnsserver.IncrementCounter interface.
func (stats *Stats) IncrementCounter(Key string) {
	stats.lock.Lock()
	stats.values[Key]++
	stats.lock.Unlock()
}

// IncrementCounterBy adds Value to the counter for Key
// Implements dnsserver.IncrementCounterBy interface.
func (stats *Stats) IncrementCounterBy(Key string, Value int64) {
	stats.lock.Lock()
	stats.values[Key] += Value
	stats.lock.Unlock()
}

// ResetCounter sets the counter for Key to 0.
// Implements dnsserver.ResetCounter interface.
func (stats *Stats) ResetCounter(Key string) {
	stats.lock.Lock()
	stats.values[Key] = 0
	stats.lock.Unlock()
}

// ResetCounterTo sets the counter for Key to the given value.
// Implements dnsserver.ResetCounterTo interface.
func (stats *Stats) ResetCounterTo(Key string, Value int64) {
	stats.lock.Lock()
	stats.values[Key] = Value
	stats.lock.Unlock()
}

// Get implements export.Int interface
func (stats *Stats) Get() map[string]int64 {
	var ret = make(map[string]int64)
	stats.lock.Lock()
	for key, val := range stats.values {
		ret[key] = val
	}
	stats.lock.Unlock()
	return ret
}
