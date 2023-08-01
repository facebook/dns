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
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/facebook/dns/dnsrocks/metrics"
)

// ConnectionMonitor is a wrapper around `net.Conn` which serves to log per connection metrics.
type ConnectionMonitor struct {
	connection    net.Conn
	transportName MonitorType
	stats         *metrics.Stats
}

// NewConnectionMonitor makes a new ConnectionMonitor from `net.Conn`.
// It also initializes and increments the appropriate counters for the connection.
func NewConnectionMonitor(c net.Conn, transportName MonitorType, s *metrics.Stats) *ConnectionMonitor {
	cm := &ConnectionMonitor{
		connection:    c,
		transportName: transportName,
		stats:         s,
	}

	if cm.stats == nil {
		cm.stats = metrics.NewStats()
	}

	cm.stats.IncrementCounter(formatConnectionMonitorStatName(cm.transportName, "Accept", true))
	cm.stats.IncrementCounter(formatConnectionMonitorStatName(cm.transportName, "Count", false))

	return cm
}

// Read is a passthrough for `net.Conn.Read`
func (cm *ConnectionMonitor) Read(b []byte) (int, error) {
	return cm.connection.Read(b)
}

// Write is a passthrough for `net.Conn.Write`
func (cm *ConnectionMonitor) Write(b []byte) (int, error) {
	return cm.connection.Write(b)
}

// Close calls `net.Conn.Close` and then increments counters
func (cm *ConnectionMonitor) Close() error {
	err := cm.connection.Close()
	if err != nil {
		return err
	}

	cm.stats.IncrementCounterBy(formatConnectionMonitorStatName(cm.transportName, "Close", true), 1)
	cm.stats.IncrementCounterBy(formatConnectionMonitorStatName(cm.transportName, "Count", false), -1)

	return nil
}

// LocalAddr is a passthrough for `net.Conn.LocalAddr`
func (cm *ConnectionMonitor) LocalAddr() net.Addr {
	return cm.connection.LocalAddr()
}

// RemoteAddr is a passthrough for `net.Conn.RemoteAddr`
func (cm *ConnectionMonitor) RemoteAddr() net.Addr {
	return cm.connection.RemoteAddr()
}

// SetDeadline is a passthrough for `net.Conn.SetDeadline`
func (cm *ConnectionMonitor) SetDeadline(t time.Time) error {
	return cm.connection.SetDeadline(t)
}

// SetReadDeadline is a passthrough for `net.Conn.SetReadDeadline`
func (cm *ConnectionMonitor) SetReadDeadline(t time.Time) error {
	return cm.connection.SetReadDeadline(t)
}

// SetWriteDeadline is a passthrough for `net.Conn.SetWriteDeadline`
func (cm *ConnectionMonitor) SetWriteDeadline(t time.Time) error {
	return cm.connection.SetWriteDeadline(t)
}

// TLSConnectionMonitor is a wrapper around `tls.Conn` which serves to log per connection metrics.
type TLSConnectionMonitor struct {
	connection    *tls.Conn
	transportName MonitorType
	stats         *metrics.Stats
}

// NewTLSConnectionMonitor makes a new ConnectionMonitor from `net.Conn`.
// It also initializes and increments the appropriate counters for the connection.
func NewTLSConnectionMonitor(c net.Conn, transportName MonitorType, s *metrics.Stats) *TLSConnectionMonitor {
	cm := &TLSConnectionMonitor{
		connection:    c.(*tls.Conn),
		transportName: transportName,
		stats:         s,
	}

	if cm.stats == nil {
		cm.stats = metrics.NewStats()
	}

	cm.stats.IncrementCounter(formatConnectionMonitorStatName(cm.transportName, "Accept", true))
	cm.stats.IncrementCounter(formatConnectionMonitorStatName(cm.transportName, "Count", false))

	return cm
}

// Read is a passthrough for `net.Conn.Read`
func (cm *TLSConnectionMonitor) Read(b []byte) (int, error) {
	return cm.connection.Read(b)
}

// Write is a passthrough for `net.Conn.Write`
func (cm *TLSConnectionMonitor) Write(b []byte) (int, error) {
	return cm.connection.Write(b)
}

// Close calls `net.Conn.Close` and then increments counters
func (cm *TLSConnectionMonitor) Close() error {
	err := cm.connection.Close()
	if err != nil {
		return err
	}

	cm.stats.IncrementCounterBy(formatConnectionMonitorStatName(cm.transportName, "Close", true), 1)
	cm.stats.IncrementCounterBy(formatConnectionMonitorStatName(cm.transportName, "Count", false), -1)

	return nil
}

// LocalAddr is a passthrough for `net.Conn.LocalAddr`
func (cm *TLSConnectionMonitor) LocalAddr() net.Addr {
	return cm.connection.LocalAddr()
}

// RemoteAddr is a passthrough for `net.Conn.RemoteAddr`
func (cm *TLSConnectionMonitor) RemoteAddr() net.Addr {
	return cm.connection.RemoteAddr()
}

// SetDeadline is a passthrough for `net.Conn.SetDeadline`
func (cm *TLSConnectionMonitor) SetDeadline(t time.Time) error {
	return cm.connection.SetDeadline(t)
}

// SetReadDeadline is a passthrough for `net.Conn.SetReadDeadline`
func (cm *TLSConnectionMonitor) SetReadDeadline(t time.Time) error {
	return cm.connection.SetReadDeadline(t)
}

// SetWriteDeadline is a passthrough for `net.Conn.SetWriteDeadline`
func (cm *TLSConnectionMonitor) SetWriteDeadline(t time.Time) error {
	return cm.connection.SetWriteDeadline(t)
}

// ConnectionState is a passthrough for `tls.Conn.ConnectionState`
func (cm *TLSConnectionMonitor) ConnectionState() tls.ConnectionState {
	return cm.connection.ConnectionState()
}

// PacketConnectionMonitor is a wrapper around `net.PacketConn` which
// servers to log per request metrics
type PacketConnectionMonitor struct {
	connection    net.PacketConn
	transportName MonitorType
	stats         *metrics.Stats
}

// NewPacketConnectionMonitor makes a new PacketConnectionMonitor from `net.PacketConn`.
// It also initializes and increments the appropriate counters for the connection.
func NewPacketConnectionMonitor(c net.PacketConn, transportName MonitorType, s *metrics.Stats) *PacketConnectionMonitor {
	pcm := &PacketConnectionMonitor{
		connection:    c,
		transportName: transportName,
		stats:         s,
	}

	if pcm.stats == nil {
		pcm.stats = metrics.NewStats()
	}

	pcm.stats.ResetCounter(formatConnectionMonitorStatName(pcm.transportName, "ReadFrom", true))
	pcm.stats.ResetCounter(formatConnectionMonitorStatName(pcm.transportName, "Close", true))
	pcm.stats.ResetCounter(formatConnectionMonitorStatName(pcm.transportName, "Count", false))

	return pcm
}

// ReadFrom calls `net.PacketConn.ReadFrom` and increments the counter
func (pcm *PacketConnectionMonitor) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := pcm.connection.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}

	pcm.stats.IncrementCounter(formatConnectionMonitorStatName(pcm.transportName, "Count", false))
	pcm.stats.IncrementCounter(formatConnectionMonitorStatName(pcm.transportName, "ReadFrom", true))

	return n, addr, nil
}

// WriteTo is a passthrough for `net.PacketConn.WriteTo`
func (pcm *PacketConnectionMonitor) WriteTo(p []byte, addr net.Addr) (int, error) {
	return pcm.connection.WriteTo(p, addr)
}

// Close calls `net.PacketConn.Close` and then increments counters
func (pcm *PacketConnectionMonitor) Close() error {
	err := pcm.connection.Close()
	if err != nil {
		return err
	}

	pcm.stats.IncrementCounterBy(formatConnectionMonitorStatName(pcm.transportName, "Close", true), 1)

	return nil
}

// LocalAddr is a passthrough for `net.PacketConn.LocalAddr`
func (pcm *PacketConnectionMonitor) LocalAddr() net.Addr {
	return pcm.connection.LocalAddr()
}

// SetDeadline is a passthrough for `net.PacketConn.SetDeadline`
func (pcm *PacketConnectionMonitor) SetDeadline(t time.Time) error {
	return pcm.connection.SetDeadline(t)
}

// SetReadDeadline is a passthrough for `net.PacketConn.SetReadDeadline`
func (pcm *PacketConnectionMonitor) SetReadDeadline(t time.Time) error {
	return pcm.connection.SetReadDeadline(t)
}

// SetWriteDeadline is a passthrough for `net.PacketConn.SetWriteDeadline`
func (pcm *PacketConnectionMonitor) SetWriteDeadline(t time.Time) error {
	return pcm.connection.SetWriteDeadline(t)
}

// formatConnectionMonitorStatName formats keys for the stats
func formatConnectionMonitorStatName(connectionType MonitorType, statName string, call bool) string {
	op := ""
	if call {
		op = "_calls"
	}

	return fmt.Sprintf("%s_connection_%s%s", connectionType, statName, op)
}
