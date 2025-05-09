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

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type anyHandler struct {
	Next plugin.Handler
}

// NewAnyHandler initialize a new anyHandler.
// This handler is setup to refuse ANY dns queries by following RFC 8482
// answering with a Synthesized HINFO RRset setting CPU as RFC8482 and OS as null string
func newAnyHandler() (*anyHandler, error) {
	ah := new(anyHandler)
	return ah, nil
}

func (ah *anyHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if r.Question[0].Qtype != dns.TypeANY {
		return plugin.NextOrFailure(ah.Name(), ah.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	// Setting response TTL to one day
	hdr := dns.RR_Header{Name: r.Question[0].Name, Ttl: 86400, Class: dns.ClassINET, Rrtype: dns.TypeHINFO}
	// Setting response: HINFO	"RFC 8482" ""
	m.Answer = []dns.RR{&dns.HINFO{Hdr: hdr, Cpu: "RFC 8482", Os: ""}}

	err := w.WriteMsg(m)
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

func (ah *anyHandler) Name() string { return "any" }
