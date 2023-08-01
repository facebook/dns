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
	"os"

	"github.com/miekg/dns"

	"github.com/facebook/dns/dnsrocks/dnsserver"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
)

func main() {
	var (
		maxans        int
		err           error
		tdb           *dnsserver.FBDNSDB
		cacheConfig   dnsserver.CacheConfig
		dbConfig      dnsserver.DBConfig
		handlerConfig dnsserver.HandlerConfig
	)
	flag.StringVar(&dbConfig.Path, "dbpath", "", "Path to CDB")
	flag.StringVar(&dbConfig.Driver, "dbdriver", "cdb", "DB driver")
	flag.IntVar(&maxans, "maxans", 1, "Max number of answer server should return.")
	qType := flag.String("qtype", "A", "Type of the query")
	qName := flag.String("qname", "", "Name to query")
	resolver := flag.String("resolver", "127.0.0.1", "IP of the resolver to simulate the query from.")
	subnet := flag.String("subnet", "", "client subnet")
	flag.Parse()

	if tdb, err = dnsserver.NewFBDNSDB(handlerConfig, dbConfig, cacheConfig, &dnsserver.TextLogger{IoWriter: os.Stdout}, &stats.DummyStats{}); err != nil {
		log.Fatalf("Failed to instantiate DB: %s", err)
	}
	if err = tdb.Load(); err != nil {
		log.Fatalf("Failed to load DB: %s %s", dbConfig.Path, err)
	}
	rec, err := tdb.QuerySingle(*qType, *qName, *resolver, *subnet, maxans)
	if err != nil {
		log.Fatalf("%s", err)
	}
	fmt.Printf("%s\n%s\n", dns.RcodeToString[rec.Rcode], rec.Msg)
}
