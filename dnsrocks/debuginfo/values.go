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
	"strings"

	"github.com/coredns/coredns/request"
)

// Pair represents a key-value pair of debug info.
// It is used instead of a map in order to provide a
// stable output order for metadata.
type Pair struct {
	Key string
	Val string
}

// Values is inspired by [url.Values], but is simplified for this use case:
//   - Percent-encoding is not used.  This keeps IPv6 addresses human-readable
//     and reduces response bloat.
//   - Keys with empty values are omitted from the encoded output.
//   - Stable-order iteration is available independent of the syntax, to support
//     TXT record generation, but sorting is avoided.  This makes lookups slow,
//     but the lookup methods are only used in the tests.
type Values []Pair

// Add appends a key-val pair to the Values.
func (v *Values) Add(key string, val string) {
	*v = append(*v, Pair{Key: key, Val: val})
}

// Encode returns the values encoded in key=value style (similar to DNS-SD).
func (v *Values) Encode() string {
	var components []string
	for _, pair := range *v {
		if len(pair.Val) > 0 {
			components = append(components, fmt.Sprintf("%s=%s", pair.Key, pair.Val))
		}
	}
	return strings.Join(components, " ")
}

// Has returns true if at least one value exists for this key.
func (v *Values) Has(key string) bool {
	for _, pair := range *v {
		if pair.Key == key {
			return true
		}
	}
	return false
}

// Get returns the first value associated with this key.
func (v *Values) Get(key string) string {
	for _, pair := range *v {
		if pair.Key == key {
			return pair.Val
		}
	}
	return ""
}

// MockInfoSrc is a mock [InfoSrc] used for testing.
type MockInfoSrc Values

// GetInfo implements the [InfoSrc] interface.
func (s *MockInfoSrc) GetInfo(request.Request) *Values {
	v := Values(*s)
	return &v
}
