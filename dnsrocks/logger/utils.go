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

package logger

import (
	"crypto/tls"
	"strings"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

const (

	// Header.Bits
	_QR = 1 << 15 // query/response (response=1)
	_AA = 1 << 10 // authoritative
	_TC = 1 << 9  // truncated
	_RD = 1 << 8  // recursion desired
	_RA = 1 << 7  // recursion available
	_Z  = 1 << 6  // Z
	_AD = 1 << 5  // authenticated data
	_CD = 1 << 4  // checking disabled
)

// TLSVersionStrings is a map of TLS version IDs to their string representation.
var TLSVersionStrings = map[uint16]string{
	tls.VersionTLS13: "TLS1.3",
	tls.VersionTLS12: "TLS1.2",
	tls.VersionTLS11: "TLS1.1",
	tls.VersionTLS10: "TLS1.0",
	// nolint:staticcheck
	tls.VersionSSL30: "SSL3.0",
}

// isSonar is an half-baked mechanism to detect whether or not a domain is a
// sonar domain.
// This is based on `ti/data/tailers/lib/sonar_util.py`'s `is_sonar_domain` and
// simplified to avoid use of regexes. The idea is that we will essentially be
// more permissive and have some false positive, which will get caught on the
// tailer side anyway, but at least avoid too much useless regex computation on
// every calls.
func isSonar(state request.Request) bool {
	// state.Name() is lower-case!
	name := state.Name()
	if strings.HasSuffix(name, ".igsonar.com.") {
		return true
	}
	if strings.HasSuffix(name, ".fbcdn.net.") && strings.Contains(name, "sonar") {
		return true
	}
	if strings.HasSuffix(name, ".snr.whatsapp.net.") {
		return true
	}
	return false
}

// Config represents the configuration used for our scribecat client.
type Config struct {
	FlushInterval int
	Timeout       int
	Retry         int
	Target        string
	Remote        string
	LogFormat     string
	SamplingRate  float64
	Category      string
}

// RequestProtocol return a string version of the protocol (UDP, TCP or TLS)
// TLS has its version appended if it is available (which should always be)
func RequestProtocol(state request.Request) string {
	proto := state.Proto() // Protocol used
	if proto == "tcp" {
		if tls := state.W.(dns.ConnectionStater).ConnectionState(); tls != nil {
			if p, ok := TLSVersionStrings[tls.Version]; ok {
				proto = p
			} else {
				proto = "tls_unknown"
			}
		}
	}
	return strings.ToUpper(proto)
}

// computeDNSFlag return a uint to represent flags set in dns message header
func computeDNSFlag(r *dns.Msg) uint16 {
	// https://www.freesoft.org/CIE/RFC/2065/40.htm for format

	var dnsFlags = uint16(r.Opcode)<<11 | uint16(r.Rcode&0xF)
	if r.Response {
		dnsFlags |= _QR
	}
	if r.Authoritative {
		dnsFlags |= _AA
	}
	if r.Truncated {
		dnsFlags |= _TC
	}
	if r.RecursionDesired {
		dnsFlags |= _RD
	}
	if r.RecursionAvailable {
		dnsFlags |= _RA
	}
	if r.Zero {
		dnsFlags |= _Z
	}
	if r.AuthenticatedData {
		dnsFlags |= _AD
	}
	if r.CheckingDisabled {
		dnsFlags |= _CD
	}

	return dnsFlags
}
