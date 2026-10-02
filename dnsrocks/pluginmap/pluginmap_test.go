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
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

type testPlugin struct{}

func (p *testPlugin) Lookup(context.Context, *Request) ([]Location, error) {
	return nil, nil
}

func TestRegistryGet(t *testing.T) {
	registry, err := newRegistry([]string{"wanted"}, map[string]Factory{
		"wanted":   func() (Plugin, error) { return &testPlugin{}, nil },
		"unwanted": func() (Plugin, error) { return &testPlugin{}, nil },
	})
	require.NoError(t, err)
	require.NotNil(t, registry.Get("wanted"))
	require.Nil(t, registry.Get("unwanted"))

	var nilRegistry *Registry
	require.Nil(t, nilRegistry.Get("wanted"))
	require.True(t, nilRegistry.Empty())
}

func TestNewRegistryConstructsOnlyEnabled(t *testing.T) {
	var builtWanted, builtUnwanted int
	available := map[string]Factory{
		"wanted": func() (Plugin, error) {
			builtWanted++
			return &testPlugin{}, nil
		},
		"unwanted": func() (Plugin, error) {
			builtUnwanted++
			return &testPlugin{}, nil
		},
	}

	registry, err := newRegistry([]string{"wanted"}, available)
	require.NoError(t, err)
	require.Equal(t, 1, builtWanted)
	require.Zero(t, builtUnwanted)
	require.NotNil(t, registry.Get("wanted"))
	require.Nil(t, registry.Get("unwanted"))
}

func TestNewRegistryBuildsEachNameOnce(t *testing.T) {
	var built int
	available := map[string]Factory{
		"pinned": func() (Plugin, error) {
			built++
			return &testPlugin{}, nil
		},
	}

	_, err := newRegistry([]string{"pinned", "pinned"}, available)
	require.NoError(t, err)
	require.Equal(t, 1, built)
}

func TestNewRegistryErrors(t *testing.T) {
	errFactory := errors.New("no backend")
	available := map[string]Factory{
		"pinned": func() (Plugin, error) { return &testPlugin{}, nil },
		"broken": func() (Plugin, error) { return nil, errFactory },
	}

	_, err := newRegistry([]string{"pinned", "typo"}, available)
	require.ErrorContains(t, err, "typo")
	require.ErrorContains(t, err, "available: broken, pinned")

	_, err = newRegistry([]string{"broken"}, available)
	require.ErrorIs(t, err, errFactory)
}

func TestNewRegistryUsesRegisteredFactories(t *testing.T) {
	registry, err := NewRegistry([]string{"pinned"})
	require.NoError(t, err)
	require.NotNil(t, registry.Get("pinned"))

	_, err = NewRegistry([]string{"nosuchplugin"})
	require.Error(t, err)
}

func TestRegisterFactoryRejectsDuplicates(t *testing.T) {
	require.Panics(t, func() { RegisterFactory("pinned", newPinnedPlugin) })
}

func TestRegisterFactoryRejectsInvalidName(t *testing.T) {
	require.Panics(t, func() { RegisterFactory("x", newPinnedPlugin) })
}

func TestPinnedPlugin(t *testing.T) {
	plugin, err := newPinnedPlugin()
	require.NoError(t, err)

	locations, err := plugin.Lookup(t.Context(), &Request{Args: "loc=abc1c01"})
	require.NoError(t, err)
	require.Equal(t, []Location{{Name: "abc1c01"}}, locations)

	_, err = plugin.Lookup(t.Context(), &Request{})
	require.ErrorContains(t, err, pinnedLocationArg)
}

func TestRegistered(t *testing.T) {
	registered := Registered()
	require.Contains(t, registered, "pinned")
	require.True(t, slices.IsSorted(registered))
}
