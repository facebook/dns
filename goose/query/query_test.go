package query

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/facebook/dns/goose/stats"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"go.uber.org/ratelimit"
)

func Test_MakeReq(t *testing.T) {
	//  return a time.Time that is used to generate the name being requested
	timeFn := func() time.Time {
		return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	// Create a DNS requests
	req := MakeReq("somedomain.com", timeFn, true, dns.Type(dns.TypeA))

	require.Regexp(
		t,
		regexp.MustCompile(`goose.1577836800.\d+.somedomain.com.`),
		req.Question[0].Name,
		"FQDN requested does not match what is expected",
	)
}

func timefunc() func() time.Time {
	i := 0
	return func() time.Time {
		i += 10000000 // increment by 10ms
		return time.Date(2020, time.November, 10, 23, 0, 0, i, time.UTC)
	}
}

func doSendMsgSuccess(*dns.Msg) (*dns.Msg, error) {
	return msgSingleARecord(), nil
}

func doSendMsgFailure(*dns.Msg) (*dns.Msg, error) {
	return new(dns.Msg), errors.New("I am an error")
}

func doSendMsgNil(*dns.Msg) (*dns.Msg, error) {
	var resp *dns.Msg
	return resp, errors.New("I am an error")
}

func Test_RunQuery(t *testing.T) {
	reqMsg := new(dns.Msg)
	reqMsg.SetQuestion(dns.Fqdn("goose.1234567.somedomain.com."), dns.TypeANY)
	tf := timefunc()
	runState := NewRunState(1, ratelimit.NewUnlimited(), false, tf)

	RunQuery(reqMsg, doSendMsgSuccess, tf, runState)

	expected := &stats.ExportedMetrics{Elapsed: 30000000, Processed: 1, Errors: 0, Latencies: []float64{10000000}}
	exportedMetrics := runState.ExportResults()
	require.Equal(t, expected, exportedMetrics)

	RunQuery(reqMsg, doSendMsgFailure, tf, runState)

	expected.Errors = 1
	expected.Latencies = append(expected.Latencies, 10000000)
	expected.Elapsed = 60000000

	exportedMetrics = runState.ExportResults()
	require.Equal(t, expected, exportedMetrics)

	RunQuery(reqMsg, doSendMsgNil, tf, runState)

	expected.Errors = 2
	expected.Latencies = append(expected.Latencies, 10000000)
	expected.Elapsed = 90000000
	exportedMetrics = runState.ExportResults()
	require.Equal(t, expected, exportedMetrics)
}

func Test_exportIntermediateMetrics(t *testing.T) {
	reqMsg := new(dns.Msg)
	reqMsg.SetQuestion(dns.Fqdn("goose.1234567.somedomain.com."), dns.TypeANY)
	tf := timefunc()
	runState := NewRunState(1, ratelimit.NewUnlimited(), false, tf)

	RunQuery(reqMsg, doSendMsgSuccess, tf, runState)

	expected := &stats.ExportedMetrics{Elapsed: 30000000, Processed: 1, Errors: 0, Latencies: []float64{10000000}}
	exportedMetrics := runState.ExportIntermediateResults()
	require.Equal(t, expected, exportedMetrics)

	RunQuery(reqMsg, doSendMsgFailure, tf, runState)

	expected.Errors = 1
	expected.Processed = 0

	exportedMetrics = runState.ExportIntermediateResults()
	require.Equal(t, expected, exportedMetrics)

	RunQuery(reqMsg, doSendMsgFailure, tf, runState)

	expected.Errors = 2
	expected.Processed = 1
	expected.Latencies = append(expected.Latencies, 10000000, 10000000)
	expected.Elapsed = 110000000
	exportedMetrics = runState.ExportResults()
	require.Equal(t, expected, exportedMetrics)
}

func Test_StateAddSuccess(t *testing.T) {
	runState := &RunState{nowfunc: timefunc()}

	runState.incProcessed()
	require.Equal(t, 1, runState.ExportResults().Processed)
}

func Test_StateAddError(t *testing.T) {
	runState := &RunState{nowfunc: timefunc()}

	runState.incErrors()
	require.Equal(t, 1, runState.ExportResults().Errors)
}

func Test_StateAddLatency(t *testing.T) {
	runState := &RunState{nowfunc: timefunc()}

	runState.addLatency(2)
	require.Equal(t, []float64{2}, runState.ExportResults().Latencies)
}
