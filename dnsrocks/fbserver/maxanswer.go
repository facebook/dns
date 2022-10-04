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
	"fmt"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type maxAnswerHandler struct {
	maxAnswer int
	Next      plugin.Handler
}

// NewWhoamI initialize a new maxAnswerHandler.
// Currently only source the cluster name on initialization.
func newMaxAnswerHandler(i int) (*maxAnswerHandler, error) {
	if i <= 0 {
		return nil, fmt.Errorf("maxAnswer must be > 0. Got %d", i)
	}
	mh := new(maxAnswerHandler)
	mh.maxAnswer = i
	return mh, nil
}

func (mh *maxAnswerHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	ctx = dnsserver.WithMaxAnswer(ctx, mh.maxAnswer)
	return plugin.NextOrFailure(mh.Name(), mh.Next, ctx, w, r)
}

func (mh *maxAnswerHandler) Name() string { return "maxAnswer" }
