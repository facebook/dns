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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strconv"
	"strings"

	tlsc "github.com/facebook/dns/dnsrocks/tlsconfig"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

type dotTLSAHandler struct {
	dotTLSAPrefix string
	rr            dns.TLSA
	ttl           uint32
	Next          plugin.Handler
}

const defaultTLSATtl uint32 = 3600

// newDotTLSA initialize a new dotTLSAHandler.
// Given a certificate, create a Domain Issued Certificate (DANE-EE) with the
// SHA-256 of the SPKI: https://tools.ietf.org/html/rfc7671#section-5.1
// Usage: 3 (DANE-EE), Selector: 1 (SPKI), Matching-Type: 1 (sha256)
// FIXME: Currently, this is reloading the certs from disk and reparsing it
// while that data was already loaded when initiating the TLS server stack.
// While sub-optimal, it is not a big deal and may be easier to just leave it
// like this.
func newDotTLSA(tlsconfig *tlsc.TLSConfig) (*dotTLSAHandler, error) {
	var cert tls.Certificate
	var x509cert *x509.Certificate
	var err error
	dh := new(dotTLSAHandler)
	if tlsconfig.DoTTLSATtl == 0 {
		dh.ttl = defaultTLSATtl
	} else {
		dh.ttl = tlsconfig.DoTTLSATtl
	}
	if cert, err = tlsc.LoadTLSCertFromFile(tlsconfig); err != nil {
		return nil, fmt.Errorf("loading certs from %v: %w", tlsconfig, err)
	}
	// cert.Certificate is an chain of one or more certificate, leaf first.
	// https://golang.org/pkg/crypto/tls/#Certificate
	// We only care about the Leaf.
	if x509cert, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
		return nil, fmt.Errorf("converting tls.Certificate to x509.Certificate %v: %w", cert, err)
	}
	dh.dotTLSAPrefix = "_" + strconv.Itoa(tlsconfig.Port) + "._tcp."
	glog.Infof("dotTLSAHandler will match prefix: %s", dh.dotTLSAPrefix)
	err = dh.rr.Sign(3, 1, 1, x509cert)
	return dh, err
}

// SetTTL allows setting the TTL of DoT TLSA records.
func (dh *dotTLSAHandler) SetTTL(ttl uint32) *dotTLSAHandler {
	dh.ttl = ttl
	return dh
}

func (dh *dotTLSAHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question[0].Name) <= len(dh.dotTLSAPrefix) || !strings.HasPrefix(strings.ToLower(r.Question[0].Name), dh.dotTLSAPrefix) {
		return plugin.NextOrFailure(dh.Name(), dh.Next, ctx, w, r)
	}
	state := request.Request{W: w, Req: r}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = true
	m.Authoritative = true
	if state.QType() == dns.TypeTLSA {
		var rr dns.RR = new(dns.TLSA)
		rr.(*dns.TLSA).Hdr = dns.RR_Header{Name: r.Question[0].Name, Ttl: dh.ttl, Rrtype: dns.TypeTLSA, Class: state.QClass()}
		rr.(*dns.TLSA).Usage = dh.rr.Usage
		rr.(*dns.TLSA).Selector = dh.rr.Selector
		rr.(*dns.TLSA).MatchingType = dh.rr.MatchingType
		rr.(*dns.TLSA).Certificate = dh.rr.Certificate

		m.Answer = []dns.RR{rr}
	}

	state.SizeAndDo(m)
	m = state.Scrub(m)
	err := state.W.WriteMsg(m)
	if err != nil {
		// nolint: nilerr
		return dns.RcodeServerFailure, nil
	}
	return dns.RcodeSuccess, nil
}

func (dh *dotTLSAHandler) Name() string { return "dot tlsa" }
