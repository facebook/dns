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
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/dnsserver"
	"github.com/facebook/dns/dnsrocks/testaid"
	"github.com/facebook/dns/dnsrocks/testutils"
)

// Fuzz tests for the full server. Each fuzzed input is delivered to a running
// server over a real socket, and the server must still answer a valid probe
// query afterwards. A panic anywhere in the serving path crashes the process,
// and a hang or a jammed throttle makes the probe time out.
//
// Run locally with e.g.:
//
//	buck2 run fbcode//dns/fbdns/fbserver:fbserver_test -- -test.run='^$' -test.fuzz='^FuzzUDP$' -test.fuzztime=60s
//
// Continuous fuzzing runs in Lionhead via the go_native_fuzzer_bundle targets
// in the BUCK file.

const fuzzIOTimeout = 5 * time.Second

// startFuzzServer starts a UDP, TCP and TLS server with most optional
// handlers enabled, so that fuzzed inputs reach as much code as possible.
// It returns the listening addresses and a pool trusting the server's
// certificate.
func startFuzzServer(f *testing.F) (map[string]string, *x509.CertPool) {
	certfile := testaid.MkTestCert(f)
	f.Cleanup(func() { os.Remove(certfile) })
	certPEM, err := os.ReadFile(certfile)
	require.NoError(f, err)
	roots := x509.NewCertPool()
	require.True(f, roots.AppendCertsFromPEM(certPEM))

	config := makeTestServerConfig(true, true)
	config.TLSConfig.CertFile = certfile
	config.TLSConfig.KeyFile = certfile
	config.MaxConcurrency = 10
	config.MaxUDPSize = 1232
	config.WhoamiDomain = "whoami.example.com"
	config.NSID = true
	config.HandlerConfig.CNAMEChasing = true
	config.HandlerConfig.MaxCNAMEHops = 10
	config.CacheConfig = dnsserver.CacheConfig{Enabled: true, LRUSize: 1024}
	config.DNSSECConfig = DNSSECConfig{
		Zones: "example.com",
		Keys:  testutils.FixturePath("../testdata/data/", KEYNAME),
	}

	portMap, srv := makeTestServer(f, config)
	f.Cleanup(srv.Shutdown)
	return portMap, roots
}

func newProbe() *dns.Msg {
	return new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA)
}

// fuzzSeedQueries returns a set of valid and slightly invalid DNS messages
// covering the main features of the server.
func fuzzSeedQueries(f *testing.F) [][]byte {
	dnssec := new(dns.Msg).SetQuestion("example.com.", dns.TypeDNSKEY)
	dnssec.SetEdns0(4096, true)

	ecs := new(dns.Msg).SetQuestion("www.example.com.", dns.TypeA)
	o, err := dnsserver.MakeOPTWithECS("1.1.1.0/24")
	require.NoError(f, err)
	ecs.Extra = []dns.RR{o}

	nsid := new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA)
	nsid.SetEdns0(1232, false)
	nsid.IsEdns0().Option = append(nsid.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})

	badVers := new(dns.Msg).SetQuestion("example.com.", dns.TypeSOA)
	badVers.SetEdns0(512, false)
	badVers.IsEdns0().SetVersion(1)

	multipleQuestions := new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA)
	multipleQuestions.Question = append(multipleQuestions.Question, multipleQuestions.Question[0])

	statusOpcode := new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA)
	statusOpcode.Opcode = dns.OpcodeStatus

	response := new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA)
	response.Response = true

	msgs := []*dns.Msg{
		new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeA),
		new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeAAAA),
		new(dns.Msg).SetQuestion("foo.example.com.", dns.TypeHTTPS),
		new(dns.Msg).SetQuestion("longchain.example.com.", dns.TypeA),
		new(dns.Msg).SetQuestion("cycle.example.com.", dns.TypeA),
		new(dns.Msg).SetQuestion("www.nonauth.example.com.", dns.TypeA),
		new(dns.Msg).SetQuestion("lotofns.example.org.", dns.TypeNS),
		new(dns.Msg).SetQuestion("missing.example.org.", dns.TypeAAAA),
		new(dns.Msg).SetQuestion("www.nothere.org.", dns.TypeA),
		new(dns.Msg).SetQuestion("example.com.", dns.TypeANY),
		new(dns.Msg).SetQuestion("example.com.", dns.TypeSOA),
		new(dns.Msg).SetQuestion("whoami.example.com.", dns.TypeTXT),
		dnssec,
		ecs,
		nsid,
		badVers,
		multipleQuestions,
		statusOpcode,
		response,
	}

	seeds := make([][]byte, 0, len(msgs)+2)
	seeds = append(seeds, []byte{}, []byte{0})
	for _, m := range msgs {
		buf, err := m.Pack()
		require.NoError(f, err)
		seeds = append(seeds, buf)
	}
	return seeds
}

// tcpFrame prepends the two-byte length prefix used by DNS over TCP.
func tcpFrame(msgs ...[]byte) []byte {
	var out []byte
	for _, m := range msgs {
		n := len(m)
		if n > math.MaxUint16 {
			panic("message too long for DNS over TCP")
		}
		out = binary.BigEndian.AppendUint16(out, uint16(n))
		out = append(out, m...)
	}
	return out
}

func addStreamSeeds(f *testing.F) {
	seeds := fuzzSeedQueries(f)
	for _, s := range seeds {
		f.Add(tcpFrame(s))
	}
	// Pipelined queries, and a frame that is longer than the data sent.
	f.Add(tcpFrame(seeds[2], seeds[3]))
	f.Add(tcpFrame(seeds[2])[:len(seeds[2])/2])
}

// sendStream writes data on conn, half-closes it and reads until the server
// closes the connection.
func sendStream(t *testing.T, conn net.Conn, data []byte, closeWrite func() error) {
	require.NoError(t, conn.SetDeadline(time.Now().Add(fuzzIOTimeout)))
	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, conn)
		readErr <- err
	}()
	// Write errors are expected when the server rejects the stream early.
	if _, err := conn.Write(data); err == nil {
		_ = closeWrite()
	}
	err := <-readErr
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		t.Fatalf("server did not close the connection within %s", fuzzIOTimeout)
	}
}

// probeStream checks that the server still answers over a stream transport.
func probeStream(t *testing.T, c *dns.Client, addr string) {
	m := newProbe()
	r, _, err := c.Exchange(m, addr)
	require.NoError(t, err, "server stopped answering over %s", c.Net)
	require.Equal(t, m.Id, r.Id)
}

func FuzzUDP(f *testing.F) {
	portMap, _ := startFuzzServer(f)
	addr := portMap["udp"]
	conn, err := net.Dial("udp", addr)
	require.NoError(f, err)
	f.Cleanup(func() { conn.Close() })

	for _, s := range fuzzSeedQueries(f) {
		f.Add(s)
	}

	buf := make([]byte, dns.MaxMsgSize)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := conn.Write(data)
		require.NoError(t, err)

		// The probe is sent after the fuzzed packet on the same socket, so
		// replies to the fuzzed packet (if any) are read and discarded here.
		probe, err := newProbe().Pack()
		require.NoError(t, err)
		for range fuzzIOTimeout / time.Second {
			_, err = conn.Write(probe)
			require.NoError(t, err)
			if readUDPReply(t, conn, buf, probe) {
				return
			}
		}
		t.Fatalf("server stopped answering over UDP")
	})
}

// readUDPReply reads datagrams for up to a second, until it finds the reply
// to probe.
func readUDPReply(t *testing.T, conn net.Conn, buf, probe []byte) bool {
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	for {
		n, err := conn.Read(buf)
		if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
			return false
		}
		require.NoError(t, err)
		if n >= 3 && buf[0] == probe[0] && buf[1] == probe[1] && buf[2]&0x80 != 0 {
			return true
		}
	}
}

func FuzzTCP(f *testing.F) {
	portMap, _ := startFuzzServer(f)
	addr := portMap["tcp"]
	addStreamSeeds(f)
	client := &dns.Client{Net: "tcp", Timeout: fuzzIOTimeout}

	f.Fuzz(func(t *testing.T, data []byte) {
		conn, err := net.DialTimeout("tcp", addr, fuzzIOTimeout)
		require.NoError(t, err)
		defer conn.Close()
		sendStream(t, conn, data, conn.(*net.TCPConn).CloseWrite)
		probeStream(t, client, addr)
	})
}

func FuzzTLS(f *testing.F) {
	portMap, roots := startFuzzServer(f)
	addr := portMap["tcp-tls"]
	addStreamSeeds(f)
	tlsConfig := &tls.Config{
		RootCAs:            roots,
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
	}
	client := &dns.Client{Net: "tcp-tls", TLSConfig: tlsConfig, Timeout: fuzzIOTimeout}

	f.Fuzz(func(t *testing.T, data []byte) {
		dialer := &net.Dialer{Timeout: fuzzIOTimeout}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		require.NoError(t, err)
		defer conn.Close()
		sendStream(t, conn, data, conn.CloseWrite)
		probeStream(t, client, addr)
	})
}
