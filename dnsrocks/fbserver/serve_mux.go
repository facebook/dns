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

	"github.com/coredns/coredns/plugin"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// serveMux performs some validation checking and then starts serving the DNS
// request through the plugin handler chain.
type serveMux struct {
	defaultHandler plugin.Handler
}

func (mux *serveMux) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) < 1 {
		dns.HandleFailed(w, req)
		return
	}
	ctx := context.TODO()
	_, err := mux.defaultHandler.ServeDNS(ctx, w, req)
	if err != nil {
		glog.Errorf("%v", err)
	}
}
