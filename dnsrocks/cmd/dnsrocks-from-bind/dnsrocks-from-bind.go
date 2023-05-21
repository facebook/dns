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
	"io"
	"os"
	"strings"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

func getRecs(r io.Reader, origin string) ([]dns.RR, error) {
	zp := dns.NewZoneParser(r, origin, "")
	results := []dns.RR{}

	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		results = append(results, rr)
	}

	if err := zp.Err(); err != nil {
		return results, err
	}
	return results, nil
}

// remove extra dots
func normalizeDot(s string) string {
	f := strings.Split(s, ".")
	toWrite := make([]string, 0, len(f))
	for _, s := range f {
		n := len(s)
		if n > 0 {
			toWrite = append(toWrite, s[:n])
		}
	}
	return strings.Join(toWrite, ".")
}

// order in which we output each record type
var order = []string{
	"A",
	"AAAA",
	"CNAME",
	"MX",
	"TXT",
	"SRV",
	"SPF",
}

func adjustTTL(ttl uint32) uint32 {
	if ttl < 3600 {
		return uint32(3600)
	}
	return ttl
}

func processRecs(recs []dns.RR) map[string][]string {
	results := map[string][]string{}
	for _, rr := range recs {
		// TODO: we can create dnsdata records and use MarshalText,
		// but now it only outputs full form which is not the best for human reading
		switch v := rr.(type) {
		case *dns.A:
			// +fqdn,ip,ttl,timestamp,lo
			line := fmt.Sprintf("+%s,%s,%d", normalizeDot(v.Hdr.Name), v.A, adjustTTL(v.Hdr.Ttl))
			results["A"] = append(results["A"], line)
		case *dns.AAAA:
			// +fqdn,ip,ttl,timestamp,lo
			line := fmt.Sprintf("+%s,%s,%d", normalizeDot(v.Hdr.Name), v.AAAA, adjustTTL(v.Hdr.Ttl))
			results["AAAA"] = append(results["AAAA"], line)
		case *dns.CNAME:
			// Cfqdn,x,ttl,timestamp,lo
			line := fmt.Sprintf("C%s,%s,%d", normalizeDot(v.Hdr.Name), normalizeDot(v.Target), adjustTTL(v.Hdr.Ttl))
			results["CNAME"] = append(results["CNAME"], line)
		case *dns.TXT:
			// fqdn,s,ttl,timestamp,lo
			line := fmt.Sprintf("'%s,%s,%d", normalizeDot(v.Hdr.Name), v.Txt[0], adjustTTL(v.Hdr.Ttl))
			results["TXT"] = append(results["TXT"], line)
		case *dns.SPF:
			// fqdn,s,ttl,timestamp,lo
			line := fmt.Sprintf("'%s,%s,%d", normalizeDot(v.Hdr.Name), v.Txt[0], adjustTTL(v.Hdr.Ttl))
			results["SPF"] = append(results["SPF"], line)
		case *dns.SRV:
			// Sfqdn,ip,x,port,priority,weight,ttl,timestamp
			line := fmt.Sprintf("S%s,,%s,%d,%d,%d,%d", normalizeDot(v.Hdr.Name), normalizeDot(v.Target), v.Port, v.Priority, v.Weight, adjustTTL(v.Hdr.Ttl))
			results["SRV"] = append(results["SRV"], line)
		case *dns.MX:
			// @fqdn,ip,x,dist,ttl,timestamp,lo
			line := fmt.Sprintf("@%s,,%s,%d,%d", normalizeDot(v.Hdr.Name), normalizeDot(v.Mx), v.Preference, adjustTTL(v.Hdr.Ttl))
			results["MX"] = append(results["MX"], line)
		// ignore
		case *dns.SOA:
		case *dns.NS:
		default:
			log.Warningf("Unsupported record type %T", rr)
		}
	}
	return results
}

func getSOAns(origin string) []string {
	result := []string{
		fmt.Sprintf("Z%s,a.ns.facebook.com,dns.facebook.com,,14400,1800,604800,3600,3600", origin),
	}
	for _, ns := range []string{"a", "b", "c", "d"} {
		result = append(result, fmt.Sprintf("&%s,,%s.ns.facebook.com,172800", origin, ns))
	}
	return result
}

func main() {
	rawOrigin := flag.String("origin", "", "Zone's origin to fill apex records if not explicitly specified")
	addSOA := flag.Bool("addSOA", true, "If we should generate SOA and NS records")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Convert DNS Zone in BIND/RFC 1035 format to TinyDNS/FBDNS format.\nWARNING: Ignores NS/SOA records.\nTTL < 3600 are set to 3600\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -origin example.com < /tmp/zone.bind > /tmp/zone.tiny\n", os.Args[0])
	}
	flag.Parse()
	if *addSOA && *rawOrigin == "" {
		log.Fatal("You need to specify 'origin' for SOA and NS is records")
	}
	origin := normalizeDot(*rawOrigin)
	rrs, err := getRecs(os.Stdin, origin)
	if err != nil {
		log.Fatalf("Failed parsing: %v", err)
	}
	recMap := processRecs(rrs)
	if *addSOA {
		recs := getSOAns(origin)
		for _, line := range recs {
			fmt.Println(line)
		}
		fmt.Println()
	}
	// group by record type
	for _, k := range order {
		lines := recMap[k]
		if len(lines) > 0 {
			fmt.Printf("# %ss\n", k)
			for _, line := range lines {
				fmt.Println(line)
			}
			fmt.Println()
		}
	}
}
