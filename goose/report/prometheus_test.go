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
	exportedMetrics := &stats.ExportedMetrics{Elapsed: 100 * time.Second, Processed: 1, Errors: 0, Latencies: []float64{1000, 2000, 3000}}
	r := &PrometheusMetricsReporter{Addr: ":0"}
	go func() {
		_ = r.Initialize()
	}()
	time.Sleep(1 * time.Millisecond)
	err := r.ReportMetrics(exportedMetrics)
	require.NoError(t, err)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_response_success", 1)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_response_error", 0)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_max_us", 3)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_min_us", 1)
	requireMetricRegisteredAndHasExpectedValue(t, r.registry, "dns_goose_latency_avg_us", 2)
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
