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

package fbserver

import (
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnssec"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// DNSSECConfig contains the zones and keys configs received from command line
// e.g comma-separated lists.
type DNSSECConfig struct {
	Zones string
	Keys  string
}

const (
	defaultCap = 10000 // default capacity of the cache.
)

func normalizeZone(zone string) string {
	return strings.ToLower(dns.Fqdn(zone))
}

// initializeZonesKeys initialize the keys for a given set of zones.
func initializeZonesKeys(zones []string, keyfiles []string) ([]string, []*dnssec.DNSKEY, bool, *cache.Cache, error) {
	nZones := []string{}
	keys, capacity, splitkeys, err := dnssecParse(keyfiles)
	if err != nil {
		return nZones, nil, false, nil, err
	}
	ca := cache.New(capacity)

	nZones = make([]string, len(zones))
	for i, z := range zones {
		nZones[i] = normalizeZone(z)
	}

	return nZones, keys, splitkeys, ca, nil
}

func dnssecParse(keyfiles []string) ([]*dnssec.DNSKEY, int, bool, error) {
	keys := make([]*dnssec.DNSKEY, len(keyfiles))

	capacity := defaultCap

	k, e := keyParse(keyfiles)
	if e != nil {
		return nil, 0, false, e
	}
	copy(keys, k)

	// Check if we have both KSKs and ZSKs.
	zsk, ksk := 0, 0
	for _, k := range keys {
		if isKSK(k) {
			ksk++
		} else if isZSK(k) {
			zsk++
		}
	}
	splitkeys := zsk > 0 && ksk > 0

	return keys, capacity, splitkeys, nil
}

// Return true if, and only if, this is a zone key with the SEP bit unset. This implies a ZSK (rfc4034 2.1.1).
func isZSK(k *dnssec.DNSKEY) bool {
	return k.K.Flags&(1<<8) == (1<<8) && k.K.Flags&1 == 0
}

// Return true if, and only if, this is a zone key with the SEP bit set. This implies a KSK (rfc4034 2.1.1).
func isKSK(k *dnssec.DNSKEY) bool {
	return k.K.Flags&(1<<8) == (1<<8) && k.K.Flags&1 == 1
}

func keyParse(keyfiles []string) ([]*dnssec.DNSKEY, error) {
	keys := []*dnssec.DNSKEY{}

	for _, k := range keyfiles {
		glog.Infof("Loading DNSSEC key %s", k)
		base := k
		// Kmiek.nl.+013+26205.key, handle .private or without extension: Kmiek.nl.+013+26205
		if strings.HasSuffix(k, ".key") {
			base = k[:len(k)-4]
		}
		if strings.HasSuffix(k, ".private") {
			base = k[:len(k)-8]
		}
		k, err := dnssec.ParseKeyFile(base+".key", base+".private")
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}

	return keys, nil
}

func newDNSSECHandler(srv *Server, next plugin.Handler) (dnssec.Dnssec, error) {
	cZones := strings.Split(srv.conf.DNSSECConfig.Zones, ",")
	cKeys := strings.Split(srv.conf.DNSSECConfig.Keys, ",")
	zones, keys, splitkeys, ca, err := initializeZonesKeys(cZones, cKeys)
	if err != nil {
		return dnssec.Dnssec{}, err
	}
	return dnssec.New(zones, keys, splitkeys, next, ca), nil
}
