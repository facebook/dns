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
	"math"
	"time"
)

var (
	errors        = "response.error"
	latencyMax    = "latency.max.us"
	latencyMean   = "latency.mean.us"
	latencyMedian = "latency.median.us"
	latencyMin    = "latency.min.us"
	latencyAvg    = "latency.avg.us"
	successes     = "response.success"
)

func toTime(t float64) time.Duration {
	return time.Duration(t) * time.Nanosecond
}

// The float64s sent to ODS are denominated in microseconds
func toMicro(t float64) int {
	return int(math.Round(t / float64(1000)))
}
