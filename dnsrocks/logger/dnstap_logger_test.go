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
	"github.com/stretchr/testify/require"
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
			require.NotNil(t, err)
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
			require.Nil(t, err)
			require.NotNil(t, l)
		})
	}
}

// TestLoggerErrorBadTarget makes sure that we Error if the provided
// target is not valid.
func TestLoggerErrorBadTarget(t *testing.T) {
	slc := Config{Target: "invalid"}
	_, err := NewLogger(slc)
	require.NotNil(t, err)
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
			require.Nil(t, err)
			require.NotNil(t, l)
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
			require.NotNil(t, err)
		})
	}
}

// TestLoggerErrorBadStdoutFormat makes sure that we Error if the provided
// stdout format is not valid.
func TestLoggerErrorBadStdoutFormat(t *testing.T) {
	slc := Config{Target: "stdout", LogFormat: "invalid"}
	_, err := NewLogger(slc)
	require.NotNil(t, err)
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
			require.NotNil(t, l)
			require.Nil(t, err)
		})
	}
}

// TestIsSonarMoreNames test various names. Their signature may me invalid or
// such, but they will pass out filter.
func TestIsSonarMoreNames(t *testing.T) {
	testCases := []string{
		"w8dd9dd9310-GMYTANJUGM2DAMZX.snr.whatsapp.net.",
		"w8dd9dd9310KMNRWGYD-GMYTANJUGM2DAMZX.snr.whatsapp.net.",
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
			require.True(t, isSonar(s))
		})
	}
}

// TestIsSonarInvalidNames test slight modifications of valid names and ensure
// they are not accepted as sonar candidates.
func TestIsSonarInvalidNames(t *testing.T) {
	testCases := []string{
		"sonar.whatsapp.net.",
		"w8dd9dd9310KMNRWGYD-GMYTANJUGM2DAMZX-snr.whatsapp.net.",
		"22iyjg4y0.sonar.instagram.com.",
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
			require.False(t, isSonar(s))
		})
	}
}
