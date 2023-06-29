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

package whoami

import (
	"context"
	"fmt"
	"strings"

	"github.com/facebookincubator/dns/dnsrocks/debuginfo"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Handler is the base struct
// representing the Handler
type Handler struct {
	whoamiDomain string
	getInfo      debuginfo.InfoSrc
	Next         plugin.Handler
}

// NewWhoami initializes a new whoami Handler.
func NewWhoami(d string) (*Handler, error) {
	wh := new(Handler)
	wh.getInfo = debuginfo.GetInfo
	wh.whoamiDomain = strings.ToLower(dns.Fqdn(d))
	return wh, nil
}

// ServeDNS serves whoami queries
func (wh *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question[0].Name) != len(wh.whoamiDomain) || strings.ToLower(r.Question[0].Name) != wh.whoamiDomain {
		return plugin.NextOrFailure(wh.Name(), wh.Next, ctx, w, r)
	}
	state := request.Request{W: w, Req: r}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = true
	m.Authoritative = true
	if state.QType() == dns.TypeTXT {
		mkTxt := func(key, value string) dns.RR {
			var rr dns.RR = new(dns.TXT)
			rr.(*dns.TXT).Hdr = dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: state.QClass()}
			rr.(*dns.TXT).Txt = []string{fmt.Sprintf("%s %s", key, value)}
			return rr
		}
		for _, pair := range wh.getInfo(state) {
			m.Answer = append(m.Answer, mkTxt(pair.Key, pair.Val))
		}
	}

	state.SizeAndDo(m)
	m = state.Scrub(m)
	err := state.W.WriteMsg(m)
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

// Name returns the handlers name
func (wh *Handler) Name() string { return "whoami" }
