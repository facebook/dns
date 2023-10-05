package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_latencyAggregator(t *testing.T) {
	e := &ExportedMetrics{Elapsed: 3 * time.Second, Processed: 8, Errors: 0, Latencies: []float64{25.01, 35.02, 10.03, 17.04, 29.05, 14.06, 21.07, 31.08}}
	w := e.AggregateLatencies()

	require.Equal(t, float64(10.03), w.Min)
	require.Equal(t, float64(35.02), w.Max)
	require.Equal(t, float64(22.795), w.Mean)
	require.Equal(t, float64(21.07), w.Median)
	require.Equal(t, float64(14.06), w.Lowerq)
	require.Equal(t, float64(29.05), w.Upperq)
}
