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
	"fmt"
	"log"

	"github.com/golang/glog"
	"github.com/repustate/go-cdb"
)

func main() {
	dbPath := flag.String("dbpath", "", "Path to cdb")
	dumbSlotStats := flag.Bool("slotstats", false, "Dump information about slots")
	dumbHashStats := flag.Bool("hashstats", false, "Dumps stats about hash. This can be used to see how many entries there is for a specific key.")
	flag.Parse()

	if *dbPath == "" {
		log.Fatalf("-dbpath must be a file: %v", *dbPath)
	}
	c, err := cdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open cdb at %v", *dbPath)
	}
	defer c.Close()

	if *dumbHashStats {
		err := c.ForEachKeys(func(keyHash uint32, key, value []byte) {
			fmt.Printf("KEY %d | %v | %v\n", keyHash, key, value)
		})
		if err != nil {
			glog.Errorf("Failed to dump hash stats %v", err)
		}
	}

	if *dumbSlotStats {
		slotstats := make(map[uint32]int)
		err := c.ForEachKeys(func(keyHash uint32, key, value []byte) {
			slotstats[keyHash]++
		})
		if err != nil {
			glog.Errorf("Failed to dump slot stats %v", err)
		}
		for k, v := range slotstats {
			fmt.Printf("SLOT %d %d\n", k, v)
		}
	}
}
