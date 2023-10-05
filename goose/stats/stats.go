package stats

import (
	"sort"
	"time"

	"gonum.org/v1/gonum/stat"
)

// Reporter is used to report stats
type Reporter interface {
	ReportMetrics(*ExportedMetrics) error
	Initialize() error
}

func average(samples []float64) float64 {
	total := float64(0)
	for _, x := range samples {
		total += x
	}
	return total / float64(len(samples))
}

// ExportedMetrics holds the basic metrics returned by the query engine
type ExportedMetrics struct {
	Elapsed time.Duration
	// processed is the number of queries successfully processed.
	Processed int
	// errors is the number of queries that failed.
	Errors int
	// Latencies contain per query latency
	Latencies []float64
}

// QPSTotal returns the number of queries processed in one second.
func (m *ExportedMetrics) QPSTotal() (q float64) {
	e := m.Elapsed
	return float64(m.Processed+m.Errors) / e.Seconds()
}

// LatencyStats stores latency data statistics
type LatencyStats struct {
	Min     float64
	Max     float64
	Mean    float64
	Median  float64
	Lowerq  float64
	Upperq  float64
	Average float64
}

func newLatencyStats() *LatencyStats {
	return &LatencyStats{0, 0, 0, 0, 0, 0, 0}
}

// AggregateLatencies aggregates query latency metrics
func (m *ExportedMetrics) AggregateLatencies() *LatencyStats {
	l := newLatencyStats()
	sort.Float64s(m.Latencies)
	if len(m.Latencies) > 0 {
		l.Min = stat.Quantile(0.0, stat.Empirical, m.Latencies, nil)
		l.Max = stat.Quantile(1.0, stat.Empirical, m.Latencies, nil)
		l.Mean = stat.Mean(m.Latencies, nil)
		l.Median = stat.Quantile(0.5, stat.Empirical, m.Latencies, nil)
		l.Upperq = stat.Quantile(0.75, stat.Empirical, m.Latencies, nil)
		l.Lowerq = stat.Quantile(0.25, stat.Empirical, m.Latencies, nil)
		l.Average = average(m.Latencies)
	}
	return l
}
