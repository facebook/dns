// Package cdb reads and writes cdb ("constant database") files.
//
// See the original cdb specification and C implementation by D. J. Bernstein
// at http://cr.yp.to/cdb.html.
package cdb

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"

	spooky "github.com/dgryski/go-spooky"
	"github.com/repustate/go-cdb/portablemmap"
)

const (
	headerSize = uint32(256 * 8)
)

type Cdb struct {
	// Slice backed by the mmapped file.
	mmappedData []byte
}

type Context struct {
	loop   uint32 // number of hash slots searched under this key
	khash  uint32 // initialized if loop is nonzero
	kpos   uint32 // initialized if loop is nonzero
	hpos   uint32 // initialized if loop is nonzero
	hslots uint32 // initialized if loop is nonzero
	dpos   uint32 // initialized if FindNext() returns true
	dlen   uint32 // initialized if FindNext() returns true
}

func newWithFile(f *os.File) (*Cdb, error) {
	mmappedData, err := portablemmap.Mmap(f)
	if err != nil {
		return nil, err
	}

	return &Cdb{
		mmappedData,
	}, nil
}

// Open opens the named file read-only and returns a new Cdb object.  The file
// should exist and be a cdb-format database file.
func Open(name string) (*Cdb, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	// We do not need to keep the file opened after it is mmapped.
	defer f.Close()

	return newWithFile(f)
}

// Close closes the cdb for any further reads.
func (c *Cdb) Close() (err error) {
	// Unmap data.
	return portablemmap.Munmap(c.mmappedData)
}

// New creates a new Cdb from the given ReaderAt, which should be a cdb format
// database.
func New(r io.ReaderAt) *Cdb {
	c, _ := newWithFile(r.(*os.File))

	return c
}

// NewContext returns a new context to be used in CDB calls.
func NewContext() *Context {
	// Zero values for the context are ok, so no need to set them
	// explicitly here.
	return &Context{}
}

// Reset resets a context
func (c *Context) Reset() {
	c.loop = 0
}

// Data returns the first data value for the given key.
// If no such record exists, it returns EOF.
func (c *Cdb) Data(key []byte, context *Context) ([]byte, error) {
	c.FindStart(context)
	if err := c.find(key, context); err != nil {
		return nil, err
	}

	data := c.mmappedData[context.dpos : context.dpos+context.dlen]

	return data, nil
}

// FindStart resets the cdb to search for the first record under a new key.
func (c *Cdb) FindStart(context *Context) { context.loop = 0 }

// FindNext returns the next data value for the given key as a byte slice.
// If there are no more records for the given key, it returns EOF.
// FindNext acts as an iterator: The iteration should be initialized by calling
// FindStart and all subsequent calls to FindNext should use the same key value.
func (c *Cdb) FindNext(key []byte, context *Context) ([]byte, error) {
	if err := c.find(key, context); err != nil {
		return nil, err
	}

	return c.mmappedData[context.dpos : context.dpos+context.dlen], nil
}

// Find returns the first data value for the given key as a byte slice.
// Find is the same as FindStart followed by FindNext.
func (c *Cdb) Find(key []byte, context *Context) ([]byte, error) {
	c.FindStart(context)

	return c.FindNext(key, context)
}

func (c *Cdb) find(key []byte, context *Context) (err error) {
	/*
		// This block takes a fair amount of resources. Commenting it out to save
		// about 20% of resource. Panic must be handled at a higher level.
		defer func() {
			if e := recover(); e != nil {
				err = e.(error)
			}
		}()
	*/
	var pos, h uint32

	klen := uint32(len(key))
	if context.loop == 0 {
		h = spooky.Hash32(key)
		context.hpos, context.hslots = c.readNums((h<<3)&2047,
			context)
		if context.hslots == 0 {
			return io.EOF
		}
		context.khash = h
		h >>= 8
		h %= context.hslots
		h <<= 3
		context.kpos = context.hpos + h
	}

	for context.loop < context.hslots {
		h, pos = c.readNums(context.kpos, context)
		if pos == 0 {
			return io.EOF
		}
		context.loop++
		context.kpos += 8
		if context.kpos == context.hpos+(context.hslots<<3) {
			context.kpos = context.hpos
		}
		if h == context.khash {
			rklen, rdlen := c.readNums(pos, context)
			if rklen == klen {
				if c.match(key, pos+8) {
					context.dlen = rdlen
					context.dpos = pos + 8 + klen
					return nil
				}
			}
		}
	}

	return io.EOF
}

func (c *Cdb) match(key []byte, pos uint32) bool {
	return bytes.Equal(c.mmappedData[pos:pos+uint32(len(key))], key)
}

func (c *Cdb) readNums(pos uint32, context *Context) (uint32, uint32) {
	data := c.mmappedData[pos : pos+8]

	return binary.LittleEndian.Uint32(data),
		binary.LittleEndian.Uint32(data[4:])
}

// ForEachKeys will call a function with the key hash as well as key and value.
func (c *Cdb) ForEachKeys(f func(keyHash uint32, key, value []byte)) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	var context = NewContext()

	for i := 0; i < 256; i++ {
		hpos, hslots := c.readNums(uint32(i)<<3, context)

		for s := 0; uint32(s) < hslots; s++ {
			hval, rpos := c.readNums(hpos+uint32(s)<<3, context)
			if rpos != 0 {
				klen, vlen := c.readNums(rpos, context)
				x := rpos + 8
				kval := c.mmappedData[x : x+klen]
				x += klen
				vval := c.mmappedData[x : x+vlen]
				f(hval, kval, vval)
			}
		}

	}
	return
}
