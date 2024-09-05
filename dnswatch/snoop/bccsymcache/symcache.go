/*
Copyright (c) Facebook, Inc. and its affiliates.
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

package bccsymcache

/*
#include <bcc/bcc_common.h>
#include <bcc/bcc_syms.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Symbol represents a symbol in a binary.
type Symbol struct {
	Name         string
	DemangleName string
	Module       string
	Offset       uint64
}

// Cache is a cache of symbols.
type Cache struct {
	pid   int
	cache unsafe.Pointer
}

// New creates a new cache for the given process ID.
func New(pid int) *Cache {
	pidC := C.int(pid)
	cache := C.bcc_symcache_new(pidC, nil)
	return &Cache{pid: pid, cache: cache}
}

// Free frees the cache.
func (c *Cache) Free() {
	C.bcc_free_symcache(c.cache, C.int(c.pid))
}

// ResolveAddr resolves an address to a symbol.
func (c *Cache) ResolveAddr(addr uint64) (*Symbol, error) {
	sym := &C.struct_bcc_symbol{}
	symC := (*C.struct_bcc_symbol)(unsafe.Pointer(sym))
	addrC := C.uint64_t(addr)
	res := C.bcc_symcache_resolve(c.cache, addrC, symC)
	if res < 0 {
		return nil, fmt.Errorf("unable to locate addr %d for pid %d: %d", addr, c.pid, res)
	}

	return &Symbol{
		Name:         C.GoString(symC.name),
		DemangleName: C.GoString(symC.demangle_name),
		Module:       C.GoString(symC.module),
		Offset:       uint64(symC.offset),
	}, nil
}
