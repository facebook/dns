/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rdb

import (
	"os"
	"testing"

	"github.com/facebook/dns/dnsrocks/dnsdata/rdb"
	"github.com/facebook/dns/dnsrocks/testaid"
)

func TestMain(m *testing.M) {
	os.Exit(testaid.Run(m, "../../testdata/data"))
}

func TestApplyDiff0(t *testing.T) {
	testrdb := testaid.TestRDB
	if err := rdb.ApplyDiff("/dev/null", testrdb.Path); err != nil {
		t.Fatal(err)
	}
}

var diff1 = `
-&example.org,,b.ns.example.org,172800,,
-+b.ns.example.org,5.5.6.5,172800,,
+&example.org,7.8.8.1,c.ns.example.org,172800,,
++abc.new,7.8.8.1,7200,,
-+kill.me,1.2.3.4
-+patch.me,1.1.1.1
`

// search key for one of the added records above
var lookKey1 = []byte("\000\000\003abc\003new\000")
var lookKey1v2 = []byte("\x00o\003new\003abc\000\000\000")

// expected length of the value added for the above key
var nvalue1 = 23

// search key for one of the deleted records above
var lookKey2 = []byte("\000\000\003kill\002me\000")
var lookKey2v2 = []byte("\x00o\002me\003kill\000\000\000")

// search key for one of the deleted records above
var lookKey3 = []byte("\000\000\005patch\002me\000")
var lookKey3v2 = []byte("\x00o\002me\005patch\000\000\000")

// expected length of the remaining value for the above key
var nvalue3 = 35

func TestApplyDiff(t *testing.T) {
	testApplyDiff(t, testaid.TestRDB, lookKey1, lookKey2, lookKey3)
}

func TestApplyDiffV2(t *testing.T) {
	testApplyDiff(t, testaid.TestRDBV2, lookKey1v2, lookKey2v2, lookKey3v2)
}

func testApplyDiff(t *testing.T, baseDB testaid.TestDB, key1, key2, key3 []byte) {
	testdiff, err := os.CreateTemp("", "testdiff")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testdiff.Name())
	if _, err := testdiff.Write([]byte(diff1)); err != nil {
		t.Fatal(err)
	}
	testdiff.Close()

	testdb := baseDB
	if err := rdb.ApplyDiff(testdiff.Name(), testdb.Path); err != nil {
		t.Fatal(err)
	}

	db, err := rdb.NewReader(testdb.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CatchWithPrimary(); err != nil {
		t.Fatal(err)
	}

	t.Run("Additions", func(t *testing.T) {
		search := rdb.NewContext()

		value1, err := db.Find(key1, search)
		if err != nil {
			t.Fatal(err)
		}
		if len(value1) != nvalue1 {
			t.Errorf("expected len %v, got %v", nvalue1, len(value1))
		}
	})

	t.Run("Deletions/1/kill.me", func(t *testing.T) {
		search := rdb.NewContext()

		value2, err := db.Find(key2, search)
		if err == nil {
			t.Errorf("expected error, got result of len %d", len(value2))
		}
	})

	t.Run("Deletions/2/patch.me", func(t *testing.T) {
		search := rdb.NewContext()
		values := make([][]byte, 0, 2)

		err := db.ForEach(
			key3,
			func(value []byte) error {
				values = append(values, value)
				return nil
			},
			search)
		if err != nil {
			t.Fatal(err)
		}

		if len(values) != 1 {
			t.Errorf("expected single value, %d values found", len(values))
		}

		value3 := values[0]
		if len(value3) != nvalue3 {
			t.Errorf("expected len %v, got %v", nvalue3, len(value3))
		}
	})
}
