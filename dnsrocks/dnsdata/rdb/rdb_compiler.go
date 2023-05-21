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
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"golang.org/x/sync/errgroup"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
)

// CleanRDBDir removes all files from the directory - for instance, to clean up output directory before compilation
func CleanRDBDir(rdbDir string) error {
	dir, err := os.Open(rdbDir)
	if err != nil {
		return fmt.Errorf("error opening directory %s: %w", rdbDir, err)
	}
	var files []string
	files, err = dir.Readdirnames(0)
	if err != nil {
		return fmt.Errorf("error reading directory %s: %w", rdbDir, err)
	}
	for _, fileName := range files {
		fullName := path.Join(rdbDir, fileName)
		if err = os.Remove(fullName); err != nil {
			return fmt.Errorf("error removing file %s: %w", fullName, err)
		}
	}
	return nil
}

// CompilationOptions allows us to provide options to RDB compiler
type CompilationOptions struct {
	NumCPU         int  // Parser and builder parallelism
	UseV2KeySyntax bool // specifies whether v2 keys syntax should be used
	// builder-related settings
	UseBuilder          bool // if we use RDB builder (mem hungry, fastest) or not
	BuilderUseHardlinks bool // if RDB builder can use hardlinks instead of copying sst files
	// batch-related settings
	BatchNumParallel int // When not using builder, how many batches can we backlog while parsing, affects mem consumption
	BatchSize        int // When not using builder, ize of RDB batches
}

func compileBuilder(in io.Reader, codec *dnsdata.Codec, destPath string, opts CompilationOptions) (int, error) {
	var builder *Builder
	var err error
	var g errgroup.Group
	// Open or create database
	if builder, err = NewBuilder(destPath, opts.BuilderUseHardlinks); err != nil {
		return 0, fmt.Errorf("error opening database at %s: %w", destPath, err)
	}
	defer builder.FreeBuilder()

	log.Println("Reading ...")
	// Scan
	counter := 0
	nw := 0

	store := func(data []dnsdata.MapRecord) {
		for _, m := range data {
			builder.ScheduleAdd(m.Key, m.Value)
			nw++
			counter++
			if counter == DefaultBatchSize {
				counter = 0
				log.Println(nw)
			}
		}
	}

	// will be closed by ParseParallelStream
	resultsChan := make(chan []dnsdata.MapRecord, opts.NumCPU)

	g.Go(func() error {
		return dnsdata.ParseStream(in, codec, resultsChan, opts.NumCPU)
	})
	for v := range resultsChan {
		store(v)
	}

	// final flush
	if err = builder.Execute(); err != nil {
		return nw, fmt.Errorf("building database failed: %w", err)
	}

	return nw, g.Wait()
}

func compileBatches(in io.Reader, codec *dnsdata.Codec, destPath string, opts CompilationOptions) (int, error) {
	var db *RDB
	var err error
	var g errgroup.Group
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	limiter := make(chan struct{}, opts.BatchNumParallel)
	defer close(limiter)

	db, err = NewRDB(destPath)
	if err != nil {
		return 0, fmt.Errorf("error opening database at %s: %w", destPath, err)
	}
	defer func() {
		if err != nil {
			// run compaction if no errors happened
			db.db.CompactRangeAll()
		}
		if ierr := db.Close(); ierr != nil {
			log.Printf("error closing database: %v", err)
			if err == nil {
				// report closing error if no other errors happened
				err = ierr
			}
		}
	}()
	rdbBatch := db.CreateBatch()

	log.Println("Reading ...")
	// Scan
	counter := 0
	nw := 0

	store := func(data []dnsdata.MapRecord) {
		for _, m := range data {
			rdbBatch.Add(m.Key, m.Value)

			nw++
			counter++
			if counter == batchSize {
				counter = 0
				log.Println(nw)
				b := rdbBatch
				limiter <- struct{}{}
				g.Go(func() error {
					if err := db.ExecuteBatch(b); err != nil {
						<-limiter
						return fmt.Errorf("error executing batch: %w", err)
					}
					<-limiter
					return nil
				})
				rdbBatch = db.CreateBatch()
			}
		}
	}

	// will be closed by ParseParallelStream
	resultsChan := make(chan []dnsdata.MapRecord, opts.NumCPU)

	g.Go(func() error {
		return dnsdata.ParseStream(in, codec, resultsChan, opts.NumCPU)
	})
	for v := range resultsChan {
		store(v)
	}

	// final flush
	if !rdbBatch.IsEmpty() {
		log.Println("Flushing batch")
		if err := db.ExecuteBatch(rdbBatch); err != nil {
			return nw, fmt.Errorf("error executing batch: %w", err)
		}
	}

	return nw, g.Wait()
}

// Compile data from io.Reader into RDB database at destPath.
// useHardlinks allows to use hardlinks in Builder mode. Not supported by fbcode filesystem.
func Compile(in io.Reader, serial uint32, destPath string, opts CompilationOptions) (int, error) {
	codec := initCodec(serial)
	codec.Features.UseV2Keys = opts.UseV2KeySyntax

	if opts.UseBuilder {
		return compileBuilder(in, codec, destPath, opts)
	}
	return compileBatches(in, codec, destPath, opts)
}

// CompileToRDB compiles inputFileName into RDB database at destPath.
// useHardlinks allows to use hardlinks in Builder mode. Not supported by fbcode filesystem.
func CompileToRDB(inputFileName, destPath string, o CompilationOptions) (int, error) {
	return CompileToSpecificRDBVersion(inputFileName, destPath, o)
}

// CompileToSpecificRDBVersion compiles inputFileName into RDB database at destPath and
// useHardlinks allows to use hardlinks in Builder mode. Not supported by fbcode filesystem.
// useV2KeySyntax specifies whether v2 keys syntax should be used
func CompileToSpecificRDBVersion(inputFileName, destPath string, o CompilationOptions) (int, error) {
	// Open infile for read
	ifile, err := os.Open(inputFileName)
	if err != nil {
		return 0, fmt.Errorf("error opening input file %s: %w", inputFileName, err)
	}
	defer ifile.Close()
	serial, err := dnsdata.DeriveSerial(ifile)
	if err != nil {
		return 0, fmt.Errorf("error accessing input file %s: %w", inputFileName, err)
	}
	return Compile(ifile, serial, destPath, o)
}

func initCodec(serial uint32) *dnsdata.Codec {
	codec := new(dnsdata.Codec)
	codec.Serial = serial
	codec.Acc.Ranger.Enable()
	codec.Acc.NoPrefixSets = true
	codec.NoRnetOutput = true
	return codec
}
