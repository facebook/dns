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

package dnsserver

import (
	"fmt"
	"io"
	"strings"

	"github.com/facebook/dns/dnsrocks/db"

	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Logger is an interface for logging messages
type Logger interface {
	// LogFailed logs a message when we could not construct an answer
	LogFailed(state request.Request, ecs *dns.EDNS0_SUBNET, loc *db.Location, responseTime int64)
	// Log logs a DNS response
	Log(state request.Request, r *dns.Msg, ecs *dns.EDNS0_SUBNET, loc *db.Location, responseTime int64)
}

// TextLogger logs to an io.Writer
type TextLogger struct {
	IoWriter io.Writer
}

// Log is used to log to an ioWriter.
func (l *TextLogger) Log(state request.Request, _ *dns.Msg, _ *dns.EDNS0_SUBNET, _ *db.Location, _ int64) {
	fmt.Fprintf(l.IoWriter, "[%s] %s %s %s\n",
		state.IP(), strings.ToUpper(state.Proto()),
		state.Name(), state.Type())
}

// LogFailed is used to log failures
func (l *TextLogger) LogFailed(state request.Request, ecs *dns.EDNS0_SUBNET, loc *db.Location, responseTime int64) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, dns.RcodeServerFailure)
	l.Log(state, m, ecs, loc, responseTime)
}

// DummyLogger logs nothing
type DummyLogger struct{}

// Log is used to log to an ioWriter.
func (l *DummyLogger) Log(_ request.Request, _ *dns.Msg, _ *dns.EDNS0_SUBNET, _ *db.Location, _ int64) {
}

// LogFailed is used to log failures
func (l *DummyLogger) LogFailed(_ request.Request, _ *dns.EDNS0_SUBNET, _ *db.Location, _ int64) {
}
