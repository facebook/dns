package report

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/goose/stats"

	dto "github.com/prometheus/client_model/go"
)

func TestReportMetrics(t *testing.T) {
	exportedMetrics := &stats.ExportedMetrics{Elapsed: 100 * time.Second, Processed: 1, Errors: 0, Latencies: []float64{1, 2, 3}}
	r := &PrometheusMetricsReporter{Addr: ":0"}
	go r.Initialize()
	time.Sleep(1 * time.Millisecond)
	r.ReportMetrics(exportedMetrics)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_success", 1)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_failed", 0)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_max", 3)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_min", 1)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_avg", 2)
}

func requireMetricRegisteredAndHasExpectedValue(t *testing.T, registry *prometheus.Registry, metricKey string, expectedValue float64) {
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
