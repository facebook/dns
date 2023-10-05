package query

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/facebook/dns/goose/stats"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"
)

// QTypeStrToDNSQtype transforms qtypestr (eg: "A") to miekg/dns qtype eg: dns.TypeA
func QTypeStrToDNSQtype(qTypeStr string) (dns.Type, error) {
	switch strings.ToLower(qTypeStr) {
	case "a":
		return dns.Type(dns.TypeA), nil
	case "aaaa":
		return dns.Type(dns.TypeAAAA), nil
	case "cname":
		return dns.Type(dns.TypeCNAME), nil
	case "ns":
		return dns.Type(dns.TypeNS), nil
	case "txt":
		return dns.Type(dns.TypeTXT), nil
	case "srv":
		return dns.Type(dns.TypeSRV), nil
	case "ptr":
		return dns.Type(dns.TypePTR), nil
	case "mx":
		return dns.Type(dns.TypeMX), nil
	case "soa":
		return dns.Type(dns.TypeSOA), nil
	case "caa":
		return dns.Type(dns.TypeCAA), nil
	default:
		return dns.Type(dns.TypeNone), fmt.Errorf("invalid query type: %s", qTypeStr)
	}
}

// ProcessQueryInputFile parses a dnsperf compatible query input file for qnames/qtypes
func ProcessQueryInputFile(path string) ([]string, []dns.Type, error) {
	qnames := make([]string, 0)
	qtypes := make([]dns.Type, 0)
	file, err := os.Open(path)
	if err != nil {
		return qnames, qtypes, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		qnames = append(qnames, fields[0])
		qtype, err := QTypeStrToDNSQtype(fields[1])
		if err != nil {
			return qnames, qtypes, err
		}
		qtypes = append(qtypes, qtype)
	}
	return qnames, qtypes, scanner.Err()
}

// MakeReq creates a DNS request
func MakeReq(domain string, now func() time.Time, randomise bool, qtype dns.Type) *dns.Msg {
	record := domain
	if randomise {
		prefix := "goose"
		r := rand.Intn(1000000000)
		record = fmt.Sprintf("%s.%d.%d.%s.", prefix, now().Unix(), r, domain)
	}
	log.Debugf("Request: %v", record)
	msg := dns.Msg{}
	msg.SetQuestion(dns.Fqdn(record), uint16(qtype))
	return &msg
}

// MonitorTarget checks that target is responding
func MonitorTarget(sigPause chan struct{}, monPort int, monHost string) {
	go func() {
		for {
			err := monitorPort(monPort, monHost)
			if err != nil {
				log.Errorf("%v", err)
				sigPause <- struct{}{}
			} else {
				time.Sleep(time.Second)
			}
		}
	}()
}

func monitorPort(port int, host string) error {
	timeout := 500 * time.Millisecond
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		var opError *net.OpError
		if ok := errors.As(err, &opError); ok && opError.Timeout() {
			return fmt.Errorf("connection to Monitor Port timed out: %w", opError)
		}
		return fmt.Errorf("connection to Monitor Port failed with error: %w", err)
	}
	defer conn.Close()
	return nil
}

// SendMsg takes a pointer to a DNS request and returns a pointer to a DNS reply
type SendMsg func(*dns.Msg) (*dns.Msg, error)

// ClientConnection creates a persistent connection for an already existing dns client
func ClientConnection(client *dns.Client, dport int, host string) (*dns.Conn, error) {
	return client.Dial(net.JoinHostPort(host, strconv.Itoa(dport)))
}

// DNSClient creates a new DNS Cloent
func DNSClient(timeout time.Duration) *dns.Client {
	client := new(dns.Client)
	client.Timeout = timeout
	return client
}

// DNS sends a DNS request and checks the response from the DNS server
func DNS(client *dns.Client, check ValidateResponse, timeout time.Duration, conn *dns.Conn) SendMsg {
	if client == nil {
		client = DNSClient(timeout)
	}
	client.Timeout = timeout

	return func(request *dns.Msg) (*dns.Msg, error) {
		resp, _, err := client.ExchangeWithConn(request, conn)
		if err != nil {
			log.Debugf("Connection failure: %v", err)
			return nil, err
		}
		if err = check(resp); err != nil {
			log.Debugf("Response received not valid: %v", err)
			return nil, err
		}
		return resp, nil
	}
}

// RunQuery wraps the function that sends the DNS query and passes metrics to a channel
func RunQuery(reqMsg *dns.Msg, requestFunc SendMsg, now func() time.Time, state *RunState) {
	reqStart := now()
	log.Debugf("%v:: Request: %v", reqStart.Nanosecond(), reqMsg.Question[0].Name)
	_, err := requestFunc(reqMsg)
	if err != nil {
		state.incErrors()
	} else {
		state.incProcessed()
	}
	reqEnd := now()
	latency := reqEnd.Sub(reqStart)
	state.addLatency(float64(latency))
	log.Debugf("%v:: Response Latency: %v", reqEnd.Nanosecond(), latency)
}

// RunState object holds the state of the performance test
type RunState struct {
	// rate limits the queries per second.
	limiter ratelimit.Limiter
	// queriesToSend is the number of queries left to send.
	queriesToSend int
	// startTime is the time when the test has been started.
	startTime time.Time
	// processed is the number of queries successfully processed.
	processed int
	// errors is the number of queries that failed.
	errors int
	// unexportedLatencies contain per query latency which havent been exported yet.
	unexportedLatencies []float64
	// lastExportedAt is the last time we printed the intermediate state.
	lastExportedAt        time.Time
	lastExportedProcessed int
	lastExportedErrors    int
	// alreadyExportedLatencies contain per query latency which have already been exported by `ExportIntermediateResults`, these still need to be accounted at the final export
	alreadyExportedLatencies []float64

	// are we running in daemon mode
	daemon bool
	// m protects all fields.
	m sync.Mutex

	// nowfunc is used by unittests to manipulate Now(). normally it's time.Now
	nowfunc func() time.Time
}

// NewRunState creates a new RunState instance
func NewRunState(queriesToSend int, limiter ratelimit.Limiter, daemon bool, nowfunc func() time.Time) *RunState {
	return &RunState{
		limiter:                  limiter,
		queriesToSend:            queriesToSend,
		startTime:                nowfunc(),
		processed:                0,
		errors:                   0,
		unexportedLatencies:      make([]float64, 0),
		lastExportedAt:           time.Time{},
		lastExportedProcessed:    0,
		lastExportedErrors:       0,
		alreadyExportedLatencies: make([]float64, 0),
		daemon:                   daemon,
		m:                        sync.Mutex{},
		nowfunc:                  nowfunc,
	}
}

// ExportIntermediateResults is used to export intermediate results while the test is still in progress
func (r *RunState) ExportIntermediateResults() *stats.ExportedMetrics {
	r.m.Lock()
	defer r.m.Unlock()
	processed := r.processed - r.lastExportedProcessed
	failed := r.errors - r.lastExportedErrors

	startTime := r.lastExportedAt
	if r.lastExportedAt.IsZero() {
		startTime = r.startTime
	}

	elapsed := r.nowfunc().Sub(startTime)
	latencies := r.unexportedLatencies
	if !r.daemon { // in daemon mode we don't care about the final export's correctness, but this slice would grow indefinitely, so let's not save already exported latencies
		r.alreadyExportedLatencies = append(r.alreadyExportedLatencies, r.unexportedLatencies...)
	}
	r.unexportedLatencies = make([]float64, 0)

	r.lastExportedAt = r.nowfunc()
	r.lastExportedProcessed = r.processed
	r.lastExportedErrors = r.errors
	return &stats.ExportedMetrics{
		Elapsed:   elapsed,
		Processed: processed,
		Errors:    failed,
		Latencies: latencies,
	}
}

// ExportResults returns the final results of the test
func (r *RunState) ExportResults() *stats.ExportedMetrics {
	r.m.Lock()
	defer r.m.Unlock()
	return &stats.ExportedMetrics{
		Elapsed:   r.nowfunc().Sub(r.startTime),
		Processed: r.processed,
		Errors:    r.errors,
		Latencies: append(r.alreadyExportedLatencies, r.unexportedLatencies...),
	}
}

// decQueriesToSend decrements queriesToSend number, returns the new value.
func (r *RunState) decQueriesToSend() (q int) {
	r.m.Lock()
	defer r.m.Unlock()
	r.queriesToSend--
	return r.queriesToSend
}

// incProcessed increments processed number
func (r *RunState) incProcessed() {
	r.m.Lock()
	defer r.m.Unlock()
	r.processed++
}

// incErrors increments errors number
func (r *RunState) incErrors() {
	r.m.Lock()
	defer r.m.Unlock()
	r.errors++
}

// addLatency records a query execution time
func (r *RunState) addLatency(latency float64) {
	r.m.Lock()
	defer r.m.Unlock()
	r.unexportedLatencies = append(r.unexportedLatencies, latency)
}

func (r *RunState) getProcessedQueries() int {
	r.m.Lock()
	defer r.m.Unlock()
	return r.processed + r.errors
}

// RunQueries starts loading the target host with DNS queries
func RunQueries(dport int, host string, domains []string, timeout time.Duration, randomiseQueries bool, qTypes []dns.Type, now func() time.Time, runState *RunState, sigpause chan struct{}) error {
	client := DNSClient(timeout)
	conn, err := ClientConnection(client, dport, host)
	if err != nil {
		return err
	}
	request := DNS(client, CheckResponse, timeout, conn)
	queriesToSend := runState.decQueriesToSend()
	for queriesToSend >= 0 || runState.daemon {
		select {
		case <-sigpause:
			log.Warningf("Pausing for 5 seconds as Monitor Host/Port not responding")
			time.Sleep(time.Second * 5)
		default:
			idx := runState.getProcessedQueries() % len(domains)
			reqMsg := MakeReq(domains[idx], time.Now, randomiseQueries, qTypes[idx])
			runState.limiter.Take()
			RunQuery(reqMsg, request, now, runState)
			queriesToSend = runState.decQueriesToSend()
		}
	}
	return nil

}
