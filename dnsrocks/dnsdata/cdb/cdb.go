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

package cdb

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/facebook/dns/dnsrocks/dnsdata"

	cdb "github.com/repustate/go-cdb"
	"golang.org/x/sync/errgroup"
)

// CreatorOptions provides options to create CDB
type CreatorOptions struct {
	NumCPU int
}

// NewDefaultCreatorOptions gives default options
func NewDefaultCreatorOptions() *CreatorOptions {
	return &CreatorOptions{
		NumCPU: runtime.NumCPU(),
	}
}

// CreateCDB compiles CDB with native Go compiler
func CreateCDB(ipath string, opath string, options *CreatorOptions) (mw int, err error) {
	if options == nil {
		options = NewDefaultCreatorOptions()
	}
	// Open infile for read
	ifile, err := os.Open(ipath)
	if err != nil {
		return 0, fmt.Errorf("can't open input file: %w", err)
	}
	defer ifile.Close()
	serial, err := dnsdata.DeriveSerial(ifile)
	if err != nil {
		return 0, fmt.Errorf("can't stat input file: %w", err)
	}

	// cleanup partially written db in case of failure
	defer func() {
		if err != nil {
			os.Remove(opath)
		}
	}()

	db, err := cdb.NewWriter(opath)
	if err != nil {
		return 0, fmt.Errorf("can't create output database: %w", err)
	}
	defer db.Close()

	return CreateCDBFromReader(ifile, db, serial, options.NumCPU)
}

// CreateCDBFromReader compiles CDB with native Go compiler, reading data from io.ReadCloser
func CreateCDBFromReader(r io.Reader, db cdb.Writer, serial uint32, workers int) (nw int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic while writing CDB: %v", r)
		}
	}()

	// Initialize the codec
	codec := new(dnsdata.Codec)
	codec.Serial = serial

	// will be closed by ParseParallelStream
	resultsChan := make(chan []dnsdata.MapRecord, workers)

	var g errgroup.Group
	g.Go(func() error {
		return dnsdata.ParseStream(r, codec, resultsChan, workers)
	})
	nw = 0
	for v := range resultsChan {
		for _, m := range v {
			err := db.Put(m.Key, m.Value)
			if err != nil {
				return nw, err
			}
			nw++
		}
	}
	if err := g.Wait(); err != nil {
		return nw, fmt.Errorf("can't create output database: %w", err)
	}
	return nw, nil
}
