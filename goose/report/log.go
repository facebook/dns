package report

import (
	"github.com/facebook/dns/goose/stats"

	log "github.com/sirupsen/logrus"
)

// LogStatsReporter is a reporter that reports to log
type LogStatsReporter struct{}

// Initialize does nothing, just to meet the interface requirements
func (r *LogStatsReporter) Initialize() error {
	return nil
}

// ReportMetrics sends metric to log
func (r *LogStatsReporter) ReportMetrics(exportedMetrics *stats.ExportedMetrics) error {
	aggregatedLatencyStats := exportedMetrics.AggregateLatencies()
	log.Infof(
		`Response Latency Data:(S/F: %v/%v) Max: %v Min: %v Mean: %v Median: %v Upper Quartile: %v Lower Quartile: %v`,
		exportedMetrics.Processed, exportedMetrics.Errors, toTime(aggregatedLatencyStats.Max), toTime(aggregatedLatencyStats.Min), toTime(aggregatedLatencyStats.Mean),
		toTime(aggregatedLatencyStats.Median), toTime(aggregatedLatencyStats.Upperq), toTime(aggregatedLatencyStats.Lowerq),
	)
	log.Infof("Requests: Successful: %v Failed: %v", exportedMetrics.Processed, exportedMetrics.Errors)
	log.Infof("Elapsed: %v", exportedMetrics.Elapsed)
	return nil
}
