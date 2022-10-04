package cdb

import (
	"bytes"
	"os"
	"testing"
)

func TestReader(t *testing.T) {
	// Create test database.
	databaseName := createDatabaseWithWriter(t)
	defer os.Remove(databaseName)

	r, err := NewReader(databaseName)
	if err != nil {
		t.Fatalf("Failed opening database %q : %s", databaseName, err)
	}

	defer r.Close()

	ok := r.Exists([]byte("one"), 0)
	if !ok {
		t.Errorf("Expected to find key 'one' in tag 0 but did not.")
	}

	ok = r.Exists([]byte("one"), 1)
	if ok {
		t.Errorf("Expected to not find key 'one' in tag 1 but did.")
	}

	v, ok := r.First([]byte("one"), 0)
	if !ok {
		t.Errorf("Expected to find key 'one' in tag 0 but did not.")
	}

	if bytes.Compare(v, []byte("1")) != 0 {
		t.Errorf("Expected value []byte{1} but got %#v.", v)
	}
}
