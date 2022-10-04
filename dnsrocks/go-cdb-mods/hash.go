package cdb

import (
	"hash"

	"github.com/dgryski/go-spooky"
)

// New returns a new hash computing the cdb checksum.
func cdbHash() hash.Hash32 {
	d := spooky.New(0, 0)
	return d
}
