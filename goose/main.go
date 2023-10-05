package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/facebook/dns/goose/query"
	"github.com/facebook/dns/goose/report"
	"github.com/facebook/dns/goose/stats"
	"github.com/miekg/dns"

	"go.uber.org/ratelimit"

	log "github.com/sirupsen/logrus"
)

const pprofHTTP = "localhost:6060"

var (
	daemon       bool
	logLevel     string
	totalQueries int
	dport        int
	// names   string
	domain              string
	host                string
	logging             bool
	timeout             time.Duration
	duration            time.Duration
	samplingInterval    time.Duration
	debugger            bool
	maxqps              int
	randomiseQueries    bool
	qTypeStr            string
	monitorPort         int
	monitorHost         string
	parallelConnections int
	reportJSON          bool
	inputFile           string
	exporterAddr        string
)

func main() {
	flag.BoolVar(&daemon, "daemon", false,
		"Running in daemon mode means that metrics will be exported rather than printed to stdout")
	flag.StringVar(&logLevel, "loglevel", "info", "Set a log level. Can be: debug, info, warning, error")
	flag.IntVar(&totalQueries, "total-queries", 50000, "Total queries to send")
	flag.IntVar(&dport, "port", 53, "destination port")
	flag.StringVar(&domain, "domain", "", "Domain for uncached queries")
	flag.StringVar(&qTypeStr, "query-type", "A", "Query type to be used for the query")
	flag.StringVar(&inputFile, "input-file", "", "The file that contains queries to be made in qname qtype format")
	flag.StringVar(&host, "host", "127.0.0.1", "IP address of DNS server to test")
	flag.BoolVar(&logging, "enable-logging", true, "Whether to enable logging or not")
	flag.BoolVar(&randomiseQueries, "randomise-queries", false, "Whether to randomise dns queries to bypass potential caching")
	flag.IntVar(&monitorPort, "monitor-port", 8953, "DNS queries not sent if this port is down on the monitored host (defaults to unbound remote-control port)")
	flag.StringVar(&monitorHost, "monitor-host", "127.0.0.1", "DNS queries not sent if the monitored port on this host is down")
	flag.StringVar(&exporterAddr, "exporter-addr", ":6869", "Exporter bind address")
	flag.DurationVar(&duration, "max-duration", 0*time.Second, "Maximum duration of test (seconds)")
	flag.DurationVar(&timeout, "timeout", 3*time.Second, "Duration of timeout for queries")
	flag.DurationVar(&samplingInterval, "sample", 0*time.Second, "Sampling frequency for reporting (seconds)")
	flag.BoolVar(&debugger, "pprof", false, "Enable pprof")
	flag.IntVar(&maxqps, "max-qps", 0, "max number of QPS")
	flag.IntVar(&parallelConnections, "parallel-connections", 1, "max number of parallel connections")
	flag.BoolVar(&reportJSON, "report-json", false, "Report run results to stdout in json format")
	flag.Parse()

	// Set the seed
	rand.Seed(time.Now().UTC().UnixNano())

	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("Unrecognized log level: %v", logLevel)
	}

	if domain == "" && inputFile == "" {
		log.Fatal("Need to specify either domain or input file, neither is specified")

	}
	if domain != "" && inputFile != "" {
		log.Fatal("Need to specify either domain or input file, both are specified, please only specify one of them")

	}
	qnames := make([]string, 0)
	qtypes := make([]dns.Type, 0)
	var err error
	if inputFile != "" {
		qnames, qtypes, err = query.ProcessQueryInputFile(inputFile)
		if err != nil {
			log.Fatalf("Failed to process query input file: %s %v", inputFile, err)
		}
	} else {
		qnames = []string{domain}
		qType, qtypeErr := query.QTypeStrToDNSQtype(qTypeStr)
		if qtypeErr != nil {
			log.Fatalf("%v", err)
		}
		qtypes = []dns.Type{qType}
	}
	sigStop := make(chan os.Signal, 1)
	sigPause := make(chan struct{}, 1)

	var reporter stats.Reporter = &report.LogStatsReporter{}
	if reportJSON {
		reporter = &report.JSONStatsReporter{}
	}
	// Do not stop unless interrupted
	signal.Notify(sigStop, syscall.SIGINT)  // ^C (Control-C).
	signal.Notify(sigStop, syscall.SIGQUIT) // ^\ (Control-Backslash)
	signal.Notify(sigStop, syscall.SIGTERM) // kill/pkill etc

	if debugger {
		log.Infof("Starting profiler on %s", pprofHTTP)
		go func() {
			listenError := http.ListenAndServe(pprofHTTP, nil)
			if listenError != nil {
				log.Errorf("failed to start pprof: %s", listenError.Error())
			}
		}()
	}

	if daemon {
		log.Infof("Running in daemon mode")
		log.Infof("Monitor Host/Port is %s:%d", monitorHost, monitorPort)
		query.MonitorTarget(sigPause, monitorPort, monitorHost)
		// @fb-only: reporter = &report.ODSMetricsReporter{Prefix: "dns.goose", Addr: exporterAddr}
		reporter = &report.PrometheusMetricsReporter{Addr: exporterAddr} // @oss-only

		// Do nothing on SIGHUP (terminal disconnect)
		signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)

	} else {
		log.Infof("The total number of DNS requests will be: %v", totalQueries)
	}

	goosestr := `
	 _______________________________
	< Honking at %s:%d with %s QPS.>
	--------------------------------
	 \
	  \
	   \
			        ___
			      .^   ""-.
			  _.-^( e   _  '.
		   	'-===.>_.-^ '.   "
				  "	 "
				  :    "
				  :    |   __.--._
				  |    '--"       ""-._    _.^)
				 /                     ""-^  _>
				:                          -^>
				:                 .__>    __)
				 \	 '._      .__.-'  .-'
				  '.___    '-.__.-'       /
				   '-.__    .    _.'    /
				      \_____> >'.__/_.""
				    .'.----'  | |
				  .' /        | |
			  	'^-/       ___| :
					   >--  /
					   >.'.'
					   '-^`
	go func() {
		reporterInitError := reporter.Initialize()
		if reporterInitError != nil {
			log.Errorf("Failed to initialize stats reporter %v", reporterInitError)
		}
	}()
	var rate ratelimit.Limiter
	qpsStr := "Unlimited"
	if maxqps > 0 {
		log.Infof("Limiting max qps to: %d", maxqps)
		rate = ratelimit.New(maxqps)
		qpsStr = fmt.Sprint(maxqps)
	} else {
		rate = ratelimit.NewUnlimited()
	}
	log.Infof(goosestr, host, dport, qpsStr)
	runState := query.NewRunState(totalQueries, rate, daemon, time.Now)
	if duration != 0 && !daemon {
		timer := time.NewTimer(duration)
		go func() {
			<-timer.C
			log.Info("Max duration reached. Exiting.")
			close(sigStop)
		}()
	}
	periodicReporterStop := make(chan os.Signal, 1)
	if samplingInterval != 0 {
		log.Infof("Sampling interval is %v", samplingInterval)
		samplingTicker := time.NewTicker(samplingInterval)
		go func() {
			for {
				select {
				case <-samplingTicker.C:
					sampledMetrics := runState.ExportIntermediateResults()
					repErr := reporter.ReportMetrics(sampledMetrics)
					if err != nil {
						log.Errorf("Failed to report metrics %v", repErr)
					}
				case <-periodicReporterStop:
					return
				}
			}
		}()
	}

	go func() {

		log.Infof(
			"Starting the test and running %d connections in parallel",
			parallelConnections,
		)
		var wg sync.WaitGroup
		for i := 0; i < parallelConnections; i++ {
			wg.Add(1)
			go func() {
				qErr := query.RunQueries(dport, host, qnames, timeout, randomiseQueries, qtypes, time.Now, runState, sigPause)
				if err != nil {
					log.Errorf("Failed to run queries %v", qErr)
				}
				wg.Done()
			}()
		}
		wg.Wait()

		log.Info("Finished running all connections")
		close(sigStop)
	}()
	select { // nolint: gosimple
	case <-sigStop:
		log.Infof("No more requests will be sent")
		close(periodicReporterStop)
	}
	if !daemon {
		log.Info("The test results are:")

		finishedMetrics := runState.ExportResults()
		err = reporter.ReportMetrics(finishedMetrics)
		if err != nil {
			log.Errorf("Failed to report metrics %v", err)
		}
	}
}
