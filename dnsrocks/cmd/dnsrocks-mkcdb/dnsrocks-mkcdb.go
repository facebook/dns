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
	"runtime"
	"runtime/pprof"

	"github.com/facebook/dns/dnsrocks/dnsdata/cdb"

	"flag"
	"log"
	"os"
)

func main() {
	ipath := flag.String("i", "data", "File path to input dns data")
	opath := flag.String("o", "data.cdb", "Output path to write the dns DB")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "write memory profile to `file`")
	numCPU := flag.Int("numcpu", 1, "number of CPUs to use for parsing in parallel, 0 means all")
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

	options := &cdb.CreatorOptions{
		NumCPU: *numCPU,
	}
	nw, err := cdb.CreateCDB(*ipath, *opath, options)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("%d records written", nw)
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
