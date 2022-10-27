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

// #cgo pkg-config: rocksdb
// #include "rocksdb/c.h"
import "C"

// LRUCache is a box for rocksdb_cache_t
type LRUCache struct {
	cCache *C.rocksdb_cache_t
}

// NewLRUCache creates a new instance of LRUCache
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		cCache: C.rocksdb_cache_create_lru(C.size_t(capacity)),
	}
}

// FreeLRUCache destroys an instance of LRUCache
func (c *LRUCache) FreeLRUCache() {
	C.rocksdb_cache_destroy(c.cCache)
}

// GetUsage returns memory usage.
func (c *LRUCache) GetUsage() uint64 {
	return uint64(C.rocksdb_cache_get_usage(c.cCache))
}

// GetPinnedUsage returns pinned memory usage.
func (c *LRUCache) GetPinnedUsage() uint64 {
	return uint64(C.rocksdb_cache_get_pinned_usage(c.cCache))
}
