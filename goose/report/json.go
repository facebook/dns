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

package report

import (
	"encoding/json"
	"os"
	"time"

	"github.com/facebook/dns/goose/stats"
)

// JSONStatsReporter is a reporter that reports to log
type JSONStatsReporter struct{}

type jsonPrintableMetrics struct {
	//Elapsed is the elapsed time duration
	Elapsed time.Duration
	// processed is the number of queries successfully processed.
	Processed int
	// errors is the number of queries that failed.
	Errors  int
	Min     float64
	Max     float64
	Mean    float64
	Median  float64
	Lowerq  float64
	Upperq  float64
	Average float64
}

// Initialize does nothing, just to meet the interface requirements
func (r *JSONStatsReporter) Initialize() error {
	return nil
}

// ReportMetrics sends metric to stdout as json
func (r *JSONStatsReporter) ReportMetrics(exportedMetrics *stats.ExportedMetrics) error {
	aggregatedLatencyStats := exportedMetrics.AggregateLatencies()
	return json.NewEncoder(os.Stdout).Encode(jsonPrintableMetrics{
		Elapsed:   exportedMetrics.Elapsed,
		Processed: exportedMetrics.Processed,
		Errors:    exportedMetrics.Errors,
		Min:       aggregatedLatencyStats.Min,
		Max:       aggregatedLatencyStats.Max,
		Mean:      aggregatedLatencyStats.Mean,
		Median:    aggregatedLatencyStats.Median,
		Lowerq:    aggregatedLatencyStats.Lowerq,
		Upperq:    aggregatedLatencyStats.Upperq,
		Average:   aggregatedLatencyStats.Average,
	})
}
