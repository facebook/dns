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
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetricsServer contains the struct for the PrometheusMetricsServer
type PrometheusMetricsServer struct {
	registry *prometheus.Registry
	addr     string
	stats    map[string]*Stats
}

// NewMetricsServer creates a PrometheusMetricsServer
func NewMetricsServer(addr string) (server *PrometheusMetricsServer, err error) {
	server = &PrometheusMetricsServer{
		registry: prometheus.NewRegistry(), addr: addr, stats: make(map[string]*Stats),
	}
	server.registry.MustRegister(collectors.NewBuildInfoCollector())
	server.registry.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollections(collectors.GoRuntimeMemStatsCollection | collectors.GoRuntimeMetricsCollection),
	))

	return server, nil
}

// Serve sets up  and starts the prometheus http server
func (s *PrometheusMetricsServer) Serve() error {
	glog.Infof("Starting prometheus metrics server at %q\n", s.addr)
	http.Handle("/metrics", promhttp.HandlerFor(
		s.registry,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))
	return http.ListenAndServe(s.addr, nil)
}

// SetAlive adds the alive metric into the metrics registry
func (s *PrometheusMetricsServer) SetAlive() {
	status := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alive",
		Help: "Server running",
	})

	s.registry.MustRegister(status)
	status.Set(1.0)
}

// ConsumeStats registers a Stats instance to be added to the prometheus metrics registry
func (s *PrometheusMetricsServer) ConsumeStats(category string, stats *Stats) error {
	s.stats[category] = stats
	return nil
}

// UpdateExporter syncs the registered Stats instances to the metrics registry
func (s *PrometheusMetricsServer) UpdateExporter() {
	for range time.Tick(1 * time.Second) {
		for category, stats := range s.stats {
			metricsmap := stats.Get()
			for mkey, mval := range metricsmap {
				promCollector := prometheus.NewGauge(prometheus.GaugeOpts{
					Namespace: flattenKey(category),
					Name:      flattenKey(mkey),
					Help:      mkey,
				})
				err := s.registry.Register(promCollector)
				if err != nil {
					switch err.(type) {
					case prometheus.AlreadyRegisteredError:
						alreadyexistingerror := err.(prometheus.AlreadyRegisteredError)
						promCollector = alreadyexistingerror.ExistingCollector.(prometheus.Gauge)
					default:
						glog.Errorf("failed to register metric %s %v", mkey, err)
						continue
					}
				}
				promCollector.Set(float64(mval))

			}
		}
	}
}

func flattenKey(key string) string {
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, "=", "_")
	key = strings.ReplaceAll(key, "/", "_")
	return key
}
