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

package fbserver

import (
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/dnsserver"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
	"github.com/facebook/dns/dnsrocks/metrics"
	"github.com/facebook/dns/dnsrocks/testaid"
)

func TestMain(m *testing.M) {
	os.Exit(testaid.Run(m, "../testdata/data"))
}

func makeTestServerConfig(tcp, tls bool) ServerConfig {
	s := NewServerConfig()
	s.IPAns["::1"] = 1
	s.Port = 0
	s.TCP = tcp
	if tls {
		s.TLS = tls
		// FIXME: Possibly handle CryptoSSL
		// Certs/Priv keys can be created using `testaid.mkTestCert`
	}
	db := testaid.TestCDB
	s.DBConfig.Driver = db.Driver
	s.DBConfig.Path = db.Path
	s.DBConfig.ReloadInterval = 100
	s.NumCPU = runtime.NumCPU()
	return s
}

// makeTestServer spins up standalone DNS servers based on the ServerConfig `config`.
// returns a map of `network`/listening address.
// network can be any of udp, tcp, tcp-tls
func makeTestServer(t testing.TB, config ServerConfig) (map[string]string, *Server) {
	var m = make(map[string]string)
	logger := dnsserver.DummyLogger{}
	stats := stats.DummyStats{}
	metricsExporter, _ := metrics.NewMetricsServer(":0")
	srv := NewServer(config, &logger, &stats, metricsExporter)
	numServers := int64(1) // UDP is always enabled
	if config.TCP {
		numServers++
	}
	if config.TLS {
		numServers++
	}
	serverUpChan := make(chan string)
	onUp := func() {
		serverUpChan <- "up"
	}

	srv.NotifyStartedFunc = onUp
	require.Nil(t, srv.Start(), "Failed to start servers!")

	for i := numServers; i > 0; i-- {
		<-serverUpChan
	}
	close(serverUpChan)

	for _, s := range srv.servers {
		if s.Listener != nil {
			m[s.Net] = s.Listener.Addr().String()
		} else {
			m[s.Net] = s.PacketConn.LocalAddr().String()
		}
	}
	return m, srv
}

// RunUDPTestServer spins up a standalone UDP DNS server.
// returns a map of `network`/listening address.
// network should only be `udp` in this case.
func RunUDPTestServer(t *testing.T) (map[string]string, *Server) {
	config := makeTestServerConfig(false, false)
	return makeTestServer(t, config)
}

// RunUDPTCPTestServer spins up a standalone UDP DNS server.
// returns a map of `network`/listening address.
// network should only be `udp` or `tcp` in this case.
func RunUDPTCPTestServer(t *testing.T) (map[string]string, *Server) {
	config := makeTestServerConfig(true, false)
	return makeTestServer(t, config)
}

// RunUDPTLSTestServer spins up a standalone UDP DNS server.
// returns a map of `network`/listening address.
// network should only be `udp` or `tcp` in this case.
func RunUDPTLSTestServer(t *testing.T) (map[string]string, *Server) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	config := makeTestServerConfig(false, true)
	config.TLSConfig.CertFile = certfile
	config.TLSConfig.KeyFile = certfile
	return makeTestServer(t, config)
}

// TestRunUDPTestServer simple test to ensure that we only create a UDP server.
func TestRunUDPTestServer(t *testing.T) {
	portMap, srv := RunUDPTestServer(t)
	defer srv.Shutdown()
	require.Len(t, portMap, 1)
	require.Contains(t, portMap, "udp")
}

// TestRunUDPTCPTestServer simple test to ensure that we create a TCP server.
// Currently, there is always a UDP server which is started.
func TestRunUDPTCPTestServer(t *testing.T) {
	portMap, srv := RunUDPTCPTestServer(t)
	defer srv.Shutdown()
	require.Len(t, portMap, 2)
	require.Contains(t, portMap, "udp")
	require.Contains(t, portMap, "tcp")
}

// TestRunUDPTLSTestServer simple test to ensure that we create a TCP server.
// Currently, there is always a UDP server which is started.
func TestRunUDPTLSTestServer(t *testing.T) {
	portMap, srv := RunUDPTLSTestServer(t)
	defer srv.Shutdown()
	require.Len(t, portMap, 2)
	require.Contains(t, portMap, "udp")
	require.Contains(t, portMap, "tcp-tls")
}

// TestUDPDNSServerWithQueries creates a standalone UDP server and test a
// couple of DNS queries against it.
func TestUDPDNSServerWithQueries(t *testing.T) {
	portMap, srv := RunUDPTestServer(t)
	defer srv.Shutdown()

	testCases := []struct {
		qname  string
		target string
	}{
		{
			qname:  "foo2.example.com.",
			target: "some-other.domain.",
		},
		{
			qname:  "cnamemap.example.net.",
			target: "bar.example.net.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.qname, func(t *testing.T) {
			c := new(dns.Client)
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, dns.TypeA)
			r, _, err := c.Exchange(m, portMap["udp"])
			require.Nil(t, err)
			require.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			require.Equal(t, tc.target, cname)
		})
	}
}

// TestUDPDNSServerQueryMultipleQuestions creates a standalone UDP server and
// test a query that contains multiple questions.
// Since https://github.com/miekg/dns/commit/2c18e7259a35458cf282adbfa12b04de0d00c899
// any request with anything different than 1 question, anything in Answer,
// Authority or Additional section will result in an invalid DNS request.
func TestUDPDNSServerQueryMultipleQuestions(t *testing.T) {
	portMap, srv := RunUDPTestServer(t)
	defer srv.Shutdown()

	qname := "foo.example.com."
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(qname, dns.TypeA)
	m.Question = append(
		m.Question,
		dns.Question{Name: qname, Qtype: dns.TypeAAAA, Qclass: dns.ClassINET},
	)
	r, _, err := c.Exchange(m, portMap["udp"])
	require.Nil(t, err)

	// We expect a FORMERR and nothing in question or answer section (and anything
	// else for that matter).
	require.Equal(t, r.Rcode, dns.RcodeFormatError)
	require.Equal(t, 0, len(r.Question))
	require.Equal(t, 0, len(r.Answer))
}

// TestTCPDNSServerWithQueries creates a standalone TCP server and test a
// couple of DNS queries against it.
func TestTCPDNSServerWithQueries(t *testing.T) {
	portMap, srv := RunUDPTCPTestServer(t)
	defer srv.Shutdown()

	testCases := []struct {
		qname  string
		target string
	}{
		{
			qname:  "foo2.example.com.",
			target: "some-other.domain.",
		},
		{
			qname:  "cnamemap.example.net.",
			target: "bar.example.net.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.qname, func(t *testing.T) {
			c := new(dns.Client)
			c.Net = "tcp"
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, dns.TypeA)
			r, _, err := c.Exchange(m, portMap["tcp"])
			require.Nil(t, err)
			require.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			require.Equal(t, tc.target, cname)
		})
	}
}

// TestTLSDNSServerWithQueries creates a standalone TCP server and test a
// couple of DNS queries against it.
func TestTLSDNSServerWithQueries(t *testing.T) {
	portMap, srv := RunUDPTLSTestServer(t)
	defer srv.Shutdown()

	testCases := []struct {
		qname  string
		target string
	}{
		{
			qname:  "foo2.example.com.",
			target: "some-other.domain.",
		},
		{
			qname:  "cnamemap.example.net.",
			target: "bar.example.net.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.qname, func(t *testing.T) {
			c := new(dns.Client)
			c.Net = "tcp-tls"
			c.TLSConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
			m := new(dns.Msg)
			m.SetQuestion(tc.qname, dns.TypeA)
			r, _, err := c.Exchange(m, portMap["tcp-tls"])
			require.Nil(t, err)
			require.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			require.Equal(t, tc.target, cname)
		})
	}
}

// exchange sends and reads a message over an existing connection.
// This is heavily copied from github.com/miekg/dns/client.go code at
// https://fburl.com/puemzrx0
// This allows us to possibly write multiple requests over the same connection.
func exchange(t *testing.T, co *dns.Conn, c *dns.Client, m *dns.Msg) (r *dns.Msg) {
	opt := m.IsEdns0()
	// If EDNS0 is used use that for size.
	if opt != nil && opt.UDPSize() >= dns.MinMsgSize {
		co.UDPSize = opt.UDPSize()
	}
	// Otherwise use the client's configured UDP size.
	if opt == nil && c.UDPSize >= dns.MinMsgSize {
		co.UDPSize = c.UDPSize
	}

	// write with the appropriate write timeout
	err := co.SetWriteDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err, "Error setting write deadline")
	err = co.WriteMsg(m)
	require.Nil(t, err, "Error writing message")

	err = co.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err, "Error setting Read Deadline")
	r, err = co.ReadMsg()
	require.Nil(t, err, "Error reading message")
	require.Equal(t, m.Id, r.Id, "Response message does not have the same ID")

	return r
}

// TestTCPTimeout test that a TCP connection will timeout when idle for more
// than TCPidleTimeout. We need to send a first request because the first one
// is using readTimeout and only from the second query and on we start using
// idleTimeout.
// https://fburl.com/p61zy6nn
func TestTCPTimeout(t *testing.T) {
	config := makeTestServerConfig(true, false)
	config.TCPIdleTimeout = 2 * time.Second
	portMap, srv := makeTestServer(t, config)
	defer srv.Shutdown()

	// Create client
	c := new(dns.Client)
	c.Net = "tcp"

	// Create DNS msg
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	// Create connection
	var co *dns.Conn
	co, err := c.Dial(portMap["tcp"])
	require.Nil(t, err, "Error connecting to server")
	defer co.Close()

	// First request, which is using ReadTimeout
	r := exchange(t, co, c, m)
	require.Equal(t, r.Question[0].Name, "example.com.", "Mismatching question")
	time.Sleep(config.TCPIdleTimeout + 1)

	// Now we are going to read something and expect to get EOF before the
	// timeout occurs.
	one := make([]byte, 1)
	err = co.SetReadDeadline(time.Now().Add(1 * time.Second))
	require.NoError(t, err, "Error setting Read Deadline")

	_, err = co.Read(one)
	require.Equal(t, io.EOF, err, "Connection was expected to be closed by server")
}

// TestMultipleQueryoverTCP confirm that we can send multiple queries over the
// same TCP connection.
func TestMultipleQueryoverTCP(t *testing.T) {
	config := makeTestServerConfig(true, false)
	config.TCPIdleTimeout = 2 * time.Second
	portMap, srv := makeTestServer(t, config)
	defer srv.Shutdown()

	// messages
	msgs := []*dns.Msg{
		new(dns.Msg).SetQuestion("example.com.", dns.TypeA),
		new(dns.Msg).SetQuestion("example.net.", dns.TypeAAAA),
	}
	// Create client
	c := new(dns.Client)
	c.Net = "tcp"

	// Create connection
	var co *dns.Conn
	co, err := c.Dial(portMap["tcp"])
	require.Nil(t, err, "Error connecting to server")
	defer co.Close()

	for _, m := range msgs {
		r := exchange(t, co, c, m)
		require.Equal(t, m.Question[0].Name, r.Question[0].Name, "Mismatching question name")
		require.Equal(t, m.Question[0].Qtype, r.Question[0].Qtype, "Mismatching question type")
	}
}

func TestMaxUDPSize(t *testing.T) {
	testCases := []struct {
		clientMax uint16
		serverMax int
		truncated bool
	}{
		// No EDNS0 (clientMax is unset)
		{
			truncated: true,
		},
		{
			serverMax: 2048,
			truncated: true,
		},
		// One or both is too small.
		{
			clientMax: 512,
			// serverMax is unset
			truncated: true,
		},
		{
			clientMax: 512,
			serverMax: 512,
			truncated: true,
		},
		{
			clientMax: 2048,
			serverMax: 512,
			truncated: true,
		},
		{
			clientMax: 512,
			serverMax: 2048,
			truncated: true,
		},
		// Both are big enough.
		{
			clientMax: 2048,
			serverMax: 2048,
			truncated: false,
		},
		{
			clientMax: 2048,
			serverMax: 8192,
			truncated: false,
		},
		// Invalid server max sizes
		{
			clientMax: 2048,
			serverMax: 511, // Too small, ignored.
			truncated: false,
		},
		{
			clientMax: 2048,
			serverMax: 65536, // Too large, ignored.
			truncated: false,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d/%d", tc.clientMax, tc.serverMax), func(t *testing.T) {
			config := makeTestServerConfig(true, false)
			config.MaxUDPSize = tc.serverMax
			portMap, srv := makeTestServer(t, config)
			defer srv.Shutdown()

			c := new(dns.Client)
			query := new(dns.Msg)
			qname := "lotofns.example.org."
			query.SetQuestion(qname, dns.TypeNS)
			if tc.clientMax > 0 {
				query.SetEdns0(tc.clientMax, false)
			}
			udpResponse, _, err := c.Exchange(query, portMap["udp"])
			require.Nil(t, err)

			if tc.clientMax > 0 {
				require.NotNil(t, udpResponse.IsEdns0())
			} else {
				require.Nil(t, udpResponse.IsEdns0())
			}

			require.Equal(t, tc.truncated, udpResponse.Truncated)

			c.Net = "tcp"
			tcpResponse, _, err := c.Exchange(query, portMap["tcp"])
			require.Nil(t, err)
			require.NotNil(t, tcpResponse)
			require.False(t, tcpResponse.Truncated)
		})
	}
}

// Regression test for throttle jamming bug.
func TestThrottleJamming(t *testing.T) {
	// Setup
	qname := "example.com."
	validQuery := (&dns.Msg{}).SetQuestion(qname, dns.TypeA)
	acceptBuf, err := validQuery.Pack()
	require.Nil(t, err)

	badOpcode := validQuery.Copy()
	badOpcode.MsgHdr.Opcode = dns.OpcodeStatus
	notImplementedBuf, err := badOpcode.Pack()
	require.Nil(t, err)

	response := validQuery.Copy()
	response.MsgHdr.Response = true
	ignoredBuf, err := response.Pack()
	require.Nil(t, err)

	noQuestions := validQuery.Copy()
	noQuestions.Question = nil
	rejectBuf, err := noQuestions.Pack()
	require.Nil(t, err)

	invalidBuf := make([]byte, 1)

	N := 10
	config := makeTestServerConfig(true, false)
	config.MaxConcurrency = N
	portMap, srv := makeTestServer(t, config)
	defer srv.Shutdown()

	// Try to jam the throttle by sending weird (and regular) packets.
	for i := 0; i < N*config.NumCPU; i++ {
		for _, buf := range [][]byte{acceptBuf, notImplementedBuf, ignoredBuf, rejectBuf, invalidBuf} {
			conn, err := net.Dial("tcp", portMap["tcp"])
			require.Nil(t, err)

			tcpLen := []byte{0, uint8(len(buf))}
			_, err = conn.Write(tcpLen)
			require.Nil(t, err)
			_, err = conn.Write(buf)
			require.Nil(t, err)

			conn.Close()
		}
	}

	// Check that the server is still working (not jammed).
	c := new(dns.Client)
	c.Net = "tcp"
	m := new(dns.Msg)
	m.SetQuestion("foo2.example.com.", dns.TypeA)
	r, _, err := c.Exchange(m, portMap["tcp"])
	require.Nil(t, err)
	require.NotEqual(t, 0, len(r.Answer))

	cname := r.Answer[0].(*dns.CNAME).Target
	require.Equal(t, "some-other.domain.", cname)
}

func sprayUDP(conn *net.UDPConn, limit int, qname string, qtype uint16) {
	msg := (&dns.Msg{}).SetQuestion(qname, qtype)
	buf, _ := msg.Pack()
	for i := range limit {
		// Cache busting for the QNAME, which falls on a wildcard.
		label := fmt.Sprintf("%010d", i)
		copy(buf[13:], label)
		if _, err := conn.Write(buf); err != nil {
			return
		}
	}
}

// Confirm that spraying UDP packets to the server is not the limiting
// step in BenchmarkUDP.
func BenchmarkSpray(b *testing.B) {
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})
	require.Nil(b, err)
	defer l.Close()

	host, port, err := net.SplitHostPort(l.LocalAddr().String())
	require.Nil(b, err)

	portNum, err := strconv.Atoi(port)
	require.Nil(b, err)

	addr := net.UDPAddr{
		IP:   net.ParseIP(host),
		Port: portNum, // UDP equivalent of /dev/null
	}
	conn, err := net.DialUDP("udp", nil, &addr)
	require.Nil(b, err)
	defer conn.Close()

	b.ResetTimer()
	sprayUDP(conn, b.N, "0123456789.example.com.", dns.TypeA)
	b.StopTimer()

	seconds := b.Elapsed().Seconds()
	b.ReportMetric(float64(b.N)/seconds, "queries/sec")
	// This metric is less meaningful than QPS, so we can hide it.
	b.ReportMetric(0, "ns/op")
}

// Measure the goodput of the server over UDP in answers/sec.
// Queries have randomized QNAME and ECS subnet to avoid caching.
func BenchmarkUDP(b *testing.B) {
	testCases := []struct {
		name           string
		qtype          uint16
		qname          string
		cnameChasing   bool
		maxConcurrency []int
	}{
		// A query with cname chasing disabled
		{
			name:           "TypeA",
			qtype:          dns.TypeA,
			qname:          "0123456789.example.com.",
			cnameChasing:   false,
			maxConcurrency: []int{-1, 1, 10, 100, 1000}},
		// A query with one hop
		{
			name:           "TypeA-OneHop",
			qtype:          dns.TypeA,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{-1, 1, 10, 100, 1000},
		},
		// A query with two hops
		{
			name:           "TypeA-TwoHops",
			qtype:          dns.TypeA,
			qname:          "0123456789.twohops.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{-1, 1, 10, 100, 1000},
		},
		// AAAA query with cname chasing disabled
		{
			name:           "TypeAAAA",
			qtype:          dns.TypeAAAA,
			qname:          "0123456789.example.com.",
			cnameChasing:   false,
			maxConcurrency: []int{-1, 1, 10, 100, 1000}},
		// AAAA query with one hop
		{
			name:           "TypeAAAA-OneHop",
			qtype:          dns.TypeAAAA,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{-1, 1, 10, 100, 1000},
		},
		// AAAA query with two hops
		{
			name:           "TypeAAAA-TwoHops",
			qtype:          dns.TypeAAAA,
			qname:          "0123456789.twohops.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{-1, 1, 10, 100, 1000},
		},
		// CNAME query with cname chasing disabled
		{
			name:           "TypeCNAME",
			qtype:          dns.TypeCNAME,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   false,
			maxConcurrency: []int{1},
		},
		// CNAME query with cname chasing enabled
		{
			name:           "TypeCNAME-CNAMEChasing",
			qtype:          dns.TypeCNAME,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{1},
		},
		// ANY query with cname chasing disabled
		{
			name:           "TypeANY",
			qtype:          dns.TypeANY,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   false,
			maxConcurrency: []int{1},
		},
		// ANY query with cname chasing enabled
		{
			name:           "TypeANY-CNAMEChasing",
			qtype:          dns.TypeANY,
			qname:          "0123456789.benchmark.example.com.",
			cnameChasing:   true,
			maxConcurrency: []int{1},
		},
	}

	for _, tc := range testCases {
		for _, maxConcurrency := range tc.maxConcurrency {
			b.Run(fmt.Sprintf("%s-maxConcurrency=%d", tc.name, maxConcurrency), func(b *testing.B) {
				config := makeTestServerConfig(false, false)
				config.MaxConcurrency = maxConcurrency
				config.HandlerConfig.CNAMEChasing = tc.cnameChasing
				config.HandlerConfig.MaxCNAMEHops = 10
				portMap, srv := makeTestServer(b, config)
				defer srv.Shutdown()

				addr := portMap["udp"]
				host, port, err := net.SplitHostPort(addr)
				require.Nil(b, err)
				portNum, err := strconv.Atoi(port)
				require.Nil(b, err)
				udpAddr := net.UDPAddr{
					IP:   net.ParseIP(host),
					Port: portNum,
				}
				conn, err := net.DialUDP("udp", nil, &udpAddr)
				require.Nil(b, err, "Error connecting to server")
				defer conn.Close()

				// Strategy: Spray DNS packets as fast as possible,
				// accepting that many or most will be lost.
				// Only count replies.

				b.ResetTimer()
				go sprayUDP(conn, math.MaxInt, tc.qname, tc.qtype)
				readBuf := make([]byte, 65536)
				n := 0
				for range b.N {
					n, err = conn.Read(readBuf)
					require.Nil(b, err)
				}
				b.StopTimer()
				// Terminate the spraying thread.
				conn.Close()

				lastResponse := dns.Msg{}
				err = lastResponse.Unpack(readBuf[:n])
				require.NoError(b, err)
				// To confirm response content, uncomment:
				// b.Logf("%v", lastResponse)

				seconds := b.Elapsed().Seconds()
				b.ReportMetric(float64(b.N)/seconds, "responses/sec")
				// This metric is less meaningful than QPS, so we can hide it.
				b.ReportMetric(0, "ns/op")
			})
		}
	}
}
