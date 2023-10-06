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

package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_toTime(t *testing.T) {
	w := toTime(11.0)

	require.Equal(t, time.Duration(11*time.Nanosecond), w)
}

func Test_toMicro(t *testing.T) {
	w := toMicro(2301.20)

	require.Equal(t, int(2), w)
}
