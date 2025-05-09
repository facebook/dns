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

package main

import (
	"flag"
	"log"
	"os"

	"github.com/facebook/dns/dnsrocks/dnsdata/rdb"
)

func main() {
	inputFileName := flag.String("i", "", "File path to input dns data diff")
	serial := flag.Uint("serial", 0, "Value for the Serial field of the changed SOA records")
	outputDirPath := flag.String("o", "", "Output directory path to write compiled DNS DB")
	flag.Parse()

	if *inputFileName != "" {
		if err := rdb.ApplyDiff(*inputFileName, *outputDirPath); err != nil {
			log.Fatal(err)
		}
	} else {
		if *serial == 0 {
			log.Fatal("Need to specify serial")
		}
		db, err := rdb.NewUpdater(*outputDirPath)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		if err := db.ApplyDiff(os.Stdin, uint32(*serial)); err != nil {
			log.Fatal(err)
		}
	}
}
