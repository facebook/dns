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

package dnsserver

import (
	"fmt"

	"github.com/miekg/dns"
)

// TypeToStatsPrefix is the prefix used for creating stats keys
const TypeToStatsPrefix = "DNS_query"

var typeToStats = make(map[uint16]string)

func init() {
	// initialize typeToStats map.
	for k, v := range dns.TypeToString {
		typeToStats[k] = fmt.Sprintf("%s.%s", TypeToStatsPrefix, v)
	}
}

// TypeToStatsKey returns the stats key a query of the given qtype is counted
// under, e.g. "DNS_query.AAAA". Exported so that handlers which answer a query
// without reaching this one can count it under the same key.
func TypeToStatsKey(qtype uint16) string {
	if t, ok := typeToStats[qtype]; ok {
		return t
	}
	return fmt.Sprintf("%s.TYPE%d", TypeToStatsPrefix, qtype)
}
