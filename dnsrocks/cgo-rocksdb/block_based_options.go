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

// BlockBasedOptions is a box for rocksdb_block_based_table_options_t
type BlockBasedOptions struct {
	cBlockBasedOptions *C.rocksdb_block_based_table_options_t
	filterPolicy       *FilterPolicy
	lruCache           *LRUCache
}

// NewBlockBasedOptions creates an instance of BlockBasedOptions
func NewBlockBasedOptions() *BlockBasedOptions {
	cBlockBasedOptions := C.rocksdb_block_based_options_create()
	return &BlockBasedOptions{
		cBlockBasedOptions: cBlockBasedOptions,
	}
}

// SetFullBloomFilter creates Bloom filter of the certain size
func (b *BlockBasedOptions) SetFullBloomFilter(fullBloomBits int) {
	b.filterPolicy = NewFilterPolicyBloomFull(fullBloomBits)
	C.rocksdb_block_based_options_set_filter_policy(b.cBlockBasedOptions, b.filterPolicy.cFilterPolicy)
}

// SetLRUCache creates LRU cache of the given size
func (b *BlockBasedOptions) SetLRUCache(capacity int) {
	b.lruCache = NewLRUCache(capacity)
	C.rocksdb_block_based_options_set_block_cache(b.cBlockBasedOptions, b.lruCache.cCache)
}

// FreeBlockBasedOptions frees up the memory occupied by BlockBasedOptions
func (b *BlockBasedOptions) FreeBlockBasedOptions() {
	if b.lruCache != nil {
		b.lruCache.FreeLRUCache()
	}
	C.rocksdb_block_based_options_destroy(b.cBlockBasedOptions)
	// NOTE: there is no need to explicitly call b.filterPolicy.FreeFilterPolicy(),
	// it seems that rocksdb_block_based_options_destroy() frees the memory
}
