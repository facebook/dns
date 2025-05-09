/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package fbserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/bufsize"
	"github.com/golang/glog"
	"github.com/miekg/dns"
	"golang.org/x/sys/unix"

	"github.com/facebook/dns/dnsrocks/db"
	"github.com/facebook/dns/dnsrocks/dnsserver"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
	"github.com/facebook/dns/dnsrocks/metrics"
	"github.com/facebook/dns/dnsrocks/nsid"
	"github.com/facebook/dns/dnsrocks/throttle"
	"github.com/facebook/dns/dnsrocks/tlsconfig"
	"github.com/facebook/dns/dnsrocks/whoami"
)

// Server collects all of the running servers and the server configurations.
type Server struct {
	conf            ServerConfig
	db              *dnsserver.FBDNSDB
	servers         []*dns.Server
	stats           stats.Stats
	metricsExporter anyMetricsExporter
	// If NotifyStartedFunc is set it is called once the server has started listening.
	NotifyStartedFunc func()

	// Wait group which can be used to wait for servers to initialize and start serving.
	// Client can wait for servers to start by invoking Done() method in NotifyStartedFunc
	// and wait for WaitGroup
	ServersStartedWG sync.WaitGroup
}

// DBTimestampInterval is the interval at which we update the DB timestamp, in seconds.
const DBTimestampInterval = 10

// BackendStatsInterval is the interval at which we report backend stats, in seconds.
const BackendStatsInterval = 10

const (
	// DBTimestampDBReadError occurs we cannot read DB.
	DBTimestampDBReadError = "db.timestamp.db_read_error"
	// DBTimestampInvalidTXT happens when a/the TXT record is not parseable to int64.
	DBTimestampInvalidTXT = "db.timestamp.invalid_txt"
	// DBTimestampKeySearchError means that searching the key itself failed. This is a DB lookup issue, not anything related to the key.
	DBTimestampKeySearchError = "db.timestamp.key_search_error"
	// DBTimestampKeyNotFound happens when we did not find the key, or we did not find any valid TXT record.
	DBTimestampKeyNotFound = "db.timestamp.key_not_found"
	// DBTimestampNumRun increments on every timestamp lookup.
	DBTimestampNumRun = "db.timestamp.num_run"
	// DBTimestamp is the actual key storing the db timestamp value.
	DBTimestamp = "db.timestamp"
	// DBFreshness is difference in seconds between db timestamp value and current time
	DBFreshness = "db.freshness"
)

// Parent key for tracking connection metrics for UDP, TCP, and TLS
const connectionKey = "dns.connection"

// record id.data.test for location \000\000 (e.g no location). We store a TXT
// record with the timestamp of when the DB was generated.
var dbTimestampName = []byte{2, 'i', 'd', 4, 'd', 'a', 't', 'a', 4, 't', 'e', 's', 't', 0}

type anyMetricsExporter interface {
	ConsumeStats(category string, stats *metrics.Stats) error
}

// NewServer start the server given a server config, a logger and a stat collector
func NewServer(conf ServerConfig, logger dnsserver.Logger, stats stats.Stats, metricsExporter anyMetricsExporter) *Server {
	// if no ip provided, use the default wildcard.
	if len(conf.IPAns) == 0 {
		conf.IPAns[""] = 1
	}

	tdb, err := dnsserver.NewFBDNSDB(conf.HandlerConfig, conf.DBConfig, conf.CacheConfig, logger, stats)
	failOnErr(err, "Error creating TinyDB handle")
	failOnErr(tdb.Load(), "Error loading TinyDB")
	return &Server{conf: conf, db: tdb, stats: stats, metricsExporter: metricsExporter}
}

// monitoredReader is a wrapper around dns default reader which serves to log the number of "read"
// operations by a listener, usually equaling the number of DNS queries received
type monitoredReader struct {
	rd dns.Reader
	m  *Monitor
}

// ReadTCP increments the appropriate counter, then calls the default implementation
func (v monitoredReader) ReadTCP(conn net.Conn, timeout time.Duration) ([]byte, error) {
	m, e := v.rd.ReadTCP(conn, timeout)
	if e == nil {
		v.m.incStat("Read")
	}
	return m, e
}

// ReadTCP increments the appropriate counter, then calls the default implementation
func (v monitoredReader) ReadUDP(conn *net.UDPConn, timeout time.Duration) ([]byte, *dns.SessionUDP, error) {
	m, s, e := v.rd.ReadUDP(conn, timeout)
	if e == nil {
		v.m.incStat("Read")
	}
	return m, s, e
}

// newMonitoredReader constructs a proper dns.DecorateReader from an existing Monitor
func newMonitoredReader(m *Monitor) dns.DecorateReader {
	return func(r dns.Reader) dns.Reader {
		return monitoredReader{r, m}
	}
}

// initUDPServer opens a monitored UDP socket and returns a DNS server ready
// for ActivateAndServe.
func (srv *Server) initUDPServer(addr string, h dns.Handler) (*dns.Server, error) {
	pc, err := listenUDP(addr, srv.listenConf())
	if err != nil {
		return nil, fmt.Errorf("failed to init UDP server: %w", err)
	}
	return &dns.Server{
		Addr:       addr,
		Net:        "udp",
		PacketConn: pc,
		Handler:    h,
	}, nil
}

// initTCPServer opens a monitored TCP socket and returns a DNS server ready
// for ActivateAndServe.
func (srv *Server) initTCPServer(addr string, h dns.Handler, s *metrics.Stats) (*dns.Server, error) {
	l, err := listenTCP(addr, srv.listenConf(), s)
	if err != nil {
		return nil, fmt.Errorf("failed to init TCP server: %w", err)
	}
	return &dns.Server{
		Addr:           addr,
		Net:            "tcp",
		Listener:       l,
		Handler:        h,
		DecorateReader: newMonitoredReader(l),
	}, nil
}

// initTLSServer loads TLS certificates and session keys, before opening a
// monitored TCP/TLS socket and returns a DNS server ready for ActivateAndServe.
// The context is used to teminate the session ticket refresh goroutine.
func (srv *Server) initTLSServer(ctx context.Context, addr string, h dns.Handler, conf *tlsconfig.TLSConfig, s *metrics.Stats) (*dns.Server, error) {
	tlsConf, err := tlsconfig.InitTLSConfig(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to init TLS config: %w", err)
	}
	// This listener is owned by [dns.Server], and will be closed by [dns.Server.Shutdown].
	l, err := listenTLS(addr, srv.listenConf(), tlsConf, s) //nolint:contextcheck
	if err != nil {
		return nil, fmt.Errorf("failed to init TCP/TLS server: %w", err)
	}
	return &dns.Server{
		Addr:           addr,
		Net:            "tcp-tls",
		Listener:       l,
		TLSConfig:      tlsConf,
		Handler:        h,
		DecorateReader: newMonitoredReader(l),
	}, nil
}

// listenConf returns the listener config used by individual servers to spawn
// listeners.
func (srv *Server) listenConf() net.ListenConfig {
	conf := net.ListenConfig{}
	if srv.conf.ReusePort > 0 {
		conf = net.ListenConfig{Control: reusePort}
	}
	return conf
}

// Start iterates through a list of configured IPs, and starts a separate
// goroutine to handle DNS requests on the protocols enabled in the
// configuration. Start does not block, and the servers may fail with fatal
// errors.
func (srv *Server) Start() (err error) {
	var (
		defaultHandler   plugin.Handler = srv.db
		maxAnswerHandler *maxAnswerHandler
		whoamiHandler    *whoami.Handler
		dotTLSAHandler   *dotTLSAHandler
		anyHandler       *anyHandler
		nsidHandler      *nsid.Handler
		throttleHandler  *throttle.Handler
		throttleLimiter  *throttle.Limiter
		numListeners     = srv.conf.ReusePort
	)

	// We have at least 1 listener
	if numListeners == 0 {
		numListeners = 1
	}

	// DNS connection stats
	stats := metrics.NewStats()
	err = srv.metricsExporter.ConsumeStats(connectionKey, stats)
	if err != nil {
		glog.Errorf("Failed to register metrics for consumption. %v, err: %v", stats, err)
	}

	idleTimeoutFunc := func() time.Duration {
		return srv.conf.TCPIdleTimeout
	}

	if srv.conf.TLSConfig.DoTTLSAEnabled {
		glog.Infof("Enabling DoTTLSAHandler")
		if !srv.conf.TLS {
			log.Fatalf("TLS is disabled yet are enabling DoTTLSAHandler. This is likely unexpected")
		}
		if dotTLSAHandler, err = newDotTLSA(&srv.conf.TLSConfig); err != nil {
			return fmt.Errorf("failed to initialize dotTLSAHandler: %w", err)
		}

		dotTLSAHandler.Next = defaultHandler
		defaultHandler = dotTLSAHandler
	}
	// Only add whoamiHandler to the plugin chain if it is enabled.
	if srv.conf.WhoamiDomain != "" {
		domain := strings.ToLower(dns.Fqdn(srv.conf.WhoamiDomain))
		glog.Infof("Enabling WhoAmI handler for domain %s with privateInfo=%v", domain, srv.conf.PrivateInfo)
		if whoamiHandler, err = whoami.NewWhoami(domain, srv.conf.PrivateInfo); err != nil {
			return fmt.Errorf("failed to initialize whoamiHandler: %w", err)
		}
		whoamiHandler.Next = defaultHandler
		defaultHandler = whoamiHandler
	} else {
		glog.Infof("-whoami-domain was not specified, not initializing whoamiHandler")
	}
	// Only add anyHandler to the plugin chain if it is enabled.
	if srv.conf.RefuseANY {
		glog.Infof("Enabling ANY handler")
		if anyHandler, err = newAnyHandler(); err != nil {
			return fmt.Errorf("failed to initialize anyHandler: %w", err)
		}
		anyHandler.Next = defaultHandler
		defaultHandler = anyHandler
	} else {
		glog.Infof("refuse-any flag was not specified, not initializing anyHandler")
	}

	// Only add nsidHandler to the plugin chain if it is enabled.
	if srv.conf.NSID {
		glog.Infof("Enabling NSID with privateInfo=%v", srv.conf.PrivateInfo)
		if nsidHandler, err = nsid.NewHandler(srv.conf.PrivateInfo); err != nil {
			return fmt.Errorf("failed to initialize NSID handler: %w", err)
		}
		nsidHandler.Next = defaultHandler
		defaultHandler = nsidHandler
	} else {
		glog.Info("-nsid was not specified, disabling NSID responses")
	}

	// Share one limiter across all IPs.
	if srv.conf.MaxConcurrency > 0 {
		maxWorkers := srv.conf.MaxConcurrency * srv.conf.NumCPU
		glog.Infof("Limiting total concurrency to %d", maxWorkers)
		if throttleLimiter, err = throttle.NewLimiter(maxWorkers); err != nil {
			return err
		}

		go throttle.Monitor(throttleLimiter, srv.stats, time.Second)
	}

	// For each configured IP, we may start a number of DNS servers for each
	// transport protocol.
	for ip, maxAns := range srv.conf.IPAns {
		if maxAnswerHandler, err = newMaxAnswerHandler(maxAns); err != nil {
			return fmt.Errorf("failed to initialize maxAnswerHandler: %w", err)
		}
		maxAnswerHandler.Next = defaultHandler
		glog.Infof("Creating handler with VIP %s and max answer %d", ip, maxAns)
		handler := &serveMux{
			defaultHandler: maxAnswerHandler,
		}

		if srv.conf.DNSSECConfig.Zones != "" && srv.conf.DNSSECConfig.Keys != "" {
			glog.Infof(
				"Enabling DNSSEC Handler for Zones: '%s', Keys: '%s'",
				srv.conf.DNSSECConfig.Zones,
				srv.conf.DNSSECConfig.Keys)
			dnssecHandler, err := newDNSSECHandler(srv, maxAnswerHandler)
			if err != nil {
				return fmt.Errorf("failed to initialize dnssecHandler: %w", err)
			}
			handler.defaultHandler = dnssecHandler
		} else {
			glog.Infof(
				"Not enabling DNSSEC Handler, either DNSSEC zones or keys are not specified. Zones: '%s', Keys: '%s'",
				srv.conf.DNSSECConfig.Zones,
				srv.conf.DNSSECConfig.Keys)
		}

		if srv.conf.MaxUDPSize >= dns.MinMsgSize && srv.conf.MaxUDPSize <= dns.MaxMsgSize {
			glog.Infof("Limiting UDP response size to %d", srv.conf.MaxUDPSize)
			handler.defaultHandler = bufsize.Bufsize{
				Size: srv.conf.MaxUDPSize,
				Next: handler.defaultHandler,
			}
		} else if srv.conf.MaxUDPSize > 0 {
			glog.Warningf("Ignoring non-compliant -max-udp-size: %d", srv.conf.MaxUDPSize)
		} else {
			glog.Infof("Max UDP size not set")
		}

		if throttleLimiter != nil {
			throttleHandler = throttle.NewHandler(throttleLimiter)
			throttleHandler.Next = handler.defaultHandler
			handler.defaultHandler = throttleHandler
		}

		addr := joinAddress(ip, srv.conf.Port)

		for i := 0; i < numListeners; i++ {
			// UDP is the default, and is always run.
			s, err := srv.initUDPServer(addr, handler)
			if err != nil {
				return err
			}
			s.ReadTimeout = srv.conf.ReadTimeout
			s.NotifyStartedFunc = srv.NotifyStartedFunc
			if throttleHandler != nil {
				throttleHandler.Attach(s)
			}
			srv.servers = append(srv.servers, s)
			// Server never calls Done() method, it only provides
			// this wg for client to use.
			srv.ServersStartedWG.Add(1)
			go func() {
				err := s.ActivateAndServe()
				if err != nil {
					glog.Infof("UDP server for %s failed to start: %v", addr, err)
				}
			}()

			// Optionally start a TCP server for the address as well.
			if srv.conf.TCP {
				s, err := srv.initTCPServer(addr, handler, stats)
				if err != nil {
					return err
				}
				s.MaxTCPQueries = srv.conf.MaxTCPQueries
				s.ReadTimeout = srv.conf.ReadTimeout
				s.NotifyStartedFunc = srv.NotifyStartedFunc
				s.IdleTimeout = idleTimeoutFunc
				if throttleHandler != nil {
					throttleHandler.Attach(s)
				}
				srv.servers = append(srv.servers, s)
				// Server never calls Done() method, it only provides
				// this wg for client to use.
				srv.ServersStartedWG.Add(1)
				go func() {
					err := s.ActivateAndServe()
					if err != nil {
						glog.Errorf("TCP server for %s failed to start: %v", addr, err)
					}
				}()
			}

			// Optionally start a TLS server for the address as well.
			if srv.conf.TLS {
				addr := joinAddress(ip, srv.conf.TLSConfig.Port)
				ctx, cancel := context.WithCancel(context.Background())
				s, err := srv.initTLSServer(ctx, addr, handler, &srv.conf.TLSConfig, stats)
				if err != nil {
					cancel()
					return err
				}
				s.MaxTCPQueries = srv.conf.MaxTCPQueries
				s.ReadTimeout = srv.conf.ReadTimeout
				s.NotifyStartedFunc = srv.NotifyStartedFunc
				s.IdleTimeout = idleTimeoutFunc
				if throttleHandler != nil {
					throttleHandler.Attach(s)
				}
				srv.servers = append(srv.servers, s)
				// Server never calls Done() method, it only provides
				// this wg for client to use.
				srv.ServersStartedWG.Add(1)
				go func() {
					err := s.ActivateAndServe()
					if err != nil {
						glog.Errorf("TCP-TLS server for %s failed to start: %v", addr, err)
					}
					cancel()
				}()
			}
		}
	}

	return nil
}

// Shutdown shuts down all the underlying servers and close the DB.
func (srv *Server) Shutdown() {
	glog.Infof("Shutting down %d servers", len(srv.servers))
	for _, s := range srv.servers {
		glog.Infof("Shutting down %s/%s", s.Addr, s.Net)
		err := s.Shutdown()
		if err != nil {
			glog.Errorf("%v", err)
		}
	}
	srv.db.Close()
}

// ReloadDB refreshes the data view
func (srv *Server) ReloadDB() {
	srv.db.ReloadChan <- *dnsserver.NewPartialReloadSignal()
}

// ValidateDbKey checks whether record of certain key is in db
func (srv *Server) ValidateDbKey(dbKey []byte) error {
	return srv.db.ValidateDbKey(dbKey)
}

// getDBTimestamp will lookup `dbTimestampKey` in the DB and log its value in
// `cdb.timestamp` counter.
func (srv *Server) getDBTimestamp() error {
	var (
		err            error
		rec            db.ResourceRecord
		timestamp      int64
		timestampFound bool
	)

	srv.stats.IncrementCounter(DBTimestampNumRun)
	parseResult := func(result []byte) error {
		if errors.Is(err, io.EOF) {
			return nil
		}

		if rec, err = db.ExtractRRFromRow(result, false); err != nil {
			// Not a location match
			// nolint: nilerr
			return nil
		}
		// We only care about the TXT field.
		if rec.Qtype == dns.TypeTXT {
			timestamp, err = strconv.ParseInt(string(result[rec.Offset+1:]), 10, 64)
			if err != nil {
				srv.stats.IncrementCounter(DBTimestampInvalidTXT)
				// nolint: nilerr
				return nil
			}
			timestampFound = true
		}
		return nil
	}

	reader, err := srv.db.AcquireReader()
	if err != nil {
		srv.stats.IncrementCounter(DBTimestampDBReadError)
		return err
	}
	err = reader.ForEachResourceRecord(dbTimestampName, db.ZeroID, parseResult)
	reader.Close()
	if err != nil {
		srv.stats.IncrementCounter(DBTimestampKeySearchError)
		return err
	}
	if !timestampFound {
		srv.stats.IncrementCounter(DBTimestampKeyNotFound)
		return fmt.Errorf("timestamp key not found")
	}
	srv.stats.ResetCounterTo(DBTimestamp, timestamp)

	freshness := time.Now().Unix() - timestamp
	srv.stats.ResetCounterTo(DBFreshness, freshness)

	return nil
}

// DumpBackendStats reports stats reported by DB backend as server counters
func (srv *Server) DumpBackendStats() {
	ticker := time.NewTicker(BackendStatsInterval * time.Second)
	for range ticker.C {
		srv.db.ReportBackendStats()
	}
}

// LogMapAge will log DB timestamp every `DBTimestampInterval`.
func (srv *Server) LogMapAge() {
	for _, k := range []string{
		DBTimestampDBReadError,
		DBTimestampInvalidTXT,
		DBTimestampKeySearchError,
		DBTimestampKeyNotFound,
		DBTimestampNumRun,
		DBTimestamp,
	} {
		srv.stats.ResetCounter(k)
	}
	if err := srv.getDBTimestamp(); err != nil {
		glog.Errorf("LogMapAge: %s", err)
	}
	ticker := time.NewTicker(DBTimestampInterval * time.Second)
	for range ticker.C {
		if err := srv.getDBTimestamp(); err != nil {
			glog.Errorf("LogMapAge: %s", err)
		}
	}
}

// WatchDBAndReload refreshes the data view on DB file change
func (srv *Server) WatchDBAndReload() {
	// If watcher fails - shutdown
	if err := srv.db.WatchDBAndReload(); err != nil {
		glog.Errorf("Error watching db: %s", err)
		srv.Shutdown()
	}
}

// WatchControlDirAndReload refreshes the data view on control file signals change
func (srv *Server) WatchControlDirAndReload() {
	// If watcher fails - shutdown
	if err := srv.db.WatchControlDirAndReload(); err != nil {
		glog.Errorf("Error watching control dir: %s", err)
		srv.Shutdown()
	}
}

// PeriodicDBReload reloads db map periodically
func (srv *Server) PeriodicDBReload(reloadInt int) {
	srv.db.PeriodicDBReload(reloadInt)
}

// failOnErr fatally logs errors if the error is not nil.
func failOnErr(err error, msg string) {
	if err != nil {
		glog.Fatalf("%s: %v\n", msg, err)
	}
}

// joinAddress joins a string ip address and an integer port.
func joinAddress(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// reusePort sets a UNIX socket option that allows the listener to bind to a
// port that is already in use. The delegation of traffic to listeners is
// equally distributed via this method.
func reusePort(_, _ string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}
