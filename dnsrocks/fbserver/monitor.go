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

	"github.com/facebookincubator/dns/dnsrocks/metrics"
)

// MonitorType is a transport protocol string (e.g., "tcp").
type MonitorType string

// These are the transport protocols and extensions that Monitor supports.
const (
	monitorTCP        MonitorType = "tcp"     // Unencrypted TCP
	monitorTCPWithTLS MonitorType = "tcp-tls" // TLS encrypted TCP.
	monitorUDP        MonitorType = "udp"     // UDP
)

// Monitor is a net.Listener that logs and captures socket metrics.
type Monitor struct {
	listener      net.Listener
	transportName MonitorType
	stats         *metrics.Stats
}

// NewMonitor creates a Monitor from a net.Listener.
func NewMonitor(l net.Listener, t MonitorType, s *metrics.Stats) *Monitor {
	m := &Monitor{
		listener:      l,
		transportName: t,
		stats:         s,
	}

	if m.stats == nil {
		m.stats = metrics.NewStats()
	}
	m.initListenerStats("Accept", "Close", "Addr", "Read")
	m.initConnectionStats("Accept", "Close")
	return m
}

// Accept monitors net.Listener.Accept.
func (m *Monitor) Accept() (net.Conn, error) {
	conn, err := m.listener.Accept()
	if err != nil {
		return nil, err
	}
	m.incStat("Accept")
	if m.transportName == monitorTCPWithTLS {
		return NewTLSConnectionMonitor(conn, m.transportName, m.stats), err
	}
	return NewConnectionMonitor(conn, m.transportName, m.stats), err
}

// Close monitors net.listener.Close.
func (m *Monitor) Close() error {
	if err := m.listener.Close(); err != nil {
		return err
	}
	m.incStat("Close")
	return nil
}

// Addr increments a counter and then calls net.Listener.Addr.
func (m *Monitor) Addr() net.Addr {
	m.incStat("Addr")
	return m.listener.Addr()
}

// initListenerStats clears a list method call counters to a default value.
func (m *Monitor) initListenerStats(methods ...string) {
	for _, method := range methods {
		m.stats.ResetCounter(formatMonitorStatName(m.transportName, method))
	}
}

// initConnectionStats initializes all non method call stats
func (m *Monitor) initConnectionStats(methods ...string) {
	m.stats.ResetCounter(formatConnectionMonitorStatName(m.transportName, "Count", false))
	for _, method := range methods {
		m.stats.ResetCounter(formatConnectionMonitorStatName(m.transportName, method, true))
	}
}

// incStat increments a Monitor ODS counter.
func (m *Monitor) incStat(method string) {
	m.stats.IncrementCounter(formatMonitorStatName(m.transportName, method))
}

// formatMonitorStatName formats an ODS counter name for Monitor.
func formatMonitorStatName(t MonitorType, method string) string {
	return fmt.Sprintf("%s_listener_%s_calls", t, method)
}
