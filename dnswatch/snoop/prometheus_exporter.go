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

package snoop

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	aQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "a_queries",
		Help: "The number of A queries",
	},
		[]string{"process"},
	)
	aaaaQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aaaa_queries",
		Help: "The number of AAAA queries",
	},
		[]string{"process"},
	)
	ptrQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ptr_queries",
		Help: "The number of PTR queries",
	},
		[]string{"process"},
	)
	servfailResps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "servfail_responses",
		Help: "The number of SERVFAIL responses",
	},
		[]string{"process"},
	)
	nxdomainResps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nxdomain_responses",
		Help: "The number of NXDOMAIN responses",
	},
		[]string{"process"},
	)
	noerrorResps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "noerror_responses",
		Help: "The number of NOERROR responses",
	},
		[]string{"process"},
	)
)

func startPrometheusExporter(refreshChan <-chan *ToplikeData, prometheusBind string) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Infof("Prometheus DNSWatch exporter listening on %s", prometheusBind)
		err := http.ListenAndServe(prometheusBind, nil)
		if err != nil {
			log.Errorf("failed to listen on %s\n:", prometheusBind)
		}
	}()
	for {
		t := <-refreshChan
		aQueries.WithLabelValues("all").Add(float64(t.a))
		aaaaQueries.WithLabelValues("all").Add(float64(t.aaaa))
		ptrQueries.WithLabelValues("all").Add(float64(t.ptr))
		servfailResps.WithLabelValues("all").Add(float64(t.servf))
		nxdomainResps.WithLabelValues("all").Add(float64(t.nxdom))
		noerrorResps.WithLabelValues("all").Add(float64(t.noerr))
		for _, row := range t.aggregateComm().Rows {
			aQueries.WithLabelValues(row.Comm).Add(float64(row.A.val))
			aaaaQueries.WithLabelValues(row.Comm).Add(float64(row.AAAA.val))
			ptrQueries.WithLabelValues(row.Comm).Add(float64(row.PTR.val))
			servfailResps.WithLabelValues(row.Comm).Add(float64(row.SERVF.val))
			nxdomainResps.WithLabelValues(row.Comm).Add(float64(row.NXDOM.val))
			noerrorResps.WithLabelValues(row.Comm).Add(float64(row.NOERR.val))
		}
	}
}
