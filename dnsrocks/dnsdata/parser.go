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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
)

func getWorkers(workers int) (int, error) {
	if workers < 0 {
		return workers, fmt.Errorf("bad number of workers %d", workers)
	}
	if workers == 0 {
		workers = runtime.NumCPU()
	}
	return workers, nil
}

// ParseStream parses data from io.Reader and returns results via results chan.
// Data is parsed in parallel when workers != 1.
func ParseStream(r io.Reader, codec *Codec, results chan<- []MapRecord, workers int) error {
	defer close(results)

	err := parse(
		r,
		func(line []byte) error {
			v, err := codec.ConvertLn(line)
			if err != nil {
				return fmt.Errorf("Conversion failed for line '%s': %w", line, err)
			}
			results <- v
			return nil
		},
		workers)

	if err != nil {
		return err
	}

	// Pack the accumulated state
	v, err := codec.Acc.MarshalMap()
	if err != nil {
		return fmt.Errorf("acc marshalling failed: %w", err)
	}
	results <- v

	// Pack the supported features
	v, err = codec.Features.MarshalMap()
	if err != nil {
		return fmt.Errorf("features marshalling failed: %w", err)
	}
	results <- v

	return nil
}

// ParseRecords parses data from io.Reader and returns results via results chan.
// Data is parsed in parallel when workers != 1.
func ParseRecords(r io.Reader, codec *Codec, results chan<- Record, workers int) error {
	defer close(results)

	return parse(
		r,
		func(line []byte) error {
			v, err := codec.DecodeLn(line)
			if err != nil {
				return fmt.Errorf("parsing failed for line '%s': %w", line, err)
			}
			results <- v
			return nil
		},
		workers)
}

func parse(r io.Reader, process func([]byte) error, workers int) error {
	workers, err := getWorkers(workers)
	if err != nil {
		return err
	}

	// Setup scanner to go over the file line by line
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)

	var g errgroup.Group
	c := make(chan []byte, workers*10) // 10 came out of experiments, allows some buffering

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for line := range c {
				if err := process(line); err != nil {
					return err
				}
			}
			return nil
		})
	}

	var wg sync.WaitGroup
	// Scan
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(c)
		for scanner.Scan() {
			line := bytes.TrimLeft(scanner.Bytes(), " ")
			if len(line) < 2 || bytes.HasPrefix(line, []byte("#")) {
				continue
			}
			newLine := make([]byte, len(line))
			copy(newLine, line)
			c <- newLine
		}
	}()

	if err := g.Wait(); err != nil {
		return err
	}
	wg.Wait()

	// Check we have reached EOF properly
	return scanner.Err()
}

// Parse parses data from Reader in parallel, wrapping ParseStream
func Parse(r io.Reader, codec *Codec, workers int) ([]MapRecord, error) {
	results := []MapRecord{}
	workers, err := getWorkers(workers)
	if err != nil {
		return results, err
	}
	resultsChan := make(chan []MapRecord, workers)
	var g errgroup.Group
	g.Go(func() error {
		return ParseStream(r, codec, resultsChan, workers)
	})
	for v := range resultsChan {
		results = append(results, v...)
	}
	if err := g.Wait(); err != nil {
		return results, err
	}
	return results, nil
}
