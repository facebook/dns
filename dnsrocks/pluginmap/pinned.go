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

package pluginmap

import (
	"context"
	"fmt"

	"github.com/facebook/dns/dnsrocks/dnsdata/pluginargs"
)

const pinnedLocationArg = "loc"

// pinnedPlugin returns the logical location named by the Plugin Map record.
// It provides a dependency-free example of the plugin boundary.
type pinnedPlugin struct{}

func init() { RegisterFactory("pinned", newPinnedPlugin) }

func newPinnedPlugin() (Plugin, error) { return &pinnedPlugin{}, nil }

// Lookup implements Plugin.
func (p *pinnedPlugin) Lookup(_ context.Context, req *Request) ([]Location, error) {
	location, found := pluginargs.Get(req.Args, pinnedLocationArg)
	if !found {
		return nil, fmt.Errorf("plugin map record is missing %s", pinnedLocationArg)
	}
	return []Location{{Name: location}}, nil
}
