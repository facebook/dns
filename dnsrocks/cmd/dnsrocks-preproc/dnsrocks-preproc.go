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

	"github.com/facebook/dns/dnsrocks/dnsdata"
)

func main() {
	serial := flag.Int("serial", 0, "optional SOA serial")
	flag.Parse()
	codec := new(dnsdata.Codec)
	codec.Acc.Ranger.Enable()
	codec.Acc.NoPrefixSets = true
	codec.NoRnetOutput = true
	if *serial > 0 {
		codec.Serial = uint32(*serial)
	}

	if err := codec.Preprocess(os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
