/*
Copyright (c) Facebook, Inc. and its affiliates.
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

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_latencyAggregator(t *testing.T) {
	e := &ExportedMetrics{Elapsed: 3 * time.Second, Processed: 8, Errors: 0, Latencies: []float64{25.01, 35.02, 10.03, 17.04, 29.05, 14.06, 21.07, 31.08}}
	w := e.AggregateLatencies()

	require.Equal(t, float64(10.03), w.Min)
	require.Equal(t, float64(35.02), w.Max)
	require.Equal(t, float64(22.795), w.Mean)
	require.Equal(t, float64(21.07), w.Median)
	require.Equal(t, float64(14.06), w.Lowerq)
	require.Equal(t, float64(29.05), w.Upperq)
}
