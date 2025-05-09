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

package nsid

import (
	"context"
	"encoding/hex"

	"github.com/facebook/dns/dnsrocks/debuginfo"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// Handler is a [plugin.Handler] that implements the NSID extension.
type Handler struct {
	infoGen func() debuginfo.InfoSrc
	Next    plugin.Handler
}

// NewHandler produces a new NSID insertion handler.
func NewHandler(privateInfo bool) (*Handler, error) {
	h := new(Handler)
	h.infoGen = debuginfo.Generator(privateInfo)
	return h, nil
}

// ServeDNS implements the [plugin.Handler] interface.
func (h Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if opt := r.IsEdns0(); opt != nil {
		for _, option := range opt.Option {
			if option.Option() == dns.EDNS0NSID {
				w = nsidResponseWriter{ResponseWriter: w, infoSrc: h.infoGen(), request: r}
				break
			}
		}
	}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name implements the [plugin.Handler] interface.
func (h Handler) Name() string { return "nsid" }

type nsidResponseWriter struct {
	dns.ResponseWriter
	infoSrc debuginfo.InfoSrc
	request *dns.Msg
}

// WriteMsg overrides the implementation from w.ResponseWriter.
func (w nsidResponseWriter) WriteMsg(response *dns.Msg) error {
	opt := response.IsEdns0()
	if opt == nil {
		glog.Errorf("no EDNS for NSID")
	} else {
		state := request.Request{W: w, Req: w.request}
		nsid := w.infoSrc.GetInfo(state).Encode()
		opt.Option = append(opt.Option, &dns.EDNS0_NSID{
			Code: dns.EDNS0NSID,
			Nsid: hex.EncodeToString([]byte(nsid)),
		})
	}

	return w.ResponseWriter.WriteMsg(response)
}
