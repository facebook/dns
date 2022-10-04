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

package dnsdata

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

// GetSubnets parsed data file in tinydns format and returns all subnets from Net records (prefix %)
func GetSubnets(dataPath string) ([]*net.IPNet, error) {
	nets := []*net.IPNet{}
	codec := new(Codec)
	codec.Acc.NoPrefixSets = true
	codec.NoRnetOutput = true
	f, err := os.Open(dataPath)
	if err != nil {
		return nets, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if isIgnored(line) {
			continue
		}
		rtype := decodeRtype(line)
		if rtype != prefixNet {
			continue
		}
		r, err := codec.DecodeLn(line)
		if err != nil {
			return nets, fmt.Errorf("error decoding %s: %v", string(line), err)
		}
		rr := r.(*Rnet)
		nets = append(nets, rr.ipnet)
	}
	if err := scanner.Err(); err != nil {
		return nets, err
	}
	return nets, nil
}
