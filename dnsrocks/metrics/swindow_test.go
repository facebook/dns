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

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSlidewindow(t *testing.T) {
	sw, err := newSlidingWindow(time.Second * 2)
	require.NoError(t, err)
	sw.Add(1)
	sw.Add(1)
	sw.Add(2)
	time.Sleep(time.Second)
	samples := sw.Samples()
	require.Equal(t, 3, len(samples))
	time.Sleep(time.Second * 8)
	sw.Add(5)
	samples = sw.Samples()
	require.Equal(t, 1, len(samples))
}
