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

package cdb

import (
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/facebook/dns/dnsrocks/testutils"

	"github.com/stretchr/testify/require"
)

func getData(name string) string {
	return testutils.FixturePath("../../testdata/data", name)
}

func TestCreateCDB(t *testing.T) {
	assert := require.New(t)
	tests := []struct {
		input          string
		expectedErr    error
		cdbShouldExist bool
	}{
		{getData("data.nets"), nil, true},
		{getData("data.empty"), nil, true},
		// can't parse cdb as valid data
		{getData("data.cdb"), fmt.Errorf("%w", errors.New("")), false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "cdb-test")
			cdb := path.Join(tmpDir, "data.cdb")
			require.Nil(t, err)
			defer os.RemoveAll(tmpDir)
			_, err = CreateCDB(test.input, cdb, nil)
			assert.IsType(test.expectedErr, err)

			if test.cdbShouldExist {
				assert.FileExists(cdb)
			} else {
				_, err := os.Stat(cdb)
				assert.True(os.IsNotExist(err))
			}
		})
	}
}
