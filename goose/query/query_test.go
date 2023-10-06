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

package query

import (
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/facebook/dns/goose/stats"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"go.uber.org/ratelimit"
)

func Test_QTypeStrToDNSQtype(t *testing.T) {
	qType, err := QTypeStrToDNSQtype("A")
	require.Equal(t, dns.Type(dns.TypeA), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("AAAA")
	require.Equal(t, dns.Type(dns.TypeAAAA), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("CNAME")
	require.Equal(t, dns.Type(dns.TypeCNAME), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("NS")
	require.Equal(t, dns.Type(dns.TypeNS), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("TXT")
	require.Equal(t, dns.Type(dns.TypeTXT), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("SRV")
	require.Equal(t, dns.Type(dns.TypeSRV), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("PTR")
	require.Equal(t, dns.Type(dns.TypePTR), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("MX")
	require.Equal(t, dns.Type(dns.TypeMX), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("SOA")
	require.Equal(t, dns.Type(dns.TypeSOA), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("CAA")
	require.Equal(t, dns.Type(dns.TypeCAA), qType)
	require.NoError(t, err)
	qType, err = QTypeStrToDNSQtype("derp")
	require.Equal(t, dns.Type(dns.TypeNone), qType)
	require.Error(t, err)
}

func Test_ProcessQueryInputFile(t *testing.T) {
	tf, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer os.Remove(tf.Name())
	rows := []byte("facebook.com\tAAAA\ntest.com\tCNAME")
	err = os.WriteFile(tf.Name(), rows, 0644)
	require.NoError(t, err)
	qnames, qtypes, err := ProcessQueryInputFile(tf.Name())
	require.Equal(t, []string{"facebook.com", "test.com"}, qnames)
	require.Equal(t, []dns.Type{dns.Type(dns.TypeAAAA), dns.Type(dns.TypeCNAME)}, qtypes)
	require.NoError(t, err)
	rows = []byte("facebook.com\tDERP")
	err = tf.Truncate(0)
	require.NoError(t, err)
	err = os.WriteFile(tf.Name(), rows, 0644)
	require.NoError(t, err)
	err = tf.Sync()
	require.NoError(t, err)
	qnames, qtypes, err = ProcessQueryInputFile(tf.Name())
	require.Equal(t, []string{"facebook.com"}, qnames)
	require.Equal(t, []dns.Type{}, qtypes)
	require.Error(t, err)
}
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
