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
	"runtime"
	"time"

	rocksdb "github.com/facebook/dns/dnsrocks/cgo-rocksdb"
	"github.com/facebook/dns/dnsrocks/dnsdata/rdb"
)

// compact runs manual compaction on whole DB
func compact(dbpath string) error {
	options := rdb.DefaultOptions()
	options.SetParallelism(runtime.NumCPU())
	options.OptimizeLevelStyleCompaction(0)
	// we don't need to keep old logs or old files
	options.SetDeleteObsoleteFilesPeriodMicros(0)
	options.SetKeepLogFileNum(1)
	db, err := rocksdb.OpenDatabase(dbpath, false, false, options)
	if err != nil {
		return err
	}
	defer db.CloseDatabase()
	db.CompactRangeAll()
	return nil
}

func main() {
	dbDir := flag.String("db", "", "Path to RocksDB directory")
	flag.Parse()
	if *dbDir == "" {
		log.Fatal("Need to specify db path")
	}
	log.Printf("Running manual compaction on %q", *dbDir)
	startTime := time.Now()
	if err := compact(*dbDir); err != nil {
		log.Fatalf("Failed to compact %q: %v", *dbDir, err)
	}
	log.Printf("Done in %dms", int64(time.Since(startTime)/time.Millisecond))
}
