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
	successes     = "response.success"
)

func toTime(t float64) time.Duration {
	return time.Duration(t) * time.Nanosecond
}

// The float64s sent to ODS are denominated in microseconds
func toMicro(t float64) int {
	return int(math.Round(t / float64(1000)))
}
