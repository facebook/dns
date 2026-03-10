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

package dnsmetrics

import (
	"context"
	"sync/atomic"

	"fb303/thrift/fb303_core"
	"libfb/go/fbthrift"

	"github.com/facebook/fbthrift/thrift/lib/go/thrift"
	"libfb/go/crypto/fbtls"
	"libfb/go/stats/export"
	"libfb/go/stats/sysstat"

	"github.com/golang/glog"
)

// ThriftMetricsServer is the metrics exporter for the fb wwrld
type ThriftMetricsServer struct {
	server thrift.Server
	// Status used for GetStatus thrift method
	status atomic.Value
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMetricsServer creates a ThriftMetricsServer
func NewMetricsServer(addr string) (server *ThriftMetricsServer, err error) {
	server = &ThriftMetricsServer{}
	server.status.Store(fb303_core.Fb303Status_STARTING)

	server.ctx, server.cancel = context.WithCancel(context.Background())

	_ = sysstat.ExportTo(server.ctx, export.Get())

	glog.Infof("Starting thrift fb303 server at %q\n", addr)

	// Advertise via ALPN that we support Rocket transport.
	// Rocket-enabled clients will be able to talk Rocket to us
	// immediately, without having to "upgradeToRocket" first.
	listener, err := fbtls.Listen(addr)
	if err != nil {
		return nil, err
	}
	server.server = fbthrift.NewServer(
		listener,
		fbthrift.WithServiceName("fbdns"),
		fbthrift.WithExported(export.Get()),
		fbthrift.WithCustomStatusHook(server.fb303StatusHook),
	)
	return server, nil
}

// Serve launches the thrift server
func (s *ThriftMetricsServer) Serve() error {
	return s.server.ServeContext(s.ctx)
}

// SetAlive sets the Alive status on the thrift server
func (s *ThriftMetricsServer) SetAlive() {
	s.status.Store(fb303_core.Fb303Status_ALIVE)
}

// ConsumeStats registers a stats instance to be exported via the thrift metrics exporter
func (s *ThriftMetricsServer) ConsumeStats(category string, stats *Stats) error {
	export.Get().ExportInt(category, stats)
	return nil
}

// UpdateExporter Empty for fb metrics server, needed to match the interface
func (s *ThriftMetricsServer) UpdateExporter() {
}

func (s *ThriftMetricsServer) fb303StatusHook() fb303_core.Fb303Status {
	return s.status.Load().(fb303_core.Fb303Status)
}
