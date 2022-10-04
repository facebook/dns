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

// DummyServer is a dummy metrics server
type DummyServer struct {
}

// NewDummyMetricsServer creates a new dummy metrics server
func NewDummyMetricsServer(addr string) (server *DummyServer, err error) {
	return &DummyServer{}, nil
}

// Serve for DummyMetricsServer does nothing
func (s *DummyServer) Serve() error {
	return nil
}

// SetAlive for dummy metrics server does nothing
func (s *DummyServer) SetAlive() {
}

// ConsumeStats for dummy metrics server does nothing
func (s *DummyServer) ConsumeStats(category string, stats *Stats) error {
	return nil
}

//UpdateExporter for dummy metrics server does nothing
func (s *DummyServer) UpdateExporter() {
}
