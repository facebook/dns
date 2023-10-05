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

package report

import (
	"net/http"

	"github.com/facebook/dns/goose/stats"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
)

// PrometheusMetricsReporter contains the struct for the PrometheusMetricsReporter
type PrometheusMetricsReporter struct {
	Addr               string
	registry           *prometheus.Registry
	successGauge       prometheus.Gauge
	failedGauge        prometheus.Gauge
	maxLatancyGauge    prometheus.Gauge
	meanLatencyGauge   prometheus.Gauge
	medianLatencyGauge prometheus.Gauge
	minLatencyGauge    prometheus.Gauge
	avgLatencyGauge    prometheus.Gauge
}

// Initialize sets up  and starts the prometheus http server
func (r *PrometheusMetricsReporter) Initialize() error {
	r.registry = prometheus.NewRegistry()
	r.successGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "success",
		Help:      "Number of successful queries sent",
	})
	r.failedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "failed",
		Help:      "Number of failed queries sent",
	})
	r.maxLatancyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "latency_max",
		Help:      "Max query latency in milliseconds",
	})
	r.meanLatencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "latency_mean",
		Help:      "Mean query latency in milliseconds",
	})
	r.medianLatencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "latency_median",
		Help:      "Median query latency in milliseconds",
	})
	r.minLatencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "latency_min",
		Help:      "Min query latency in milliseconds",
	})
	r.avgLatencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dns_goose",
		Name:      "latency_avg",
		Help:      "Average query latency in milliseconds",
	})

	r.registry.MustRegister(r.successGauge)
	r.registry.MustRegister(r.failedGauge)
	r.registry.MustRegister(r.maxLatancyGauge)
	r.registry.MustRegister(r.meanLatencyGauge)
	r.registry.MustRegister(r.medianLatencyGauge)
	r.registry.MustRegister(r.minLatencyGauge)
	r.registry.MustRegister(r.avgLatencyGauge)

	log.Infof("Starting prometheus metrics server at %q\n", r.Addr)
	http.Handle("/metrics", promhttp.HandlerFor(
		r.registry,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))
	return http.ListenAndServe(r.Addr, nil)
}

// ReportMetrics registers the metrics in fb303 for collection
func (r *PrometheusMetricsReporter) ReportMetrics(exportedMetrics *stats.ExportedMetrics) error {
	aggregatedLatencyStats := exportedMetrics.AggregateLatencies()
	r.successGauge.Set(float64(exportedMetrics.Processed))
	r.failedGauge.Set(float64(exportedMetrics.Errors))
	r.avgLatencyGauge.Set(aggregatedLatencyStats.Average)
	r.maxLatancyGauge.Set(aggregatedLatencyStats.Max)
	r.meanLatencyGauge.Set(aggregatedLatencyStats.Mean)
	r.medianLatencyGauge.Set(aggregatedLatencyStats.Median)
	r.minLatencyGauge.Set(aggregatedLatencyStats.Min)
	return nil
}
