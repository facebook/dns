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
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/facebook/dns/dnsrocks/dnsserver"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
	"github.com/facebook/dns/dnsrocks/pluginmap"
	"github.com/facebook/dns/dnsrocks/tlsconfig"
)

// HandlerFactory adds a configured handler in front of an existing chain. It
// receives the server's stats and logger so that a handler which answers a
// query itself, rather than passing it to the database handler at the end of
// the chain, can still record it where that handler would have.
type HandlerFactory func(plugin.Handler, stats.Stats, dnsserver.Logger) (plugin.Handler, error)

// ServerConfig represent the configuration for a given DNS server
type ServerConfig struct {
	IPAns            ipAns
	Port             int
	MaxUDPSize       int
	TCP              bool
	TLS              bool
	ReusePort        int
	MaxTCPQueries    int
	TCPIdleTimeout   time.Duration
	NumCPU           int
	MaxConcurrency   int
	ReadTimeout      time.Duration
	TLSConfig        tlsconfig.TLSConfig
	HandlerConfig    dnsserver.HandlerConfig
	CacheConfig      dnsserver.CacheConfig
	DBConfig         dnsserver.DBConfig
	PluginRegistry   *pluginmap.Registry
	WhoamiDomain     string
	HandlerFactories []HandlerFactory
	RefuseANY        bool
	DNSSECConfig     DNSSECConfig
	NSID             bool
	PrivateInfo      bool
}

type ipAns map[string]int

func (ipans ipAns) String() string {
	if ipans == nil {
		return ""
	}
	vals := make([]string, 0, len(ipans))
	for k, v := range ipans {
		vals = append(vals, fmt.Sprintf("%s,%d", k, v))
	}
	slices.Sort(vals)
	return strings.Join(vals, " ")
}

// Support setting ipAns with only "IP" or "IP,maxAns"
func (ipans ipAns) Set(v string) error {
	ipAnsSpt := strings.Split(v, ",")
	if len(ipAnsSpt) > 2 {
		return fmt.Errorf("invalid argument value %q: expected IP or IP,maxAns", v)
	}

	ip := net.ParseIP(ipAnsSpt[0])
	if ip == nil {
		return fmt.Errorf("invalid IP address %q", ipAnsSpt[0])
	}
	ans := dnsserver.DefaultMaxAnswer
	ipStr := ip.String()
	if len(ipAnsSpt) == 2 {
		num, err := strconv.Atoi(ipAnsSpt[1])
		if err != nil {
			return fmt.Errorf("invalid max answer %q: %w", ipAnsSpt[1], err)
		}
		if num <= 0 {
			return fmt.Errorf("max answer must be greater than zero, got %d", num)
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
