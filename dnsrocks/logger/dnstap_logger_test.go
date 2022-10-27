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
	"fmt"
	"testing"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
)

// TestLoggerErrorBadSamplingRate makes sure that we Error if the provided
// sampling rate is not valid.
func TestLoggerErrorBadSamplingRate(t *testing.T) {
	testCases := []float64{
		-0.1,
		1.5,
		1.000000001,
		-0.00000001,
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("sampling rate %f", tc), func(t *testing.T) {
			slc := Config{SamplingRate: tc, Target: "stdout", LogFormat: "text"}
			_, err := NewLogger(slc)
			assert.NotNil(t, err)
		})
	}
}

// TestLoggerNotErrorGoodSamplingRate makes sure that we do not Error if the
// provided sampling rate is valid.
func TestLoggerNotErrorGoodSamplingRate(t *testing.T) {
	testCases := []float64{
		0.0,
		1.0,
		0.5,
		0.00000001,
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("sampling rate %f", tc), func(t *testing.T) {
			slc := Config{SamplingRate: tc, Target: "stdout", LogFormat: "text"}
			l, err := NewLogger(slc)
			assert.Nil(t, err)
			assert.NotNil(t, l)
		})
	}
}

// TestLoggerErrorBadTarget makes sure that we Error if the provided
// target is not valid.
func TestLoggerErrorBadTarget(t *testing.T) {
	slc := Config{Target: "invalid"}
	_, err := NewLogger(slc)
	assert.NotNil(t, err)
}

// TestLoggerNotErrorGoodTarget makes sure that we do not Error if the
// provided target is valid.
func TestLoggerNotErrorGoodTarget(t *testing.T) {
	testCases := []string{
		"stdout",
		"tcp",
		"unix",
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("target %s", tc), func(t *testing.T) {
			var remote string
			switch tc {
			case "tcp":
				remote = "127.0.0.1:6000"
			case "unix":
				remote = "/var/run/dnstap.sock"
			default:
				remote = ""
			}
			slc := Config{Target: tc, Remote: remote, LogFormat: "text"}
			l, err := NewLogger(slc)
			assert.Nil(t, err)
			assert.NotNil(t, l)
		})
	}
}

func TestLoggerErrorsRemoteProtoNoTarget(t *testing.T) {
	testCases := []string{
		"tcp",
		"unix",
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("target %s", tc), func(t *testing.T) {
			slc := Config{Target: tc, LogFormat: "text"}
			_, err := NewLogger(slc)
			assert.NotNil(t, err)
		})
	}
}

// TestLoggerErrorBadStdoutFormat makes sure that we Error if the provided
// stdout format is not valid.
func TestLoggerErrorBadStdoutFormat(t *testing.T) {
	slc := Config{Target: "stdout", LogFormat: "invalid"}
	_, err := NewLogger(slc)
	assert.NotNil(t, err)
}

// TestLoggerNotErrorGoodTarget makes sure that we do not Error if the
// provided target is valid.
func TestLoggerNotErrorGoodLogFormat(t *testing.T) {
	testCases := []string{
		"text",
		"yaml",
		"json",
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("log format %s", tc), func(t *testing.T) {
			slc := Config{Target: "stdout", LogFormat: tc}
			l, err := NewLogger(slc)
			assert.NotNil(t, l)
			assert.Nil(t, err)
		})
	}
}

// TestIsSonarBadIG tests names that looks like sonat but are not.
// Those would be filtered out by tailers, but for our simple sonar domain
// detector, those are being valid.
// source fbcode/ti/data/tailers/test/sonar_util.py
func TestIsSonarBadIG(t *testing.T) {
	testCases := []string{
		"foo.igsonar.com.",
		"12iyjg4y0.igsonar.com.",                          // do not starts with 2
		"22yjg4y0.igsonar.com.",                           // 1 char short
		"22iyjgw4y0.igsonar.com.",                         // 1 char too many
		"22iy-g4y0.igsonar.com.",                          // 1 invalid char
		"1eyaqaaydaaenyrort3p32qcsua000000.igsonar.com.",  // ! starts with 2
		"2yaqaaydaaenyrort3p32qcsua000000.igsonar.com.",   // 1 char short
		"2aeyaqaaydaaenyrort3p32qcsua000000.igsonar.com.", // 1 char long
		"2e-aqaaydaaenyrort3p32qcsua000000.igsonar.com.",  // invalid char
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc, dns.TypeA)
			s := request.Request{W: nil, Req: m}
			assert.True(t, isSonar(s))
		})
	}
}

// TestIsSonarMoreNames test various names. Their signature may me invalid or
// such, but they will pass out filter.
func TestIsSonarMoreNames(t *testing.T) {
	testCases := []string{
		"w8dd9dd9310-GMYTANJUGM2DAMZX.snr.whatsapp.net.",
		"w8dd9dd9310KMNRWGYD-GMYTANJUGM2DAMZX.snr.whatsapp.net.",
		"22iyjg4y0.igsonar.com.",
		"2eyaqaaydaaenyrort3p32qcsua000000.igsonar.com.",
		"3mavldobrdjdbe3t.igsonar.com.",
		"3jbvldobmetacaadamaarxcf2gpn7pkakkqa.igsonar.com.",
		"f41366ce340-kdywlzry1535569283-sonar.xy.fbcdn.net.",
		"f41366ce340-kdywlzry1535569283-sonar.snr.fbcdn.net.",
		"f3d77e02580klz54ggi-uavvfioa1427990455-sonar.snr.fbcdn.net.",
		"f8778542230keybagbr3uurxbbfln2or43v6my-gpihagsr1427990455-sonar.xy.fbcdn.net.",
		"fd45417ba30KDACNUXI-fgeytrfl1427990452-sonar.2914.fna.fbcdn.net.",
	}
	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc, dns.TypeA)
			s := request.Request{W: nil, Req: m}
			assert.True(t, isSonar(s))
		})
	}
}

// TestIsSonarInvalidNames test slight modifications of valid names and ensure
// they are not accepted as sonar candidates.
func TestIsSonarInvalidNames(t *testing.T) {
	testCases := []string{
		"sonar.whatsapp.net.",
		"w8dd9dd9310KMNRWGYD-GMYTANJUGM2DAMZX-snr.whatsapp.net.",
		"22iyjg4y0.igsonar.instagram.com.",
		"3jbvldobmetacaadamaarxcf2gpn7pkakkqa.instagram.com.",
		"f41366ce340-kdywlzry1535569283-sonar.xy.facebook.com.",
		"f41366ce340-kdywlzry1535569283-snr.fbcdn.net.",
		"f3d77e02580klz54ggi-uavvfioa1427990455.snr.fbcdn.net.",
		"f8778542230keybagbr3uurxbbfln2or43v6my-gpihagsr1427990455-sonar.xy.fbcdn.com.",
	}
	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tc, dns.TypeA)
			s := request.Request{W: nil, Req: m}
			assert.False(t, isSonar(s))
		})
	}
}

// TestComputeDNSFlag test computeDNSFlag set the right value w.r.t. each flag
func TestComputeDNSFlag(t *testing.T) {
	m := new(dns.Msg)
	var flags int
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.Response = true
	flags |= _QR
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.Authoritative = true
	flags |= _AA
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.Truncated = true
	flags |= _TC
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.RecursionDesired = true
	flags |= _RD
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.RecursionAvailable = true
	flags |= _RA
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.Zero = true
	flags |= _Z
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.AuthenticatedData = true
	flags |= _AD
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.CheckingDisabled = true
	flags |= _CD
	assert.Equal(t, int(computeDNSFlag(m)), flags)

	m.Opcode = 5
	m.Rcode = 3
	flags |= (5 << 11) | 3
	assert.Equal(t, int(computeDNSFlag(m)), flags)
}
