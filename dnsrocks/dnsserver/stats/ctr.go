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

package stats

// Counters type implements stats.Stats interface backed by an in-memory storage
type Counters map[string]int64

// NewCounters returns new instance of the Counters type
func NewCounters() Counters {
	return make(Counters)
}

// ResetCounterTo sets the specified key to the value.
func (s Counters) ResetCounterTo(key string, value int64) {
	s[key] = value
}

// ResetCounter resets the specified key to zero.
func (s Counters) ResetCounter(key string) {
	s[key] = 0
}

// IncrementCounterBy increments the specified key by the value
func (s Counters) IncrementCounterBy(key string, value int64) {
	s[key] += value
}

// IncrementCounter increments the specified key by one
func (s Counters) IncrementCounter(key string) {
	s[key]++
}

// AddSample is not implemented here
func (s Counters) AddSample(_ string, _ int64) {
}
