package cdb

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestWriter(t *testing.T) {
	// Create test database.
	databaseName := createDatabaseWithWriter(t)
	defer os.Remove(databaseName)

	// Test reading records
	c, err := Open(databaseName)
	if err != nil {
		t.Fatalf("Error opening database : %s", err)
	}

	defer c.Close()

	context := NewContext()

	_, err = c.Data([]byte("does not exist"), context)
	if err != io.EOF {
		t.Fatalf("nonexistent key should return io.EOF")
	}

	for _, rec := range records {
		// Key with tag.
		key := append([]byte{byte(0)}, []byte(rec.key)...)

		values := rec.values

		v, err := c.Data(key, context)
		if err != nil {
			t.Fatalf("Record read failed: %s", err)
		}

		if !bytes.Equal(v, []byte(values[0])) {
			t.Fatal("Incorrect value returned")
		}

		c.FindStart(context)
		for _, value := range values {
			sr, err := c.FindNext(key, context)
			if err != nil {
				t.Fatalf("Record read failed: %s", err)
			}

			data := sr

			if !bytes.Equal(data, []byte(value)) {
				t.Fatal("value mismatch")
			}
		}
		// Read all values, so should get EOF
		_, err = c.FindNext(key, context)
		if err != io.EOF {
			t.Fatalf("Expected EOF, got %s", err)
		}
	}
}

func createDatabaseWithWriter(t *testing.T) string {
	name := "./test.cdb"

	records := []rec{
		{"one", []string{"1"}},
		{"two", []string{"2", "22"}},
		{"three", []string{"3", "33", "333"}},
	}

	w, err := NewWriter(name)
	if err != nil {
		t.Fatalf("Error creating new Writer : %s", err)
	}

	for _, r := range records {
		for _, v := range r.values {
			tag := 0
			taggedKey := append([]byte{byte(tag)}, []byte(r.key)...)
			err := w.Put(taggedKey, []byte(v))
			if err != nil {
				t.Fatalf("Error inserting db data : %s", err)
			}
		}
	}

	w.Close()

	return name
}
