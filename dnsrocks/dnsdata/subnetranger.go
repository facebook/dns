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
	"sync"

	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"
)

const (
	// InitLocationCount is the initial locationCount for NewRearranger
	InitLocationCount = 2
)

// SubnetRanger represents an aggregate structure responsible for building
// the auxiliary structures for the subnet-based location lookup
type SubnetRanger struct {
	enabled bool
	arng    map[Lmap]*Rearranger
}

// Enable enables the functionality
func (r *SubnetRanger) Enable() {
	r.arng = make(map[Lmap]*Rearranger)
	r.enabled = true
}

func (r *SubnetRanger) getRearranger(lmap Lmap) *Rearranger {
	a := r.arng[lmap]
	if a == nil {
		a = NewRearranger(InitLocationCount)
		r.arng[lmap] = a
	}
	return a
}

func (r *SubnetRanger) addSubnet(s *Rnet) error {
	if !r.enabled {
		return nil
	}
	a := r.getRearranger(s.lmap)
	return a.AddLocation(s.ipnet, s.lo)
}

// MarshalMap implements MapMarshaler
func (r *SubnetRanger) MarshalMap() (result []MapRecord, err error) {
	if !r.enabled {
		return nil, nil
	}

	perMapRecords := make(chan []MapRecord, 1)

	group := &errgroup.Group{}

	for lmap, a := range r.arng {
		// closures
		localLmap := lmap
		rearranger := a

		group.Go(
			func() error {
				var m []MapRecord
				for _, pt := range rearranger.Rearrange() {
					r := &Rrangepoint{lmap: localLmap, pt: pt}
					mr, err := r.MarshalMap()
					if err != nil {
						return err
					}
					m = append(m, mr...)
				}

				perMapRecords <- m
				return nil
			})
	}

	go func() {
		err := group.Wait()
		if err != nil {
			glog.Errorf("%v", err)
		}
		close(perMapRecords)
	}()

	for data := range perMapRecords {
		result = append(result, data...)
	}

	err = group.Wait()

	return
}

// OpenScanner creates Scanner which allows lazy reading of subnet range records in a text form
func (r *SubnetRanger) OpenScanner() (s *SubnetRangerScanner) {
	if !r.enabled {
		return nil
	}

	chunks := make(chan []string, len(r.arng))

	group := &errgroup.Group{}

	for lmap, a := range r.arng {
		// closures
		localLmap := lmap
		rearranger := a

		group.Go(
			func() error {
				maxChunkSize := 100
				chunk := make([]string, 0, maxChunkSize)

				for _, pt := range rearranger.Rearrange() {
					r := &Rrangepoint{lmap: localLmap, pt: pt}
					b, err := r.MarshalText()
					if err != nil {
						return err
					}

					chunk = append(chunk, string(b))

					if len(chunk) == maxChunkSize {
						chunks <- chunk
						chunk = make([]string, 0, maxChunkSize)
					}
				}

				if len(chunk) > 0 {
					chunks <- chunk
				}

				return nil
			})
	}

	s = &SubnetRangerScanner{chunks: chunks}

	go func() {
		err := group.Wait()
		s.SetError(err)
		close(chunks)
	}()

	return
}

// SubnetRangerScanner allows lazy reading of subnet range records in a text form
type SubnetRangerScanner struct {
	chunks       chan []string
	currentChunk []string
	index        int
	finished     bool
	err          error
	currentLine  string
	sync.RWMutex
}

// Scan pushes scanner to new line if one exists. In this case result will be
// true and that line could be read with Text() method
func (s *SubnetRangerScanner) Scan() bool {
	if s.Err() != nil || s.finished {
		return false
	}

	if s.currentChunk == nil || s.index == len(s.currentChunk) {
		rangepoint, ok := <-s.chunks
		if !ok {
			s.finished = true
			return false
		}
		s.currentChunk = rangepoint
		s.index = 0
	}

	s.currentLine = s.currentChunk[s.index]
	s.index++

	return true
}

// Text returns current line
func (s *SubnetRangerScanner) Text() string {
	return s.currentLine
}

// SetError sets error which happened during the scan (if any)
func (s *SubnetRangerScanner) SetError(err error) {
	s.Lock()
	defer s.Unlock()
	s.err = err
}

// Err returns error if Scanner met any issues, nil otherwise
func (s *SubnetRangerScanner) Err() error {
	s.Lock()
	defer s.Unlock()
	return s.err
}
