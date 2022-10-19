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
	"io"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/stats"
	"github.com/facebookincubator/dns/dnsrocks/metrics"
	"github.com/facebookincubator/dns/dnsrocks/testaid"
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
	return s
}

// makeTestServer spins up standalone DNS servers based on the ServerConfig `config`.
// returns a map of `network`/listening address.
// network can be any of udp, tcp, tcp-tls
func makeTestServer(t *testing.T, config ServerConfig) (map[string]string, *Server) {
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
	assert.Nil(t, srv.Start(), "Failed to start servers!")

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
	assert.Len(t, portMap, 1)
	assert.Contains(t, portMap, "udp")
}

// TestRunUDPTCPTestServer simple test to ensure that we create a TCP server.
// Currently, there is always a UDP server which is started.
func TestRunUDPTCPTestServer(t *testing.T) {
	portMap, srv := RunUDPTCPTestServer(t)
	defer srv.Shutdown()
	assert.Len(t, portMap, 2)
	assert.Contains(t, portMap, "udp")
	assert.Contains(t, portMap, "tcp")
}

// TestRunUDPTLSTestServer simple test to ensure that we create a TCP server.
// Currently, there is always a UDP server which is started.
func TestRunUDPTLSTestServer(t *testing.T) {
	portMap, srv := RunUDPTLSTestServer(t)
	defer srv.Shutdown()
	assert.Len(t, portMap, 2)
	assert.Contains(t, portMap, "udp")
	assert.Contains(t, portMap, "tcp-tls")
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
			assert.Nil(t, err)
			assert.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			assert.Equal(t, tc.target, cname)
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
	assert.Nil(t, err)

	// We expect a FORMERR and nothing in question or answer section (and anything
	// else for that matter).
	assert.Equal(t, r.Rcode, dns.RcodeFormatError)
	assert.Equal(t, 0, len(r.Question))
	assert.Equal(t, 0, len(r.Answer))
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
			assert.Nil(t, err)
			assert.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			assert.Equal(t, tc.target, cname)
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
			assert.Nil(t, err)
			assert.NotEqual(t, 0, len(r.Answer))

			cname := r.Answer[0].(*dns.CNAME).Target
			assert.Equal(t, tc.target, cname)
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
	assert.NoError(t, err, "Error setting write deadline")
	err = co.WriteMsg(m)
	assert.Nil(t, err, "Error writing message")

	err = co.SetReadDeadline(time.Now().Add(2 * time.Second))
	assert.NoError(t, err, "Error setting Read Deadline")
	r, err = co.ReadMsg()
	assert.Nil(t, err, "Error reading message")
	assert.Equal(t, m.Id, r.Id, "Response message does not have the same ID")

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
	assert.Nil(t, err, "Error connecting to server")
	defer co.Close()

	// First request, which is using ReadTimeout
	r := exchange(t, co, c, m)
	assert.Equal(t, r.Question[0].Name, "example.com.", "Mismatching question")
	time.Sleep(config.TCPIdleTimeout + 1)

	// Now we are going to read something and expect to get EOF before the
	// timeout occurs.
	one := make([]byte, 1)
	err = co.SetReadDeadline(time.Now().Add(1 * time.Second))
	assert.NoError(t, err, "Error setting Read Deadline")

	_, err = co.Read(one)
	assert.Equal(t, io.EOF, err, "Connection was expected to be closed by server")
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
	assert.Nil(t, err, "Error connecting to server")
	defer co.Close()

	for _, m := range msgs {
		r := exchange(t, co, c, m)
		assert.Equal(t, m.Question[0].Name, r.Question[0].Name, "Mismatching question name")
		assert.Equal(t, m.Question[0].Qtype, r.Question[0].Qtype, "Mismatching question type")
	}
}
