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

package rdb

import (
	"sync"

	rocksdb "github.com/facebookincubator/dns/dnsrocks/cgo-rocksdb"
)

// NumberOfIterators is empirically found number of iterators which make sense to keep in pool
const NumberOfIterators int = 15

// IteratorPool allows RDB iterators reuse. Iterator creation is happen to be pretty costly operation
type IteratorPool struct {
	iterators      chan iteratorPoolEntry
	enabled        bool
	createIterator func() *rocksdb.Iterator
	l              sync.Mutex
}

type iteratorPoolEntry struct {
	iterator *rocksdb.Iterator
	free     bool // if true - iterator is not taken from pool and should be destroyed on release
}

func newIteratorPool(createIterator func() *rocksdb.Iterator) *IteratorPool {
	pool := new(IteratorPool)
	pool.iterators = make(chan iteratorPoolEntry, NumberOfIterators)
	pool.createIterator = createIterator

	return pool
}

func (pool *IteratorPool) get() iteratorPoolEntry {
	if !pool.enabled {
		return iteratorPoolEntry{iterator: pool.createIterator(), free: true}
	}

	return <-pool.iterators
}

func (pool *IteratorPool) put(e iteratorPoolEntry) {
	if e.free {
		e.iterator.FreeIterator()
	} else {
		pool.iterators <- e
	}
}

// disable call will be blocked until all previously allocated iterators are released (by calling put method)
// client still can call get() method but all newly created iterators will be ephemeral and not persisted in pool
func (pool *IteratorPool) disable() {
	pool.l.Lock()
	defer pool.l.Unlock()
	if !pool.enabled {
		return
	}

	pool.enabled = false

	for i := 0; i < NumberOfIterators; i++ {
		e := <-pool.iterators
		e.iterator.FreeIterator()
	}
}

func (pool *IteratorPool) enable() {
	pool.l.Lock()
	defer pool.l.Unlock()

	if pool.enabled {
		return
	}

	for i := 0; i < NumberOfIterators; i++ {
		entry := iteratorPoolEntry{iterator: pool.createIterator()}

		pool.iterators <- entry
	}

	pool.enabled = true
}
