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

package test

import (
	"net"

	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// ResponseWriter extends test.ResponseWriter and record useful calls to methods.
type ResponseWriter struct {
	test.ResponseWriter
	writeMsgCallCount uint64
}

// ResponseWriterCustomRemote is a ResponseWriter which can receive a custom
// remote address.
type ResponseWriterCustomRemote struct {
	ResponseWriter
	RemoteIP string
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *ResponseWriter) WriteMsg(m *dns.Msg) error { t.writeMsgCallCount++; return nil }

// GetWriteMsgCallCount returns the number of WriteMsg calls that were made.
func (t *ResponseWriter) GetWriteMsgCallCount() uint64 { return t.writeMsgCallCount }

// RemoteAddr returns a net.Addr of the remote address connecting to the server.
func (t *ResponseWriterCustomRemote) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP(t.RemoteIP), Port: 40212, Zone: ""}
}
