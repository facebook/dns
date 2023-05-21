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
	"log"
	"net"
	"os"
	"time"

	"github.com/facebookincubator/dns/dnsrocks/db"

	msg "github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/request"
	dnstap "github.com/dnstap/golang-dnstap"
	"github.com/golang/glog"
	"github.com/miekg/dns"
	"google.golang.org/protobuf/proto"
)

var logger = log.New(os.Stderr, "", log.LstdFlags)

type anyDNSTapOutPut interface {
	RunOutputLoop()
	GetOutputChannel() chan []byte
}

/*
*
By getting this package its own source, we can seed it without stepping on the
global source which may be initialize in some other way.
Also, any application using this package, will get the seeding for free rather
than having to do it themselves.
See also dns/fbdns/db/wrs.go
*/
var (
	localRand = db.NewRand()
)

// DNSTapLogger logs to dnstap output
type DNSTapLogger struct {
	dnsTapOutput anyDNSTapOutPut
	samplingRate float64
}

// NewLogger initialize a DNSTapLogger by setting the right outputs and format
func NewLogger(config Config) (l *DNSTapLogger, err error) {
	if config.SamplingRate < 0.0 || config.SamplingRate > 1.0 {
		return nil, fmt.Errorf("Sampling rate should be >= 0.0 and <= 1.0. Got %f", config.SamplingRate)
	}
	l = &DNSTapLogger{samplingRate: config.SamplingRate}
	switch config.Target {
	case "stdout":
		var formatterFunc dnstap.TextFormatFunc
		switch config.LogFormat {
		case "json":
			formatterFunc = dnstap.JSONFormat
		case "yaml":
			formatterFunc = dnstap.YamlFormat
		case "text":
			formatterFunc = dnstap.TextFormat

		default:
			return nil, fmt.Errorf("%s: is an invalid log format for dnstap stdoutlogger. Valid formats are: text, json, yaml ", config.LogFormat)
		}
		l.dnsTapOutput = dnstap.NewTextOutput(os.Stdout, formatterFunc)
	case "tcp":
		var tcpOutput *dnstap.FrameStreamSockOutput
		if config.Remote == "" {
			return nil, fmt.Errorf("No remote provided for dnstap tcp target. Refusing to start")
		}
		naddr, err := net.ResolveTCPAddr(config.Target, config.Remote)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid TCP address provided to dnstap logger ", config.Remote)
		}
		tcpOutput, _ = dnstap.NewFrameStreamSockOutput(naddr)

		tcpOutput.SetTimeout(time.Duration(config.Timeout) * time.Second)
		tcpOutput.SetFlushTimeout(time.Duration(config.FlushInterval) * time.Second)
		tcpOutput.SetRetryInterval(time.Duration(config.Retry) * time.Second)
		tcpOutput.SetLogger(logger)
		l.dnsTapOutput = tcpOutput
	case "unix":
		var unixSockOutput *dnstap.FrameStreamSockOutput
		if config.Remote == "" {
			return nil, fmt.Errorf("No unix socket provided for dnstap unix socket target. Refusing to start")
		}
		naddr, err := net.ResolveUnixAddr(config.Target, config.Remote)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid unix socket provided to dnstap logger ", config.Remote)
		}
		unixSockOutput, _ = dnstap.NewFrameStreamSockOutput(naddr)
		unixSockOutput.SetTimeout(time.Duration(config.Timeout) * time.Second)
		unixSockOutput.SetFlushTimeout(time.Duration(config.FlushInterval) * time.Second)
		unixSockOutput.SetRetryInterval(time.Duration(config.Retry) * time.Second)
		unixSockOutput.SetLogger(logger)
		l.dnsTapOutput = unixSockOutput
	default:
		return nil, fmt.Errorf("%s: invalid target; valid targets are: stdout, tcp, unix", config.Target)
	}
	return l, nil
}

// StartLoggerOutput starts the dnstap logger output loop
func (l *DNSTapLogger) StartLoggerOutput() {
	go l.dnsTapOutput.RunOutputLoop()
}

// Log is used to log to dnstap.
func (l *DNSTapLogger) Log(state request.Request, r *dns.Msg, _ *dns.EDNS0_SUBNET) {
	// FIXME: implement Bad_query, EDNS_FORMERR, EDNS_BADVERS

	// We only sample non-sonar names
	if localRand.Float64() > l.samplingRate && !isSonar(state) {
		return
	}
	var buf []byte
	m := new(dnstap.Message)
	err := msg.SetQueryAddress(m, state.W.RemoteAddr())
	if err != nil {
		glog.Errorf("Failed to set QueryAddress %v for dnstap message", state.W.RemoteAddr())
	}
	err = msg.SetResponseAddress(m, state.W.LocalAddr())
	if err != nil {
		glog.Errorf("Failed to set ResponseAddress %v for dnstap message", state.W.RemoteAddr())
	}
	msg.SetQueryTime(m, time.Now())
	msg.SetType(m, dnstap.Message_AUTH_QUERY)
	buf, _ = r.Pack()
	m.QueryMessage = buf

	dt := &dnstap.Dnstap{}
	dtType := dnstap.Dnstap_MESSAGE
	dt.Type = &dtType
	dt.Message = m

	pbuf, err := proto.Marshal(dt)
	if err != nil {
		glog.Errorf("Failed to marshal Dnstap message %v", dt)
		return
	}
	output := l.dnsTapOutput.GetOutputChannel()
	select {
	case output <- pbuf:
	default:
		glog.Errorf("Failed to enqueue dnstap message %v for sending, buffer is full", m)
	}
}

// LogFailed is used to log failures
func (l *DNSTapLogger) LogFailed(state request.Request, r *dns.Msg, ecs *dns.EDNS0_SUBNET) {
	m := new(dns.Msg)
	m.SetRcode(r, dns.RcodeServerFailure)
	l.Log(state, m, ecs)
}
