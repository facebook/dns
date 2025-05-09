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

package debuginfo

import (
	"fmt"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/facebook/dns/dnsrocks/db"
	"github.com/facebook/dns/dnsrocks/logger"
)

// InfoSrc is defined to enable mocking of [GetInfo].
type InfoSrc interface {
	GetInfo(request.Request) *Values
}

type infoSrc struct {
	created time.Time
}

// makeInfoSrc creates an InfoSrc that captures the current creation time.
func makeInfoSrc() InfoSrc {
	return infoSrc{created: time.Now()}
}

// GetInfo returns the debug info related to this request.
func (i infoSrc) GetInfo(state request.Request) *Values {
	info := new(Values)
	info.Add("time", fmt.Sprintf("%.3f", float64(i.created.UnixMilli())/1000.))
	info.Add("protocol", logger.RequestProtocol(state))
	info.Add("source", state.RemoteAddr())
	// don't include destination ip address in the answer if it is unspecified
	if state.LocalIP() != "::" {
		info.Add("destination", state.LocalAddr())
	}
	if ecs := db.FindECS(state.Req); ecs != nil {
		info.Add("ecs", ecs.String())
	}
	return info
}
