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

// Package pluginmap defines location plugins selected by Plugin Map records.
package pluginmap

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/miekg/dns"
)

// Plugin resolves candidate locations for a DNS query.
type Plugin interface {
	// Lookup returns candidate locations in preference order. An empty result
	// means DNSRocks should continue with its map-based location lookup.
	Lookup(ctx context.Context, req *Request) ([]Location, error)
}

// Location is a logical location returned by a Plugin. DNSRocks owns the
// conversion from Name to its database key encoding.
type Location struct {
	Name  string
	Scope uint8
}

// Request carries the query information available to a plugin.
type Request struct {
	// Args is the raw argument string from the Plugin Map record (everything
	// after the plugin name). Its shape is the dnsdata/pluginargs grammar,
	// already validated by the time a record reaches the database; which keys
	// are meaningful, and what their values mean, is up to the plugin.
	Args string
	// ResolverIP is the IP address of the resolver, as received.
	ResolverIP string
	// ECS is the EDNS Client Subnet option, or nil if not present. It is a copy;
	// plugins report response scope through Location instead of mutating it.
	ECS *dns.EDNS0_SUBNET
}

// Factory constructs a plugin. A factory runs only for a plugin this server
// was asked to enable, so it is free to acquire resources the plugin needs.
type Factory func() (Plugin, error)

// factories contains the plugins linked into this binary.
var factories = map[string]Factory{}

// RegisterFactory makes a plugin available to NewRegistry. A plugin registers
// from an init(), so which plugins exist is settled by what a binary links.
func RegisterFactory(name string, factory Factory) {
	if err := validateName(name); err != nil {
		panic(fmt.Sprintf("pluginmap: invalid plugin name: %v", err))
	}
	if _, taken := factories[name]; taken {
		panic("pluginmap: plugin already registered: " + name)
	}
	factories[name] = factory
}

// Registered names every plugin this binary can construct, sorted.
func Registered() []string {
	return slices.Sorted(maps.Keys(factories))
}

// Registry holds the plugins constructed for this server, keyed by the name
// that Plugin Map records use to select them.
type Registry struct {
	plugins map[string]Plugin
}

// NewRegistry constructs the registered plugins named in enabled.
func NewRegistry(enabled []string) (*Registry, error) {
	return newRegistry(enabled, factories)
}

func newRegistry(enabled []string, available map[string]Factory) (*Registry, error) {
	plugins := make(map[string]Plugin, len(enabled))
	for _, name := range enabled {
		if _, done := plugins[name]; done {
			continue
		}
		if err := validateName(name); err != nil {
			return nil, fmt.Errorf("invalid plugin name: %w", err)
		}
		factory, known := available[name]
		if !known {
			return nil, fmt.Errorf("no such plugin: %q; available: %s",
				name, strings.Join(slices.Sorted(maps.Keys(available)), ", "))
		}
		plugin, err := factory()
		if err != nil {
			return nil, fmt.Errorf("constructing plugin %q: %w", name, err)
		}
		plugins[name] = plugin
	}
	return &Registry{plugins: plugins}, nil
}

// Get returns the named plugin, or nil if this server did not construct it.
func (r *Registry) Get(name string) Plugin {
	if r == nil {
		return nil
	}
	return r.plugins[name]
}

// Empty reports whether the registry holds no plugins.
func (r *Registry) Empty() bool {
	return r == nil || len(r.plugins) == 0
}

func validateName(name string) error {
	if len(name) < 2 || len(name) > math.MaxUint8 {
		return fmt.Errorf("name %q is %d bytes, outside the 2 to %d byte range",
			name, len(name), math.MaxUint8)
	}
	return nil
}
