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

package fbserver

import (
	"context"

	"github.com/facebook/dns/dnsrocks/dnsserver"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type cnameChasingHandler struct {
	Next plugin.Handler
}

// newCNAMEChasingHandler initializes a new cnameChasingHandler.
// This handler is used to control whether CNAME chasing is
// enabled/disabled in the server.
func newCNAMEChasingHandler() (*cnameChasingHandler, error) {
	ch := new(cnameChasingHandler)
	return ch, nil
}

func (ch *cnameChasingHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	ctx = dnsserver.WithCNAMEChasing(ctx, true)
	return plugin.NextOrFailure(ch.Name(), ch.Next, ctx, w, r)
}

func (ch *cnameChasingHandler) Name() string { return "cnameChasing" }
