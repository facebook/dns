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
	"os"
	"path"
	"testing"
	"time"

	"github.com/facebookincubator/dns/dnsrocks/dnsserver"
	"github.com/facebookincubator/dns/dnsrocks/dnsserver/stats"
	"github.com/facebookincubator/dns/dnsrocks/fbserver"
	"github.com/facebookincubator/dns/dnsrocks/metrics"
	"github.com/facebookincubator/dns/dnsrocks/testaid"

	"github.com/stretchr/testify/assert"
)

// Reasonable timeout (database is small and it should not take long to load it
// and start serving + we don't want to wait too long for tests to finish)
const WaitTimeout = 5 * time.Second

func TestMain(m *testing.M) {
	os.Exit(testaid.Run(m, "../../testdata/data"))
}

func getConfig(tcp bool) fbserver.ServerConfig {
	var serverConfig fbserver.ServerConfig = fbserver.NewServerConfig()

	// DNS Server config
	serverConfig.Port = 0
	serverConfig.TCP = tcp
	serverConfig.MaxTCPQueries = -1
	serverConfig.ReusePort = 0
	serverConfig.WhoamiDomain = ""
	serverConfig.RefuseANY = false
	// the default setup should be backward compatible with current spec: 1 IP address and maxanswer not specified

	// DB config
	db := testaid.TestCDB
	serverConfig.DBConfig.ReloadInterval = 100
	serverConfig.DBConfig.Path = db.Path
	serverConfig.DBConfig.Driver = db.Driver

	// Cache config
	serverConfig.CacheConfig.Enabled = false
	serverConfig.CacheConfig.LRUSize = 1024 * 1024
	serverConfig.CacheConfig.WRSTimeout = 0
	return serverConfig
}

func getFBServer(t *testing.T) (*fbserver.Server, func()) {
	serverConfig := getConfig(true)
	thriftAddr := ":0"

	serverConfig.DBConfig.Path = path.Clean(serverConfig.DBConfig.Path)

	// Thrift server
	dummyServer, err := metrics.NewMetricsServer(thriftAddr)
	assert.Nilf(t, err, "Error initializing thrift server: %s", err)

	// Logger
	l := &dnsserver.DummyLogger{}

	// stat collector
	stats := &stats.DummyStats{}

	srv := fbserver.NewServer(serverConfig, l, stats, dummyServer)
	return srv, func() {
		srv.Shutdown()
	}
}

// Wait() call should hang forever because Done() is not called anywhere
func Test_ServerWGWaitShouldHangForever(t *testing.T) {
	srv, cleanup := getFBServer(t)
	defer cleanup()

	assert.Nil(t, srv.Start(), "Failed to start server")
	waitChan := make(chan bool, 1)
	go func() {
		srv.ServersStartedWG.Wait()
		waitChan <- true
	}()

	select {
	case <-waitChan:
		t.Errorf("Wait should block forever")
	case <-time.After(WaitTimeout):
	}
}

// Wait() call should return because Done() is called in NotifyStartedFunc
func Test_ServerWGWaitShouldReturn(t *testing.T) {
	srv, cleanup := getFBServer(t)
	defer cleanup()

	srv.NotifyStartedFunc = func() {
		srv.ServersStartedWG.Done()
	}
	assert.Nil(t, srv.Start(), "Failed to start server")
	waitChan := make(chan bool, 1)
	go func() {
		srv.ServersStartedWG.Wait()
		waitChan <- true
	}()

	select {
	case <-waitChan:
	case <-time.After(WaitTimeout):
		t.Errorf("Wait should not block")
	}
}
