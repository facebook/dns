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
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/facebookincubator/dns/dnsrocks/metrics"
)

// listenUDP configures a socket with SO_REUSEPORT and returns a UDP
// net.PacketConn.
func listenUDP(addr string, conf net.ListenConfig) (net.PacketConn, error) {
	return conf.ListenPacket(context.Background(), "udp", addr)
}

// listenTCP configures a socket with SO_REUSEPORT. It then uses the socket to
// create an unencrypted, monitored TLS net.Listener. The monitor sends
// connection metrics to fb303.
func listenTCP(addr string, conf net.ListenConfig, stats *metrics.Stats) (*Monitor, error) {
	list, err := conf.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, err
	}
	return NewMonitor(list, monitorTCP, stats), nil
}

// listenTLS configures a socket with SO_REUSEPORT. It then uses the socket to
// create an encrypted, monitored TLS net.Listener. The monitor sends connection
// metrics to fb303.
func listenTLS(addr string, conf net.ListenConfig, tlsConf *tls.Config, stats *metrics.Stats) (*Monitor, error) {
	tcpList, err := conf.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("could not open tcp listener for tls conn: %w", err)
	}
	tlsList := tls.NewListener(tcpList, tlsConf)
	return NewMonitor(tlsList, monitorTCPWithTLS, stats), nil
}
