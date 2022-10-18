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

package fbserver

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/tlsconfig"

	"github.com/golang/glog"
)

// ServerConfig represent the configuration for a given DNS server
type ServerConfig struct {
	IPAns          ipAns
	Port           int
	TCP            bool
	TLS            bool
	ReusePort      int
	MaxTCPQueries  int
	TCPIdleTimeout time.Duration
	ReadTimeout    time.Duration
	TLSConfig      tlsconfig.TLSConfig
	HandlerConfig  dnsserver.HandlerConfig
	CacheConfig    dnsserver.CacheConfig
	DBConfig       dnsserver.DBConfig
	WhoamiDomain   string
	RefuseANY      bool
	DNSSECConfig   DNSSECConfig
}

type ipAns map[string]int

func (ipans ipAns) String() string {
	if ipans == nil {
		return ""
	}
	vals := make([]string, len(ipans))
	for k, v := range ipans {
		vals = append(vals, fmt.Sprintf("%s : %v, ", k, v))
	}
	return strings.Join(vals, ",")
}

// Support setting ipAns with only "IP" or "IP,maxAns"
func (ipans ipAns) Set(v string) error {
	ipAnsSpt := strings.Split(v, ",")

	ip := net.ParseIP(ipAnsSpt[0])
	ans := dnsserver.DefaultMaxAnswer
	ipStr := ip.String()
	if len(ipAnsSpt) >= 2 {
		num, err := strconv.Atoi(ipAnsSpt[1])
		if err != nil {
			glog.Fatalf("Failed to convert '%s' to int, error: %v", ipAnsSpt[1], err)
		}
		ans = num
	}
	ipans[ipStr] = ans

	return nil
}

// NewServerConfig returns a fully initialized server configuration.
func NewServerConfig() (s ServerConfig) {
	s.IPAns = make(ipAns)
	return
}
