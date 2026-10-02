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

package dnsserver

import (
	"context"
	"errors"
	"testing"

	"github.com/facebook/dns/dnsrocks/db"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
	"github.com/facebook/dns/dnsrocks/pluginmap"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type locationSelectionReader struct {
	db.Reader
	pluginMap      db.PluginMap
	pluginMapFound bool
	pluginMapErr   error
	pluginMapCalls int
	legacyLocation *db.Location
	rcodes         map[string]int
	probed         []db.ID
	legacyCalls    int
}

func (r *locationSelectionReader) FindPluginMap([]byte) (db.PluginMap, bool, error) {
	r.pluginMapCalls++
	if r.pluginMapErr != nil {
		return db.PluginMap{}, false, r.pluginMapErr
	}
	return r.pluginMap, r.pluginMapFound, nil
}

func (r *locationSelectionReader) FindLocation(
	[]byte,
	*dns.EDNS0_SUBNET,
	string,
) (*db.Location, error) {
	r.legacyCalls++
	return r.legacyLocation, nil
}

func (r *locationSelectionReader) IsAuthoritative(
	[]byte,
	db.ID,
) (bool, bool, []byte, error) {
	return true, true, []byte{0}, nil
}

func (r *locationSelectionReader) FindAnswer(
	_ []byte,
	_ []byte,
	_ string,
	_ uint16,
	locID db.ID,
	_ *dns.Msg,
	_ int,
) (bool, int) {
	r.probed = append(r.probed, locID)
	return false, r.rcodes[string(locID)]
}

type selectionPlugin struct {
	request   *pluginmap.Request
	locations []pluginmap.Location
	err       error
}

func (p *selectionPlugin) Lookup(_ context.Context, request *pluginmap.Request) ([]pluginmap.Location, error) {
	p.request = request
	return p.locations, p.err
}

type testPluginRegistry map[string]pluginmap.Plugin

func (r testPluginRegistry) Get(name string) pluginmap.Plugin {
	return r[name]
}

func (r testPluginRegistry) Empty() bool {
	return len(r) == 0
}

type countingStats struct {
	stats.DummyStats
	counters map[string]int
}

func (s *countingStats) IncrementCounter(key string) {
	s.counters[key]++
}

func testID(name string) db.ID {
	nameLen := byte(len(name)) //nolint:gosec // test names are bounded
	return append(db.ID{0xff, nameLen}, name...)
}

func testLocation(name string, scope uint8) db.Location {
	return db.Location{MapID: testID("test"), LocID: testID(name), Mask: scope}
}

func TestResolvePluginLocationsPassesRequestAndPreservesOrder(t *testing.T) {
	plugin := &selectionPlugin{locations: []pluginmap.Location{
		{Name: "primary", Scope: 24},
		{Name: "@default", Scope: 16},
	}}
	handler := &FBDNSDB{plugins: testPluginRegistry{"test": plugin}}
	ecs := &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        1,
		SourceNetmask: 24,
		Address:       []byte{192, 0, 2, 0},
	}

	locations, err := handler.resolvePluginLocations(
		t.Context(), db.PluginMap{Plugin: "test", Args: "k1=v1;k2=v2"}, ecs, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, []db.Location{
		testLocation("primary", 24),
		testLocation("@default", 16),
	}, locations)
	require.Equal(t, "k1=v1;k2=v2", plugin.request.Args)
	require.Equal(t, "192.0.2.1", plugin.request.ResolverIP)

	plugin.request.ECS.SourceScope = 12
	require.Zero(t, ecs.SourceScope)
}

func TestResolvePluginLocationsFallsThrough(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	testCases := []struct {
		name      string
		plugins   pluginProvider
		pluginMap db.PluginMap
		wantErr   error
	}{
		{name: "no registry configured"},
		{name: "no plugins enabled", plugins: testPluginRegistry{}},
		{
			name:      "record names a plugin this server did not enable",
			plugins:   testPluginRegistry{"other": &selectionPlugin{}},
			pluginMap: db.PluginMap{Plugin: "test"},
		},
		{
			name:      "plugin returns an error",
			plugins:   testPluginRegistry{"test": &selectionPlugin{err: lookupErr}},
			pluginMap: db.PluginMap{Plugin: "test"},
			wantErr:   lookupErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &FBDNSDB{plugins: tc.plugins}
			locations, err := handler.resolvePluginLocations(
				t.Context(), tc.pluginMap, nil, "192.0.2.1")
			require.Empty(t, locations)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolvePluginLocationsRejectsInvalidNames(t *testing.T) {
	plugin := &selectionPlugin{locations: []pluginmap.Location{{Name: "x"}, {Name: "valid"}}}
	handler := &FBDNSDB{plugins: testPluginRegistry{"test": plugin}}

	locations, err := handler.resolvePluginLocations(
		t.Context(), db.PluginMap{Plugin: "test"}, nil, "192.0.2.1")
	require.Error(t, err)
	require.Empty(t, locations)
}

func TestSelectLocationRetriesNXDOMAIN(t *testing.T) {
	primary := testLocation("primary", 24)
	fallback := testLocation("@default", 16)
	plugin := &selectionPlugin{locations: []pluginmap.Location{
		{Name: "primary", Scope: 24},
		{Name: "@default", Scope: 16},
	}}
	reader := &locationSelectionReader{
		pluginMap:      db.PluginMap{Plugin: "test"},
		pluginMapFound: true,
		rcodes: map[string]int{
			string(primary.LocID):  dns.RcodeNameError,
			string(fallback.LocID): dns.RcodeSuccess,
		},
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{stats: counters, plugins: testPluginRegistry{"test": plugin}}
	ecs := &dns.EDNS0_SUBNET{SourceNetmask: 24}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, ecs, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, fallback, *location)
	require.Equal(t, uint8(16), ecs.SourceScope)
	require.Equal(t, []db.ID{primary.LocID, fallback.LocID}, reader.probed)
	require.Zero(t, reader.legacyCalls)
	require.Equal(t, 1, counters.counters["DNS_location.plugin.selected"])
	require.Zero(t, counters.counters["DNS_location.map.selected"])
}

func TestSelectLocationKeepsNODATA(t *testing.T) {
	primary := testLocation("primary", 24)
	plugin := &selectionPlugin{locations: []pluginmap.Location{
		{Name: "primary", Scope: 24},
		{Name: "@default", Scope: 16},
	}}
	reader := &locationSelectionReader{
		pluginMap:      db.PluginMap{Plugin: "test"},
		pluginMapFound: true,
		rcodes:         map[string]int{string(primary.LocID): dns.RcodeSuccess},
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{stats: counters, plugins: testPluginRegistry{"test": plugin}}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, nil, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, primary, *location)
	require.Equal(t, []db.ID{primary.LocID}, reader.probed)
	require.Zero(t, reader.legacyCalls)
	require.Equal(t, 1, counters.counters["DNS_location.plugin.selected"])
	require.Zero(t, counters.counters["DNS_location.map.selected"])
}

func TestSelectLocationFallsBackToMaps(t *testing.T) {
	primary := testLocation("primary", 24)
	fallback := testLocation("@default", 16)
	legacy := testLocation("legacy", 8)
	plugin := &selectionPlugin{locations: []pluginmap.Location{
		{Name: "primary", Scope: 24},
		{Name: "@default", Scope: 16},
	}}
	reader := &locationSelectionReader{
		pluginMap:      db.PluginMap{Plugin: "test"},
		pluginMapFound: true,
		legacyLocation: &legacy,
		rcodes: map[string]int{
			string(primary.LocID):  dns.RcodeNameError,
			string(fallback.LocID): dns.RcodeNameError,
		},
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{stats: counters, plugins: testPluginRegistry{"test": plugin}}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, nil, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, legacy, *location)
	require.Equal(t, 1, reader.legacyCalls)
	require.Equal(t, 1, counters.counters["DNS_location.map.selected"])
	require.Zero(t, counters.counters["DNS_location.plugin.selected"])
}

func TestSelectLocationCountsPluginErrorAndFallsBack(t *testing.T) {
	lookupErr := errors.New("plugin failed")
	legacy := testLocation("legacy", 8)
	reader := &locationSelectionReader{
		pluginMap:      db.PluginMap{Plugin: "test"},
		pluginMapFound: true,
		legacyLocation: &legacy,
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{
		stats:   counters,
		plugins: testPluginRegistry{"test": &selectionPlugin{err: lookupErr}},
	}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, nil, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, legacy, *location)
	require.Equal(t, 1, counters.counters["DNS_location.plugin.error"])
	require.Equal(t, 1, counters.counters["DNS_location.map.selected"])
}

func TestSelectLocationReturnsPluginMapError(t *testing.T) {
	lookupErr := errors.New("database failed")
	legacy := testLocation("legacy", 8)
	reader := &locationSelectionReader{
		pluginMapErr:   lookupErr,
		legacyLocation: &legacy,
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{
		stats:   counters,
		plugins: testPluginRegistry{"test": &selectionPlugin{}},
	}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, nil, "192.0.2.1")
	require.ErrorIs(t, err, lookupErr)
	require.Nil(t, location)
	require.Zero(t, reader.legacyCalls)
	require.Zero(t, counters.counters["DNS_location.plugin.error"])
	require.Zero(t, counters.counters["DNS_location.map.selected"])
}

func TestSelectLocationReturnsFindAnswerError(t *testing.T) {
	primary := testLocation("primary", 24)
	legacy := testLocation("legacy", 8)
	plugin := &selectionPlugin{locations: []pluginmap.Location{{Name: "primary", Scope: 24}}}
	reader := &locationSelectionReader{
		pluginMap:      db.PluginMap{Plugin: "test"},
		pluginMapFound: true,
		legacyLocation: &legacy,
		rcodes:         map[string]int{string(primary.LocID): dns.RcodeServerFailure},
	}
	counters := &countingStats{counters: map[string]int{}}
	handler := &FBDNSDB{stats: counters, plugins: testPluginRegistry{"test": plugin}}

	location, err := handler.selectLocation(
		t.Context(), reader, []byte{0}, "example.com.", dns.TypeAAAA, nil, "192.0.2.1")
	require.Error(t, err)
	require.Nil(t, location)
	require.Zero(t, reader.legacyCalls)
	require.Zero(t, counters.counters["DNS_location.plugin.selected"])
	require.Zero(t, counters.counters["DNS_location.map.selected"])
}
