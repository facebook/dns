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

package main

import (
	"errors"
	"flag"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/facebook/dns/dnsrocks/dnsdata/quote"
	"github.com/facebook/dns/dnsrocks/fbserver"
	"github.com/facebook/dns/dnsrocks/logger"
	"github.com/facebook/dns/dnsrocks/metrics"

	"github.com/golang/glog"

	_ "net/http/pprof"
)

func setCPU(cpu string) (int, error) {
	var numCPU int

	availCPU := runtime.NumCPU()

	if strings.HasSuffix(cpu, "%") {
		// Percent
		var percent float32
		pctStr := cpu[:len(cpu)-1]
		pctInt, err := strconv.Atoi(pctStr)
		if err != nil || pctInt < 1 || pctInt > 100 {
			return -1, errors.New("invalid CPU value: percentage must be between 1-100")
		}
		percent = float32(pctInt) / 100
		numCPU = int(float32(availCPU) * percent)
	} else {
		// Number
		num, err := strconv.Atoi(cpu)
		if err != nil || num < 1 {
			return -1, errors.New("invalid CPU value: provide a number or percent greater than 0")
		}
		numCPU = num
	}

	if numCPU > availCPU {
		numCPU = availCPU
	}

	runtime.GOMAXPROCS(numCPU)
	return numCPU, nil
}

func main() {
	var serverConfig = fbserver.NewServerConfig()
	var loggerConfig logger.Config
	var doTTLSATtl uint64
	var metricsAddr, thriftAddr string
	var toStderr bool
	var verbosity int
	const DefaultMetricsAddr string = ":18888"
	cliflags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// DNS Server config
	cliflags.IntVar(&serverConfig.Port, "port", 8053, "port to run on")
	cliflags.IntVar(&serverConfig.MaxUDPSize, "max-udp-size", 0, "Maximum UDP response size (default: none)")
	cliflags.BoolVar(&serverConfig.TCP, "tcp", true, "Whether or not to also listen on TCP.")
	cliflags.IntVar(&serverConfig.MaxTCPQueries, "tcp-max-queries", -1, "Maximum number of queries handled on a single TCP connection before closing the socket. This also applies for TLS. (unlimited if -1).")
	// Idle Timeout default is based on miekg/dns original default: https://fburl.com/t0tmjp2c
	cliflags.DurationVar(&serverConfig.TCPIdleTimeout, "tcp-idle-timeout", 8*time.Second, "TCP/TLS connections idle timeout. A connection TCP connection will be torn down if the TCP connection is idle for that time after first read.")
	cliflags.DurationVar(&serverConfig.ReadTimeout, "read-timeout", 2*time.Second, "Sets the deadline for future Read calls and any currently-blocked Read call. A zero value means Read will not time out. For TCP, this value only applied to first read.")

	cliflags.IntVar(&serverConfig.ReusePort, "reuse-port", 0, "Whether or not to use SO_REUSEPORT when opening listeners. X = 0 to disable and start only 1 listener without SO_REUSEPORT, X > 0 to start X listeners with SO_REUSEPORT.")
	cliflags.StringVar(&serverConfig.WhoamiDomain, "whoami-domain", "", "Domain name to answer debug queries. If empty, the functionality is disabled (default disabled)")
	cliflags.BoolVar(&serverConfig.NSID, "nsid", false, "Flag to enable NSID responses with debug info (default: disabled)")
	cliflags.BoolVar(&serverConfig.PrivateInfo, "private-info", false, "Flag to add encrypted debug info (default: disabled)")
	cliflags.BoolVar(&serverConfig.RefuseANY, "refuse-any", false, "Whether or not to refuse ANY queries.")
	// the default setup should be backward compatible with current spec: 1 IP address and maxanswer not specified
	cliflags.Var(&serverConfig.IPAns, "ip", "IPs to bind to. Usage: -ip=::1 -ip=127.0.0.1 (default is wildcard)")
	cliflags.Var(&serverConfig.IPAns, "ipwithmaxans", "Max number of answers returned by query for each ip, separated by comma. Usage: -ipwithmaxans 192.0.2.53,1  -ipwithmaxans 192.0.2.35,8")

	// DNSSEC
	cliflags.StringVar(&serverConfig.DNSSECConfig.Zones, "dnssec-zones", "", "Comma separated list of zones for which DNSSEC is enabled.")
	cliflags.StringVar(&serverConfig.DNSSECConfig.Keys, "dnssec-keys", "", "Comma separated list of DNSSEC keyfile, as generated by `dnssec-keygen -a ECDSAP256SHA256 <zonename>`, to use for DNSSEC signing. Example: Kexample.com.+013+28484")
	// Handler Config
	cliflags.BoolVar(&serverConfig.HandlerConfig.AlwaysCompress, "alwaysCompress", false, "Enable unconditional compression of labels in server responses")
	cliflags.BoolVar(&serverConfig.HandlerConfig.CNAMEChasing, "cname-chasing", false, "Whether or not to do CNAME chasing. (default: disabled)")

	// DB config
	cliflags.IntVar(&serverConfig.DBConfig.ReloadInterval, "reloadtime", 10, "Time between each CDB reload")
	cliflags.DurationVar(&serverConfig.DBConfig.ReloadTimeout, "reloadtimeout", time.Second, "Time to wait for DB to finish reload")
	cliflags.BoolVar(&serverConfig.DBConfig.WatchDB, "watchdb", false, "Watch DB file change and reload")
	cliflags.StringVar(&serverConfig.DBConfig.Path, "dbpath", "./rocksdb", "Path to the database")
	cliflags.StringVar(&serverConfig.DBConfig.ControlPath, "control-path", "",
		`Path to the control directory. When not empty, FBDNS watches given directory for trigger files that control DB reloads.
Currently two types of trigger files are supported:
* 'switchdb' - full reload trigger file, must contain new DB path as a text in it
* 'reload' - partial reload (WAL catchup) trigger file, content of the file is ignored`)
	cliflags.StringVar(&serverConfig.DBConfig.Driver, "dbdriver", "rocksdb", "Name of the database engine to use (cdb, rocksdb)")

	// Cache config
	cliflags.BoolVar(&serverConfig.CacheConfig.Enabled, "cache", false, "Whether or not we should cache DNS messages")
	cliflags.IntVar(&serverConfig.CacheConfig.LRUSize, "cache-lru-size", 1024*1024, "LRU cache size")
	cliflags.Int64Var(&serverConfig.CacheConfig.WRSTimeout, "cache-wrs-timeout", 0, "How long should the weighted random sampled DNS messages should be cached. 0 to not cache them.")
	// TLS Config
	cliflags.BoolVar(&serverConfig.TLS, "tls", false, "Whether or not to also listen on TCP with TLS.")
	cliflags.IntVar(&serverConfig.TLSConfig.Port, "tls-port", 8853, "Port to run DNS-over-TLS on.")
	cliflags.StringVar(&serverConfig.TLSConfig.CertFile, "tls-cert-file", "", "Path to TLS cert file")
	cliflags.StringVar(&serverConfig.TLSConfig.KeyFile, "tls-key-file", "", "Path to TLS key file")
	cliflags.StringVar(&serverConfig.TLSConfig.CryptoSSL.Tier, "tls-cryptossl-tier", "", "Name of CryptoSSL tier.")
	cliflags.StringVar(&serverConfig.TLSConfig.CryptoSSL.CertName, "tls-cryptossl-cert-name", "", "Name of certificate to use.")
	cliflags.StringVar(&serverConfig.TLSConfig.SessionTicketKeys.SeedFile, "tls-seed-file", "", "Path to the file containing TLS tickets seeds.")
	cliflags.IntVar(&serverConfig.TLSConfig.SessionTicketKeys.SeedFileReloadInterval, "tls-seed-file-reload-interval", 60, "Interval at which to reload TLS Session Ticket Keys seeds.")
	cliflags.BoolVar(&serverConfig.TLSConfig.DoTTLSAEnabled, "tls-tlsa-record", false, "Whether or not to enable the handler to distribute TLS SPKI using DANE/TLSA")
	cliflags.Uint64Var(&doTTLSATtl, "tls-tlsa-record-ttl", 0, "TTL to use with DoT TLSA records. A value of 0 will let the plugin use its default (currently 3600)")
	// Loggers
	cliflags.StringVar(&loggerConfig.Target, "dnstap-target", "stdout", "DNSTap destination to write to. Use `stdout` for Stdout, `unix` for unix socket and `tcp` for tcp socket (stdout, tcp, unix)")
	cliflags.StringVar(&loggerConfig.Remote, "dnstap-remote", "", "DNSTap remote to write to. Provide ip:port or path-to-unix-socket")
	cliflags.StringVar(&loggerConfig.LogFormat, "dnstap-stdout-format", "text", "DNSTap log format, only in use for the `stdout` target (text, yaml, json)")
	cliflags.IntVar(&loggerConfig.Timeout, "dnstap-timeout", 1, "Timeout before dnstap client fails to connect to remote.")
	cliflags.IntVar(&loggerConfig.Retry, "dnstap-retry", 3, "Time between dnstap client reconnection attempts.")
	cliflags.IntVar(&loggerConfig.FlushInterval, "dnstap-flush-interval", 5, "Maximum time data will be kept in the output buffer.")
	cliflags.Float64Var(&loggerConfig.SamplingRate, "dnstap-sampling-rate", 1.0, "What rate of queries are being sampled in. Value should be [0.0, 1.0]. 1.0 means logging everything. The value will be coerced to the closest 1/N value")
	// scribe related config flags. To maintain cli flag compatibility
	cliflags.Float64Var(&loggerConfig.SamplingRate, "scribe-sampling-rate", 1.0, "What rate of queries are being sampled in. Value should be [0.0, 1.0]. 1.0 means logging everything. The value will be coerced to the closest 1/N value")
	cliflags.StringVar(&loggerConfig.Category, "scribe-category", "-", "Scribe category to write to. Use `-` for Stdout.")
	cliflags.IntVar(&loggerConfig.Timeout, "scribe-timeout", 1, "Timeout before scribecat client fails to connect to scribed.")
	cliflags.IntVar(&loggerConfig.Retry, "scribe-retries", 3, "Number of times scribecat client will attempt to flush messages before giving up and dropping them.")
	cliflags.IntVar(&loggerConfig.FlushInterval, "scribe-flush-interval", 5, "Interval at which the scribecat client will flush logs to scribed.")
	cliflags.StringVar(&metricsAddr, "metrics-addr", DefaultMetricsAddr, "Where to serve metrics from")
	// Just needed to maintain cli flag compatibility, for now
	cliflags.StringVar(&thriftAddr, "thrift-addr", DefaultMetricsAddr, "Where to serve thrift from")
	// Misc
	pprofconf := cliflags.String("pprof", "", "Address to have the profiler listen on, disabled if empty.")
	cpu := cliflags.String("cpu", "1", "CPU cap. Accepts percentage or integer.")
	cliflags.IntVar(&serverConfig.MaxConcurrency, "max-concurrency", -1, "Maximum number of concurrent queries per CPU (default: unlimited)")
	logPrefix := cliflags.String("log-prefix", "", "Prefix to use in logger")
	dnsRecordKeyToValidate := cliflags.String("record-key-to-validate", "", "DNS record key expected to present in DB file.")

	version := cliflags.Bool("version", false, "Print versioning information.")

	// Enable glog format (already defined by glog lib)
	// This hack is required for glog compatibility, as it does not expose verbosity level
	cliflags.BoolVar(&toStderr, "logtostderr", true, "log to standard error instead of files")
	cliflags.IntVar(&verbosity, "v", 2, "log level for V logs")
	err := cliflags.Parse(os.Args[1:])
	if err != nil {
		glog.Errorf("Failed to parse cli flags: %v", err)
	}
	err = flag.Set("logtostderr", strconv.FormatBool(toStderr))
	if err != nil {
		glog.Errorf("Failed to set glog logging to stdout. Err: %v", err)
	}
	err = flag.Set("v", strconv.FormatInt(int64(verbosity), 10))
	if err != nil {
		glog.Errorf("Failed to set glog verbosity level to 2. Err: %v", err)
	}
	flag.CommandLine = cliflags
	flag.Parse()
	// glog cli flag hack over.

	if thriftAddr != DefaultMetricsAddr {
		metricsAddr = thriftAddr
	}
	if doTTLSATtl > math.MaxUint32 {
		glog.Fatalf("tls-tlsa-record-ttl %d is greater than max uint32: %d", doTTLSATtl, math.MaxUint32)
	}
	serverConfig.TLSConfig.DoTTLSATtl = uint32(doTTLSATtl)
	serverConfig.DBConfig.Path = path.Clean(serverConfig.DBConfig.Path)
	unquotedKey, err := quote.Bunquote([]byte(*dnsRecordKeyToValidate))
	if err != nil {
		glog.Fatalf("Failed to unquote validation dns record: '%s', %v\n", *dnsRecordKeyToValidate, err)
	}
	serverConfig.DBConfig.ValidationKey = unquotedKey

	if *version {
		glog.Infof("go version: %s go arch: %s go OS: %s", runtime.Version(), runtime.GOARCH, runtime.GOOS)
		os.Exit(0)
	}
	serverConfig.NumCPU, err = setCPU(*cpu)
	failOnErr(err, "Error setting number of CPU")

	// TODO (jifen) this should be deprecated in subsequent
	// diff since IDN no longer rely on this
	if len(*logPrefix) > 0 {
		glog.Warningf("Provided prefix %s but not used", *logPrefix)
	}

	if *pprofconf != "" {
		go func() {
			err = http.ListenAndServe(*pprofconf, nil)
			if err != nil {
				glog.Errorf("Failed to start pprof. Err: %v", err)
			}
		}()
	}

	// Metrics server
	metricsServer, err := metrics.NewMetricsServer(metricsAddr)
	if err != nil {
		glog.Fatalf("cannot initialize metrics server: %s\n", err)
	}

	go func() {
		if serverError := metricsServer.Serve(); serverError != nil {
			glog.Fatalf("cannot start metrics server: %s\n", serverError)
		}
	}()

	// Logger
	l, err := logger.NewLogger(loggerConfig)
	if err != nil {
		glog.Fatalf("Error creating dnstap logger, invalid configuration provided: %s\n", err)
	}
	l.StartLoggerOutput()

	// stat collector
	stats := metrics.NewStats()

	srv := fbserver.NewServer(serverConfig, l, stats, metricsServer)

	if len(*dnsRecordKeyToValidate) > 0 {
		err = srv.ValidateDbKey(unquotedKey)
		if err != nil {
			failOnErr(err, "Invalid DB file, expected record not present.")
		}
	}
	// NotifyStartedFunc is used to notify wait group that servers are started
	srv.NotifyStartedFunc = func() {
		srv.ServersStartedWG.Done()
	}
	failOnErr(srv.Start(), "Failed to start servers")

	// It is necessary to set NotifyStartedFunc to call Done() on wait group, otherwise
	// this will block and status will never be changed
	go func() {
		srv.ServersStartedWG.Wait()
		metricsServer.SetAlive()
	}()
	err = metricsServer.ConsumeStats("dns", stats)
	if err != nil {
		glog.Errorf("Failed to register stats for consumption: %v. Err: %v", stats, err)
	}
	go metricsServer.UpdateExporter()

	hangupchan := make(chan os.Signal, 1)
	signal.Notify(hangupchan, syscall.SIGHUP)
	go func() {
		for range hangupchan {
			glog.Info("SIGHUP received, refreshing database")
			srv.ReloadDB()
		}
	}()

	if serverConfig.DBConfig.WatchDB {
		go srv.WatchDBAndReload()
	}

	if serverConfig.DBConfig.ControlPath != "" {
		go srv.WatchControlDirAndReload()
	}

	go srv.LogMapAge()
	go srv.DumpBackendStats()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	glog.Infof("Signal (%v) received, stopping\n", s)

	srv.Shutdown()
}

func failOnErr(err error, msg string) {
	if err != nil {
		glog.Fatalf("%s: %v\n", msg, err)
	}
}
