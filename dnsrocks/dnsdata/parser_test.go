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

package dnsdata

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var dataSetSize = 5000

// genRecords generates record lines from i
func genRecords(i int) [][]byte {
	return [][]byte{
		[]byte(fmt.Sprintf("=test.record.%d,192.168.0.%d,%d", i, i%254, 8000+i)),
		[]byte(fmt.Sprintf("=test.record.%d,fc0a:14f5:dead:beef:1::37%d,%d", i, i%9, 7000+i)),
		[]byte(fmt.Sprintf("Zt%d.org,a.ns.t%d.org,dns.t%d.org,%d,%d,1801,604801,%d,119,,", i, i, i, 100+i, 7000+i, i%200)),
	}
}

// genData generates test data along with verification data, using error-safe linear fashion.
// Used to double-check our parser for races and data corruption.
func genData(size int) ([]byte, []MapRecord) {
	records := [][]byte{}
	mapRecords := []MapRecord{}
	c := new(Codec)
	c.Serial = testSerial
	for i := 0; i < size; i++ {
		for _, r := range genRecords(i) {
			records = append(records, r)
			mr, err := c.ConvertLn(r)
			if err != nil {
				panic(err)
			}
			mapRecords = append(
				mapRecords,
				mr...,
			)
		}
	}
	extra, err := c.Acc.MarshalMap()
	if err != nil {
		panic(err)
	}
	mapRecords = append(mapRecords, extra...)

	extra, err = c.Features.MarshalMap()
	if err != nil {
		panic(err)
	}
	mapRecords = append(mapRecords, extra...)

	return bytes.Join(records, []byte("\n")), mapRecords
}

func getDataSet() []byte {
	dataset := []byte{}
	// we use test data from codectests defined in data_test.go
	for _, test := range codectests {
		dataset = append(dataset, test.in...)
		dataset = append(dataset, []byte("\n")...)
	}
	return dataset
}

func getExpected(useV2data bool) []MapRecord {
	expected := []MapRecord{}
	// we use test data from codectests defined in data_test.go
	for _, test := range codectests {
		if useV2data && test.outV2 != nil {
			expected = append(expected, test.outV2...)
		} else {
			expected = append(expected, test.out...)
		}
	}
	// those are generated from accumulator and aggregate all subnets
	extra := []MapRecord{
		// {0, '/'}: global prefix bitmap
		{
			Key:   []byte{0x0, 0x2f}, // 0, '/'
			Value: []byte{0x80, 0x78, 0x77, 0x60, 0x20},
		},
		// {0, '4'}: ipv4 prefix bitmap
		// prefix length 96₁₀ (60₁₆) is equivalent to /0 in ipv4-mapped ipv6 address)
		{
			Key:   []byte{0x0, 0x34}, // 0, '4'
			Value: []byte{0x80, 0x78, 0x77, 0x60},
		},
		// {0, '6'}: ipv6 prefix bitmap
		{
			Key:   []byte{0x0, 0x36}, // 0, '6'
			Value: []byte{0x20},
		},
	}
	expected = append(expected, extra...)

	if useV2data {
		versionRecord := MapRecord{
			Key:   []byte("\x00o_features"),
			Value: []byte{0x02, 0x00, 0x00, 0x00},
		}

		expected = append(expected, versionRecord)
	} else {
		versionRecord := MapRecord{
			Key:   []byte("\x00o_features"),
			Value: []byte{0x01, 0x00, 0x00, 0x00},
		}

		expected = append(expected, versionRecord)
	}

	return expected
}

func TestParseLinearBroken(t *testing.T) {
	dataset := []byte(
		"some\nrandom\nstring\n",
	)
	r := bytes.NewReader(dataset)
	_, err := Parse(r, &Codec{Serial: testSerial}, 1)
	require.NotNil(t, err, "must err on bad input")
}

func TestParseLinear(t *testing.T) {
	dataset := getDataSet()
	r := bytes.NewReader(dataset)
	results, err := Parse(r, &Codec{Serial: testSerial}, 1)
	require.Nil(t, err)
	expected := getExpected(false)
	require.Equal(t, expected, results, "data correctly parsed")
}

func TestParseLinearV2(t *testing.T) {
	dataset := getDataSet()
	r := bytes.NewReader(dataset)
	codec := &Codec{Serial: testSerial}
	codec.Features.UseV2Keys = true
	results, err := Parse(r, codec, 1)
	require.Nil(t, err)
	expected := getExpected(true)
	require.Equal(t, expected, results, "data correctly parsed")
}

func TestParseLinearGen(t *testing.T) {
	dataset, expected := genData(dataSetSize)
	r := bytes.NewReader(dataset)
	results, err := Parse(r, &Codec{Serial: testSerial}, 1)
	require.Nil(t, err)
	require.Equal(t, expected, results, "data correctly parsed")
}

func TestParseParallelBroken(t *testing.T) {
	dataset := []byte(
		"some random string\n",
	)
	r := bytes.NewReader(dataset)
	_, err := Parse(r, &Codec{Serial: testSerial}, 0)
	require.NotNil(t, err, "must err on bad input")
}

func TestParseParallelBrokenTricky(t *testing.T) {
	dataset := []byte(
		"some\nrandom\nstring\noh\nno\n",
	)
	for i := 0; i < 10; i++ {
		dataset = append(dataset, dataset...)
	}
	r := bytes.NewReader(dataset)
	// with just 2 workers and lots of broken records we would hang
	// if file reading was blocking
	_, err := Parse(r, &Codec{Serial: testSerial}, 2)
	require.NotNil(t, err, "must err on bad input")
}

func TestParseParallel(t *testing.T) {
	dataset := getDataSet()
	r := bytes.NewReader(dataset)
	results, err := Parse(r, &Codec{Serial: testSerial}, 0)
	require.Nil(t, err)
	expected := getExpected(false)
	require.ElementsMatch(t, expected, results, "data correctly parsed")
}

func TestParseParallelV2(t *testing.T) {
	dataset := getDataSet()
	r := bytes.NewReader(dataset)
	codec := &Codec{Serial: testSerial}
	codec.Features.UseV2Keys = true
	results, err := Parse(r, codec, 0)
	require.Nil(t, err)
	expected := getExpected(true)
	require.ElementsMatch(t, expected, results, "data correctly parsed")
}

func TestParseParallelGen(t *testing.T) {
	dataset, expected := genData(dataSetSize)
	r := bytes.NewReader(dataset)
	results, err := Parse(r, &Codec{Serial: testSerial}, 0)
	require.Nil(t, err)
	require.ElementsMatch(t, expected, results, "data correctly parsed")
}

func TestParseRecordsLinear(t *testing.T) {
	dataset := []byte{}
	dataset = append(dataset, []byte("8fb.com,c\001\n")...)
	dataset = append(dataset, []byte("%\\141b,192.168.1.0/24,c\001\n")...)
	dataset = append(dataset, []byte("+fb.com,1.2.3.4,3600,\\141bn")...)

	r := bytes.NewReader(dataset)
	results, err := parseRecords(r, &Codec{Serial: testSerial}, 1)
	require.Nil(t, err)

	require.Equal(t, 3, len(results), "unexpected number of results")
}

func parseRecords(r io.Reader, codec *Codec, workers int) ([]Record, error) {
	results := []Record{}
	workers, err := getWorkers(workers)
	if err != nil {
		return results, err
	}
	resultsChan := make(chan Record, workers)
	var g errgroup.Group
	g.Go(func() error {
		return ParseRecords(r, codec, resultsChan, workers)
	})
	for v := range resultsChan {
		results = append(results, v)
	}
	if err := g.Wait(); err != nil {
		return results, err
	}
	return results, nil
}

func BenchmarkParseLinear(b *testing.B) {
	dataset := getDataSet()
	for n := 0; n < b.N; n++ {
		r := bytes.NewReader(dataset)
		_, err := Parse(r, &Codec{Serial: testSerial}, 1)
		require.Nil(b, err)
	}
}

func BenchmarkParseParallel(b *testing.B) {
	dataset := getDataSet()
	for n := 0; n < b.N; n++ {
		r := bytes.NewReader(dataset)
		_, err := Parse(r, &Codec{Serial: testSerial}, 0)
		require.Nil(b, err)
	}
}

func BenchmarkParseLinearHuge(b *testing.B) {
	dataset, _ := genData(dataSetSize)
	for n := 0; n < b.N; n++ {
		r := bytes.NewReader(dataset)
		_, err := Parse(r, &Codec{Serial: testSerial}, 1)
		require.Nil(b, err)
	}
}

func BenchmarkParseParallelHuge(b *testing.B) {
	dataset, _ := genData(dataSetSize)
	for n := 0; n < b.N; n++ {
		r := bytes.NewReader(dataset)
		_, err := Parse(r, &Codec{Serial: testSerial}, 0)
		require.Nil(b, err)
	}
}
