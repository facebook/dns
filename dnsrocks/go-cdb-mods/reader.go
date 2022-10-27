package cdb

import (
	"io"
	"sync"

	"github.com/golang/glog"
	"github.com/repustate/go-cdb/portablemmap"
)

// Reader is the interface to be used by constant database readers. Its methods
// are the same present in mcdb.Reader so that can also satisfy this interface.
type Reader interface {
	// Preload preloads the database.
	Preload()

	// First returns the first value associated with the given key and tag.
	First(key []byte, tag uint8) ([]byte, bool)

	// Exists returns true if key is found. False otherwise.
	Exists(key []byte, tag uint8) bool

	// Close closes the database.
	Close()
}

// NewReader returns a constant database reader that uses go-cdb as its
// implementation.
func NewReader(fileName string) (Reader, error) {
	c, err := Open(fileName)
	if err != nil {
		return nil, err
	}

	return &reader{
		c,
	}, nil
}

type reader struct {
	c *Cdb
}

// Pool for CDB contexts.
var contextPool = sync.Pool{
	New: func() interface{} {
		return NewContext()
	},
}

func putContext(context *Context) {
	context.loop = 0
	contextPool.Put(context)
}

func (r *reader) Preload() {
	portablemmap.Prefault(r.c.mmappedData)
}

func (r *reader) First(key []byte, tag uint8) ([]byte, bool) {
	taggedKey := append([]byte{byte(tag)}, key...)

	context := contextPool.Get().(*Context)
	defer putContext(context)

	value, err := r.c.Data(taggedKey, context)
	if err != nil {
		if err != io.EOF {
			glog.Errorf("Error looking for key %v (tag %d) : %s",
				string(key), tag, err)
		}

		return nil, false
	}

	return value, true
}

func (r *reader) Exists(key []byte, tag uint8) bool {
	taggedKey := append([]byte{byte(tag)}, key...)

	context := contextPool.Get().(*Context)
	defer putContext(context)

	_, err := r.c.Find(taggedKey, context)
	if err == nil {
		return true
	}

	if err != io.EOF {
		glog.Errorf("Error looking for key %v (tag %d) : %s",
			string(key), tag, err)
	}

	return false
}

func (r *reader) Close() {
	r.c.Close()
}
