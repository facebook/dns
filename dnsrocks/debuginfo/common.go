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

package debuginfo

import (
	"github.com/coredns/coredns/request"

	"github.com/facebookincubator/dns/dnsrocks/db"
	"github.com/facebookincubator/dns/dnsrocks/logger"
)

// Pair represents a key-value pair of debug info.
// It is used instead of a map in order to provide a
// stable output order for metadata.
type Pair struct {
	Key string
	Val string
}

// InfoSrc is defined to enable mocking of [GetInfo].
type InfoSrc func(request.Request) []Pair

func makeInfo(state request.Request) []Pair {
	info := []Pair{
		{Key: "protocol", Val: logger.RequestProtocol(state)},
		{Key: "source", Val: state.RemoteAddr()},
	}
	// don't include destination ip address in the answer if it is unspecified
	if state.LocalIP() != "::" {
		info = append(info, Pair{Key: "destination", Val: state.LocalAddr()})
	}
	if ecs := db.FindECS(state.Req); ecs != nil {
		info = append(info, Pair{Key: "ecs", Val: ecs.String()})
	}
	return info
}
