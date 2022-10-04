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

// Stats is an interface for generate statistics
type Stats interface {
	ResetCounterTo(key string, value int64)
	ResetCounter(key string)
	IncrementCounterBy(key string, value int64)
	IncrementCounter(key string)
}

// DummyStats is a stub stats implementation
type DummyStats struct {
}

// ResetCounterTo stub implementation
func (s *DummyStats) ResetCounterTo(key string, value int64) {}

// ResetCounter stub implementation
func (s *DummyStats) ResetCounter(key string) {}

// IncrementCounterBy stub implementation
func (s *DummyStats) IncrementCounterBy(key string, value int64) {}

// IncrementCounter stub implementation
func (s *DummyStats) IncrementCounter(key string) {}
