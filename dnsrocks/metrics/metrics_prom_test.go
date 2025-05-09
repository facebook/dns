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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func assertMetricRegisteredAndHasExpectedValue(t *testing.T, registry *prometheus.Registry, metricKey string, expectedValue float64) {
	metrics, err := registry.Gather()
	require.Nil(t, err)
	require.NotNil(t, metrics)
	found := false
	for _, metric := range metrics {
		if metric.GetName() == metricKey {
			found = true
			require.Equal(t, metric.GetType(), dto.MetricType_GAUGE)
			rawmetric := metric.GetMetric()[0]
			require.Equal(t, *rawmetric.Gauge.Value, expectedValue)
			break
		}
	}
	require.True(t, found)
}

func TestRegistryPicksUpNewCounters(t *testing.T) {
	stats := NewStats()
	stats.IncrementCounter("test")
	metricsServer, err := NewMetricsServer(":0")
	require.Nil(t, err)
	err = metricsServer.ConsumeStats("test", stats)
	require.NoError(t, err)
	go metricsServer.UpdateExporter()
	time.Sleep(2 * time.Second)
	assertMetricRegisteredAndHasExpectedValue(t, metricsServer.registry, "test_test", 1.0)
	stats.IncrementCounter("test")
	time.Sleep(2 * time.Second)
	assertMetricRegisteredAndHasExpectedValue(t, metricsServer.registry, "test_test", 2.0)
}

func TestSetAliveExposesAliveInMetrics(t *testing.T) {
	metricsServer, err := NewMetricsServer(":0")
	require.Nil(t, err)
	metricsServer.SetAlive()
	assertMetricRegisteredAndHasExpectedValue(t, metricsServer.registry, "alive", 1.0)
}
