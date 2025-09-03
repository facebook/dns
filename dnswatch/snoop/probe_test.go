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

func TestCleanCmdline(t *testing.T) {
	var test [120]byte
	for i := range 120 {
		test[i] = byte('c')
	}
	// guaranteed from kprobe
	test[29] = 0
	test[59] = 0
	test[89] = 0
	test[119] = 0
	// first argument is zero terminated and has 10 chars
	test[10] = 0
	// add random chars, which usually appear from the kernel stack
	// after the zero terminated char string
	test[20] = 59
	test[21] = 3
	test[23] = 4
	test[27] = 7
	// second one is zero terminated and has 20 chars
	test[30+20] = 0
	test[30+20+3] = 100
	// third one is zero terminated and has 5 chars
	test[30*2+5] = 0
	test[30*2+5+4] = 1

	var good [120]byte
	cont := 0
	for range 10 {
		good[cont] = 'c'
		cont++
	}
	good[cont] = ' '
	cont++
	for range 20 {
		good[cont] = 'c'
		cont++
	}
	good[cont] = ' '
	cont++
	for range 5 {
		good[cont] = 'c'
		cont++
	}
	good[cont] = ' '
	cont++
	for range 29 {
		good[cont] = 'c'
		cont++
	}
	good[cont] = ' '
	cont++
	for ; cont < 120; cont++ {
		good[cont] = 0
	}
	//require.Equal(t, good, test)
	require.Equal(t, good, cleanCmdline(test))
}
