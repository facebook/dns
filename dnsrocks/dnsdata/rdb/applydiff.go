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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
	"github.com/facebookincubator/dns/dnsrocks/dnsdata/rdb/dbdiff"
)

func (batch *Batch) ApplyDiff(d *dbdiff.Entry) {
	for _, r := range d.Records {
		switch d.Op {
		case dbdiff.AddOp:
			batch.Add(r.Key, r.Value)
		case dbdiff.DelOp:
			batch.Del(r.Key, r.Value)
		}
	}
}

func (rdb *RDB) ApplyDiff(r io.Reader, serial uint32) error {
	codec := initCodec(serial)
	codec.Features.UseV2Keys = rdb.IsV2KeySyntaxUsed()
	batch := rdb.CreateBatch()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) < 1 || bytes.HasPrefix(line, []byte("#")) {
			continue
		}
		e := new(dbdiff.Entry)
		if err := e.ParseBytes(line); err != nil {
			return fmt.Errorf("parse error for input line '%s': %v", line, err)
		}
		if err := e.Convert(codec); err != nil {
			return fmt.Errorf("conversion error for line '%s' (op '%v'): %v", e.Bytes, e.Op, err)
		}
		batch.ApplyDiff(e)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := rdb.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("database update failed: %s", err)
	}
	return nil
}

// ApplyDiff applies a diff from inputFileName into RDB database at destPath.
func ApplyDiff(diffpath, dbpath string) error {
	rdb, err := NewUpdater(dbpath)
	if err != nil {
		return err
	}
	defer rdb.Close()
	file, err := os.Open(diffpath)
	if err != nil {
		return fmt.Errorf("%s: can't open input: %s", diffpath, err)
	}
	defer file.Close()
	serial, err := dnsdata.DeriveSerial(file)
	if err != nil {
		return fmt.Errorf("%s: can't derive SOA serial: %s", diffpath, err)
	}
	return rdb.ApplyDiff(file, serial)
}
