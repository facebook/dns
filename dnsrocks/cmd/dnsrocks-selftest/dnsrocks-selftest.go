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
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"reflect"

	"github.com/miekg/dns"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/stats"
)

func progressLine(format string, args ...interface{}) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	fmt.Printf("\u001b[1000D")
	fmt.Printf(format, args...)
}

func doWork(dbConfig dnsserver.DBConfig, nets []*net.IPNet, workers int, qType, qName string, done chan bool) error {
	var (
		err           error
		tdb           *dnsserver.FBDNSDB
		handlerConfig dnsserver.HandlerConfig
		cacheConfig   dnsserver.CacheConfig
	)
	jobs := make(chan bool, workers)
	if tdb, err = dnsserver.NewFBDNSDB(handlerConfig, dbConfig, cacheConfig, &dnsserver.DummyLogger{}, &stats.DummyStats{}); err != nil {
		return fmt.Errorf("Failed to instantiate DB: %w", err)
	}
	if err = tdb.Load(); err != nil {
		return fmt.Errorf("Failed to load DB: %s %w", dbConfig.Path, err)
	}
	defer tdb.Close()
	var g errgroup.Group

	for _, n := range nets {
		n := n
		g.Go(func() error {
			jobs <- true
			defer func() {
				<-jobs
				done <- true
			}()
			// resolver IP
			resolver := n.IP.String()
			rec, err := tdb.QuerySingle(qType, qName, resolver, "", 1)
			if err != nil {
				return err
			}
			if rec.Rcode != dns.RcodeSuccess {
				log.Printf("querying %s %s from resolver '%s': non-successful rcode %s\n%s\n", qType, qName, resolver, dns.RcodeToString[rec.Rcode], rec.Msg)
				return fmt.Errorf("querying %s %s from resolver '%s': non-successful rcode %s", qType, qName, resolver, dns.RcodeToString[rec.Rcode])
			}
			if rec.Msg == nil || len(rec.Msg.Answer) == 0 {
				log.Printf("querying %s %s from resolver '%s': no answers\n%s\n", qType, qName, resolver, rec.Msg)
				return fmt.Errorf("querying %s %s from resolver '%s': no answers", qType, qName, resolver)
			}
			// client subnet
			subnet := n.String()
			rec, err = tdb.QuerySingle(qType, qName, "", subnet, 1)
			if err != nil {
				return err
			}
			if rec.Rcode != dns.RcodeSuccess {
				log.Printf("querying %s %s from client subnet '%s': non-successful rcode %s\n%s\n", qType, qName, subnet, dns.RcodeToString[rec.Rcode], rec.Msg)
				return fmt.Errorf("querying %s %s from client subnet '%s': non-successful rcode %s", qType, qName, subnet, dns.RcodeToString[rec.Rcode])
			}
			if rec.Msg == nil || len(rec.Msg.Answer) == 0 {
				log.Printf("querying %s %s from client subnet '%s': no answers\n%s\n", qType, qName, subnet, rec.Msg)
				return fmt.Errorf("querying %s %s from client subnet '%s': no answers", qType, qName, subnet)
			}
			return nil
		})
	}

	return g.Wait()
}

func verifyMaps() error {
	var (
		// tdb           *dnsserver.FBDNSDB
		dbConfig dnsserver.DBConfig
		// handlerConfig dnsserver.HandlerConfig
		// cacheConfig   dnsserver.CacheConfig
		dataPath   string
		qType      string
		qName      string
		workers    int
		batchSize  int
		noProgress bool
	)
	mapsCommand := flag.NewFlagSet("maps", flag.ExitOnError)
	mapsCommand.StringVar(&dbConfig.Path, "dbpath", "", "Path to compiled DB")
	mapsCommand.StringVar(&dbConfig.Driver, "dbdriver", "rocksdb", "DB driver")
	mapsCommand.StringVar(&dataPath, "datapath", "", "Path to data in TinyDNS format")
	mapsCommand.IntVar(&workers, "workers", 100, "Controls parallelism")
	mapsCommand.IntVar(&batchSize, "batchsize", 10000, "Controls how many records we query from DB before reopening it, controls mem consumption")
	mapsCommand.StringVar(&qType, "qtype", "A", "Type of the query")
	mapsCommand.StringVar(&qName, "qname", "fb.com", "Name to query")
	mapsCommand.BoolVar(&noProgress, "np", false, "Don't show progress")
	err := mapsCommand.Parse(os.Args[2:])
	if err != nil {
		return err
	}
	flag.Parse()
	log.Printf("Parsing all map subnets from %s", dataPath)
	nets, err := dnsdata.GetSubnets(dataPath)
	if err != nil {
		return fmt.Errorf("getting subnets from %s: %w", dbConfig.Path, err)
	}
	total := len(nets)
	done := make(chan bool, workers)
	log.Printf("total map subnets: %d\n", total)
	log.Printf("starting map verification by querying %s %s from every subnet\n", qType, qName)
	// basic progress indicator
	go func() {
		cur := 0
		for <-done {
			cur++
			progressLine("processed subnet %d/%d", cur, total)
			if cur == total {
				fmt.Println()
				return
			}
		}
	}()

	// split workload in batches, in each batch we open new DB and finally close it.
	// otherwise if we open DB and query all records from it really fast
	// RocksDB eats crazy amount of ram
	splits := int(math.Ceil(float64(total) / float64(batchSize)))
	for i := 0; i < splits; i++ {
		start := i * batchSize
		end := (i + 1) * batchSize
		if end > total {
			end = total
		}
		if err := doWork(dbConfig, nets[start:end], workers, qType, qName, done); err != nil {
			return err
		}
	}
	fmt.Println()
	log.Println("Queries from all subnets got answers")
	return nil
}

func verifyMarshal() error {
	var dataPath string
	marshalCommand := flag.NewFlagSet("marshal", flag.ExitOnError)
	marshalCommand.StringVar(&dataPath, "datapath", "", "Path to data in TinyDNS format")
	err := marshalCommand.Parse(os.Args[2:])
	if err != nil {
		return err
	}
	flag.Parse()
	codec := new(dnsdata.Codec)
	codec.Serial = 123456
	f, err := os.Open(dataPath)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lines := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		lines++
		r, err := codec.DecodeLn(line)
		if err != nil {
			return err
		}
		firstOut, err := r.MarshalMap()
		if err != nil {
			return err
		}
		gotText, err := r.MarshalText()
		if err != nil {
			return err
		}
		// now, parse marshalled text and see if it produces same record
		secondOut, err := codec.ConvertLn(gotText)
		if err != nil {
			return fmt.Errorf("error converting: %w", err)
		}
		if !reflect.DeepEqual(secondOut, firstOut) {
			log.Printf("encoded line `%s` differs after being marshalled to `%s`", line, gotText)
			return fmt.Errorf("encoded line `%s` differs (second parse vs first): %v != %v (\n%#v\n%#v\n)",
				line,
				secondOut, firstOut,
				secondOut, firstOut)
		}
		progressLine("processed line %d", lines)
	}
	fmt.Println()
	if err := scanner.Err(); err != nil {
		return err
	}
	log.Println("All lines parsed back to records and back successfully")
	return nil
}

func usage() {
	fmt.Printf(`
Usage: %q <test> <args>
Perform one of self-tests on live data.
Available self-tests:
	maps: validates that given record is resolvable from all subnets defined in given data file.
		Needs both compiled database and it's source data file in TinyDNS format.
	marshal: make sure that parsing record, serializing it back to text and parsing again is idempotent.
		Needs source data file in TinyDNS format.
`,
		os.Args[0])
}

func main() {
	if len(os.Args) <= 1 {
		usage()
		os.Exit(1)
	}
	testName := os.Args[1]

	testRegistry := map[string]func() error{
		"maps":    verifyMaps,
		"marshal": verifyMarshal,
	}
	f, found := testRegistry[testName]
	if !found {
		usage()
		log.Fatalf("Unknown test: %s", testName)
	}
	log.Printf("Running self-test '%s'", testName)
	if err := f(); err != nil {
		log.Fatalf("self-test '%s' failed: %v", testName, err)
	}
	fmt.Println("Self-Test completed successfully!")
}
