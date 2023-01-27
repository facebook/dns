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

package rocksdb

/*
// @fb-only: #include "rocksdb/src/include/rocksdb/c.h"
#cgo pkg-config: "rocksdb"
#include "rocksdb/c.h" // @oss-only
*/
import "C"

// FilterPolicy is a box for policy filter. Bloom Filters and policy filters: https://github.com/facebook/rocksdb/wiki/RocksDB-Bloom-Filter
type FilterPolicy struct {
	cFilterPolicy *C.rocksdb_filterpolicy_t
}

// NewFilterPolicyBloom creates a new filter policy with block-based Bloom Filter
func NewFilterPolicyBloom(bitsPerKey int) *FilterPolicy {
	return &FilterPolicy{
		cFilterPolicy: C.rocksdb_filterpolicy_create_bloom(C.double(bitsPerKey)),
	}
}

// NewFilterPolicyBloomFull creates a new filter policy with new Bloom Filter
func NewFilterPolicyBloomFull(bitsPerKey int) *FilterPolicy {
	return &FilterPolicy{
		cFilterPolicy: C.rocksdb_filterpolicy_create_bloom_full(C.double(bitsPerKey)),
	}
}

// FreeFilterPolicy frees up memory for FilterPolicy
func (p *FilterPolicy) FreeFilterPolicy() {
	C.rocksdb_filterpolicy_destroy(p.cFilterPolicy)
}
