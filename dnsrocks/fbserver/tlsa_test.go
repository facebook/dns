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
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"os"
	"testing"

	"github.com/facebook/dns/dnsrocks/dnsserver/test"
	"github.com/facebook/dns/dnsrocks/testaid"
	tlsc "github.com/facebook/dns/dnsrocks/tlsconfig"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// Create a TLSConfig suitable for dotTLSAHandler
func makeDotTLSAHandlerTLSConfig(certfile string, port int) *tlsc.TLSConfig {
	return &tlsc.TLSConfig{
		CertFile: certfile,
		KeyFile:  certfile,
		Port:     port,
	}
}

// TestDotTLSAHandlerBadTypeGoodPrefix tests that return NOERROR/NODATA when a
// matching QName pattern is provided but for a different QType than TLSA.
func TestDotTLSAHandlerBadTypeGoodPrefix(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tlsconfig := makeDotTLSAHandlerTLSConfig(certfile, 1234)
	w := &test.ResponseWriter{}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("_1234._tcp.example.com."), dns.TypeA)
	rec := dnstest.NewRecorder(w)
	dh, err := newDotTLSA(tlsconfig)
	require.Nil(t, err)
	rc, err := dh.ServeDNS(context.TODO(), rec, req)

	require.Nil(t, err)
	require.Equal(t, rc, dns.RcodeSuccess)
	require.Equal(t, len(rec.Msg.Answer), 0)
}

// TestDotTLSAHandlerAnyTypeBadPrefix tests that we continue to the next handler when a
// qname not matching the prefix is queried, whatever the QType.
func TestDotTLSAHandlerAnyTypeBadPrefix(t *testing.T) {
	testCases := []uint16{dns.TypeTLSA, dns.TypeA}

	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tlsconfig := makeDotTLSAHandlerTLSConfig(certfile, 1234)
	expectedError := "plugin/dot tlsa: no next plugin found"
	for _, tc := range testCases {
		t.Run(dns.TypeToString[tc], func(t *testing.T) {
			w := &test.ResponseWriter{}
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn("_12345._tcp.example.com."), tc)
			rec := dnstest.NewRecorder(w)
			dh, err := newDotTLSA(tlsconfig)
			require.Nil(t, err)
			rc, err := dh.ServeDNS(context.TODO(), rec, req)

			require.Equal(t, rc, dns.RcodeServerFailure)
			require.Equal(t, expectedError, err.Error())
		})
	}
}

// loadSPKIFromCert parses a cert file and extract the SPKI
func loadSPKIFromCert(t *testing.T, certfile string) []byte {
	cert, err := tls.LoadX509KeyPair(certfile, certfile)
	require.Nilf(t, err, "Loading X509 keypair from %v", certfile)

	x509cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.Nilf(t, err, "Converting tls.Certificate to x509.Certificate %v", cert)

	return x509cert.RawSubjectPublicKeyInfo
}

// TestDotTLSAHandlerGoodTypeGoodPrefix tests that we return a properly formatted TLSA
// record, e.g Usage (3 == DANE-EE), Selector (1 == SPKI), MatchingType (1 == SHA-256)
// We create a temporary x509 cert from which we extract the SPKI in its hexlified
// form and check against TLSA.Certificate
func TestDotTLSAHandlerGoodTypeGoodPrefix(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)

	spki := loadSPKIFromCert(t, certfile)
	spkisum := sha256.Sum256(spki)

	testCases := []struct {
		qname              string
		ttl                uint32
		answerUsage        uint8
		answerSelector     uint8
		answerMatchingType uint8
		answerCertificate  string
		expectedCount      int
	}{
		{
			// _1234._tcp.example.com. matches dotTLSAHandler pattern
			// also has default TTL.
			qname:              "_1234._tcp.example.com.",
			answerUsage:        3,
			answerSelector:     1,
			answerMatchingType: 1,
			answerCertificate:  hex.EncodeToString(spkisum[:]),
			expectedCount:      1,
		},
		{
			// _1234._TCP.example.com. matches dotTLSAHandler pattern (case insensitive)
			// TTL should be set to 60.
			qname:              "_1234._TCP.example.com.",
			ttl:                60,
			answerUsage:        3,
			answerSelector:     1,
			answerMatchingType: 1,
			answerCertificate:  hex.EncodeToString(spkisum[:]),
			expectedCount:      1,
		},
	}

	for _, tc := range testCases {
		w := &test.ResponseWriter{}
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), dns.TypeTLSA)
		tlsconfig := makeDotTLSAHandlerTLSConfig(certfile, 1234)
		tlsconfig.DoTTLSATtl = tc.ttl
		rec := dnstest.NewRecorder(w)
		dh, err := newDotTLSA(tlsconfig)
		require.Nil(t, err)
		rc, err := dh.ServeDNS(context.TODO(), rec, req)

		require.Equal(t, dns.RcodeSuccess, rc)
		require.Nil(t, err)
		require.Equal(t, dns.RcodeSuccess, rec.Rcode, "RcodeSuccess was expected to be returned.")
		require.Equal(t, tc.expectedCount, len(rec.Msg.Answer), "Number of answers should be %d", tc.expectedCount)
		require.Equal(t, tc.answerUsage, rec.Msg.Answer[0].(*dns.TLSA).Usage, "Answer should be %s", tc.answerUsage)
		require.Equal(t, tc.answerSelector, rec.Msg.Answer[0].(*dns.TLSA).Selector, "Answer should be %s", tc.answerSelector)
		require.Equal(t, tc.answerMatchingType, rec.Msg.Answer[0].(*dns.TLSA).MatchingType, "Answer should be %s", tc.answerMatchingType)
		require.Equal(t, tc.answerCertificate, rec.Msg.Answer[0].(*dns.TLSA).Certificate, "Answer should be %#v", tc.answerCertificate)
		if tc.ttl == 0 {
			require.Equal(t, defaultTLSATtl, rec.Msg.Answer[0].(*dns.TLSA).Hdr.Ttl, "Answer TTL does not match default")
		} else {
			require.Equal(t, tc.ttl, rec.Msg.Answer[0].(*dns.TLSA).Hdr.Ttl, "Answer TTL does not match provided TTL")
		}
	}
}
