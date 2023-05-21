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

package main

import (
	"flag"
	"log"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata/cdb"
	"github.com/facebookincubator/dns/dnsrocks/dnsdata/rdb"
)

func main() {
	inputFileName := flag.String("i", "data", "File path to input dns data")
	outputPath := flag.String("o", "", "Output path to write compiled DNS DB")
	useHardlinks := flag.Bool("h", false, "While using RDB builder allows to move files instead of copying during ingestion phase. It is faster, but doesn't work on filesystems that don't support hardlinks")
	rmOld := flag.Bool("rm", false, "Remove all files from output path before compiling")
	numCPU := flag.Int("numcpu", 1, "control parallelism, 0 means all available CPUs")
	batchNum := flag.Int("batchnum", 0, "(RocksDB-only) controls number of parallel RDB batches when not using builder, 0 means unlimited")
	batchSize := flag.Int("batchsize", rdb.DefaultBatchSize, "(RocksDB-only) controls size of batches, 0 means whatever is default.")
	useBuilder := flag.Bool("b", true, "(RocksDB-only) Use RDB builder (fast and furious)")
	useV2Keys := flag.Bool("useV2Keys", false, "(RocksDB-only) Use V2 keys syntax")
	dbDriver := flag.String("dbdriver", "rocksdb", "DB driver (cdb or rocksdb)")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "write memory profile to `file`")
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	switch *dbDriver {
	case "rocksdb":
		// cleanup output directory
		if *rmOld {
			if err := rdb.CleanRDBDir(*outputPath); err != nil {
				log.Fatal(err)
			}
		}
		o := rdb.CompilationOptions{
			BuilderUseHardlinks: *useHardlinks,
			NumCPU:              *numCPU,
			UseBuilder:          *useBuilder,
			BatchNumParallel:    *batchNum,
			BatchSize:           *batchSize,
			UseV2KeySyntax:      *useV2Keys,
		}
		writtenRecs, err := rdb.CompileToRDB(
			*inputFileName, *outputPath, o,
		)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("%d records written", writtenRecs)
	case "cdb":
		if *useHardlinks {
			log.Fatal("Cannot use hardlinks with driver cdb")
		}
		if *rmOld {
			if err := os.RemoveAll(*outputPath); err != nil {
				log.Fatal(err)
			}
		}
		options := &cdb.CreatorOptions{
			NumCPU: *numCPU,
		}
		writtenRecs, err := cdb.CreateCDB(*inputFileName, *outputPath, options)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%d records written", writtenRecs)
	default:
		log.Fatalf("unsupported db driver '%s'", *dbDriver)
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
		f.Close()
	}
}
