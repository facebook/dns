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

package db

import (
	"bytes"
	"fmt"
	"math"
)

// pluginMapKeyElement is the map type under which Plugin Map records are stored.
var pluginMapKeyElement = []byte{0, 'P'}

// PluginMap is the plugin name and arguments encoded by a Plugin Map record.
type PluginMap struct {
	Plugin string
	Args   string
}

// FindPluginMap returns the Plugin Map record that applies to qname. The
// boolean reports whether a record was found.
func (r *DataReader) FindPluginMap(qname []byte) (PluginMap, bool, error) {
	data, err := r.db.dbi.FindMap(qname, pluginMapKeyElement, r.context)
	if err != nil {
		return PluginMap{}, false, fmt.Errorf("looking up Plugin Map record: %w", err)
	}
	if data == nil {
		return PluginMap{}, false, nil
	}

	name, args := parsePluginValue(data)
	return PluginMap{Plugin: string(name), Args: string(args)}, true, nil
}

// NewLocation encodes logical map and location names as a database Location.
func NewLocation(mapName, locationName string, mask uint8) (Location, error) {
	mapID, err := encodeID(mapName)
	if err != nil {
		return Location{}, fmt.Errorf("encoding map name: %w", err)
	}
	locationID, err := encodeID(locationName)
	if err != nil {
		return Location{}, fmt.Errorf("encoding location name: %w", err)
	}
	return Location{MapID: mapID, LocID: locationID, Mask: mask}, nil
}

func parsePluginValue(data []byte) (name, args []byte) {
	name, args, _ = bytes.Cut(data, []byte{0})
	return name, args
}

func encodeID(name string) (ID, error) {
	if len(name) < 2 || len(name) > math.MaxUint8 {
		return nil, fmt.Errorf("name %q is %d bytes, outside the 2 to %d encodable range",
			name, len(name), math.MaxUint8)
	}
	if len(name) == 2 {
		return ID(name), nil
	}

	id := make(ID, 2+len(name))
	id[0] = 0xff
	id[1] = byte(len(name)) //nolint:gosec // length is bounded above
	copy(id[2:], name)
	return id, nil
}
