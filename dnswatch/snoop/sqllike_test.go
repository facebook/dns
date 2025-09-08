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

	"github.com/stretchr/testify/require"
)

func TestSqllike(t *testing.T) {
	s := &SqllikeData{
		Where:   "COL2=2,TEST<30,TEST>10",
		Orderby: "-COL2,TEST",
		Groupby: "COL2",
	}
	loadMaps := make([]map[string]interface{}, 0)
	for i := range 40 {
		lMap := map[string]interface{}{
			"LATENCY": i * 3,
			"TEST":    i,
			"COL2":    i % 5,
		}
		loadMaps = append(loadMaps, lMap)
	}
	s.Setup(loadMaps)
	s.SolveWhere()
	s.SolveOrderby()
	rows, cols := s.Df.Dims()
	require.Equal(t, 4, rows)
	require.Equal(t, 3, cols)

	val, err := s.Df.Elem(0, 0).Int()
	require.Nil(t, err)
	require.Equal(t, 2, val)
	val, err = s.Df.Elem(0, 1).Int()
	require.Nil(t, err)
	require.Equal(t, 36, val)
	val, err = s.Df.Elem(0, 2).Int()
	require.Nil(t, err)
	require.Equal(t, 12, val)
	val, err = s.Df.Elem(1, 0).Int()
	require.Nil(t, err)
	require.Equal(t, 2, val)
	val, err = s.Df.Elem(1, 1).Int()
	require.Nil(t, err)
	require.Equal(t, 51, val)
	val, err = s.Df.Elem(1, 2).Int()
	require.Nil(t, err)
	require.Equal(t, 17, val)

	s.SolveGroupby()
	// COL2
	val, err = s.Df.Elem(0, 0).Int()
	require.Nil(t, err)
	require.Equal(t, 2, val)
	// LATENCY_COUNT
	val, err = s.Df.Elem(0, 1).Int()
	require.Nil(t, err)
	require.Equal(t, 4, val)
	// LATENCY_MAX
	val, err = s.Df.Elem(0, 2).Int()
	require.Nil(t, err)
	require.Equal(t, 81, val)
	// LATENCY_MEAN
	val, err = s.Df.Elem(0, 3).Int()
	require.Nil(t, err)
	require.Equal(t, 58, val)
	// LATENCY_MEDIAN
	val, err = s.Df.Elem(0, 4).Int()
	require.Nil(t, err)
	require.Equal(t, 58, val)
	// LATENCY_MIN
	val, err = s.Df.Elem(0, 5).Int()
	require.Nil(t, err)
	require.Equal(t, 36, val)
}
