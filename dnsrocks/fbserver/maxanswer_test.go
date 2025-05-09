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

package fbserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewMaxAnswerHandlerNegativeNumber tests that we return an error when
// given a negative number
func TestNewMaxAnswerHandlerNegativeNumber(t *testing.T) {
	_, err := newMaxAnswerHandler(-2)
	require.NotNil(t, err)
}

// TestNewMaxAnswerHandlerZeroNumber tests that we return an error when
// given 0.
func TestNewMaxAnswerHandlerZeroNumber(t *testing.T) {
	_, err := newMaxAnswerHandler(0)
	require.NotNil(t, err)
}

// TestNewMaxAnswerHandlerPositiveNumber tests that we do not return an error
// when given a positive number
func TestNewMaxAnswerHandlerPositiveNumber(t *testing.T) {
	_, err := newMaxAnswerHandler(1)
	require.Nil(t, err)
}
