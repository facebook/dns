/*
Copyright (c) Facebook, Inc. and its affiliates.
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

package snoop

import (
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// TestWatch is used to ensure that Watch populates Consumer
// maps with data from filter queue.
func TestWatch(t *testing.T) {
	c := Consumer{
		Config: &Config{
			CleanPeriod: 1 * time.Minute,
		},
	}
	c.Setup()
	for i := 0; i < 10; i++ {
		c.filterQueue <- &FilterDTO{
			Timestamp: int64(i),
			DstPort:   10,
			SrcPort:   10,
			DNS: &layers.DNS{
				ID: uint16(i),
				QR: false,
			},
		}
	}
	for i := 0; i < 10; i++ {
		c.probeQueue <- &ProbeDTO{
			ProbeData: EnhancedProbeData{
				Tgid:       uint32(3 * i),
				SockPortNr: int32(2 * i),
			},
		}
	}
	go func() {
		c.Watch()
	}()
	time.Sleep(1 * time.Second)

	require.Equal(t, 0, c.portToProcess[0].pid)
	require.Equal(t, 3, c.portToProcess[2].pid)
	require.Equal(t, 6, c.portToProcess[4].pid)

	require.Equal(t, int64(2), c.displayMap[UniqueDNS{port: PortNr(10), dnsID: 2}].qTimestamp)
	require.Equal(t, int64(1), c.displayMap[UniqueDNS{port: PortNr(10), dnsID: 1}].qTimestamp)
	require.Equal(t, int64(7), c.displayMap[UniqueDNS{port: PortNr(10), dnsID: 7}].qTimestamp)

	c.CleanDisplayMap()
	require.Equal(t, 0, len(c.displayMap))
}

func TestDisplayMapToToplike(t *testing.T) {
	c := Consumer{
		Config: &Config{
			CleanPeriod: 1 * time.Minute,
		},
	}
	c.Setup()
	for i := 0; i < 10; i++ {
		c.filterQueue <- &FilterDTO{
			Timestamp: int64(i),
			DstPort:   10,
			SrcPort:   uint16(i % 5),
			DNS: &layers.DNS{
				ID: uint16(i % 5),
				QR: false,
			},
		}
	}
	for i := 0; i < 10; i++ {
		c.probeQueue <- &ProbeDTO{
			ProbeData: EnhancedProbeData{
				Pid:        0,
				SockPortNr: int32(2 * i),
			},
		}
	}
	go func() {
		c.Watch()
	}()
	time.Sleep(1 * time.Second)

	top := c.displayMapToToplike()
	require.Equal(t, 5, top.total)
	require.Equal(t, 1, len(top.Rows))
}
