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
	"os"
	"strings"

	"github.com/miekg/dns"

	"github.com/facebookincubator/dns/dnsrocks/db"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
)

func main() {
	var (
		err                 error
		dbConfig            dnsserver.DBConfig
		packedQName         = make([]byte, 255)
		cdb                 *db.DB
		reader              db.Reader
		ecsLoc, resolverLoc *db.Location
		offset              int
		csvPath             string
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), `
	Provided a data.cdb file and a CSV formated file of resolver IP, ECS, qname,
	runs a query against the DB and gather both results for ECS or resolver based
	results and print the results when they differ.
	See https://fburl.com/fbdns-ecscmp for more information as to how to use this tool.
	`)
		flag.PrintDefaults()
	}
	flag.StringVar(&dbConfig.Path, "dbpath", "", "Path to CDB")
	flag.StringVar(&csvPath, "csvpath", "", "Path to CSV file. Format is resolverIP,ECS,QName")

	flag.Parse()

	if cdb, err = db.Open(dbConfig.Path, "cdb"); err != nil {
		log.Fatalf("Failed to open data.cdb file: %v", err)
	}

	if reader, err = db.NewReader(cdb); err != nil {
		log.Fatalf("Failed create new CDB Reader: %v", err)
	}

	csvFile, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("Failed to open CSV file: %v", err)
	}
	defer csvFile.Close()

	scanner := bufio.NewScanner(csvFile)
	resultTotal := 0
	resultDiffer := 0
	for scanner.Scan() {
		s := strings.Split(strings.TrimSpace(scanner.Text()), ",")
		fqdn := dns.Fqdn(s[2])
		if offset, err = dns.PackDomainName(fqdn, packedQName, 0, nil, false); err != nil {
			log.Fatal(err)
		}
		// Without ECS
		req := makeDNSPacket(fqdn, dns.TypeA)

		if _, resolverLoc, err = reader.FindLocation(packedQName[:offset], req, s[0]); err != nil {
			log.Fatalf("Failed to find location without ECS set: %v", err)
		}

		// With ECS
		setECS(req, s[1])
		if _, ecsLoc, err = reader.FindLocation(packedQName[:offset], req, s[0]); err != nil {
			log.Fatalf("Failed to find location with ECS set: %v", err)
		}
		resultTotal++
		if resolverLoc.LocID[0] != ecsLoc.LocID[0] || resolverLoc.LocID[1] != ecsLoc.LocID[1] {

			resultDiffer++
			fmt.Printf("%d %d\n", resolverLoc.LocID[1], ecsLoc.LocID[1])
		}
	}

	fmt.Printf("Targeting between ECS and Resolver. Identical: %d, Different: %d\n", resultTotal-resultDiffer, resultDiffer)
	if err := scanner.Err(); err != nil {
		log.Fatalf("Failed to read line from : %v", err)
	}
}

func makeDNSPacket(qname string, qtype uint16) *dns.Msg {
	req := new(dns.Msg)

	req.SetQuestion(qname, qtype)
	return req
}

func setECS(msg *dns.Msg, subnet string) {

	o, err := dnsserver.MakeOPTWithECS(subnet)
	if err != nil {
		log.Fatalf("Failed to generate ECS option for %s %v", subnet, err)
	}
	msg.Extra = []dns.RR{o}

}
