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
	"time"

	log "github.com/sirupsen/logrus"
)

const bufferSizeLimit = 512 // bytes.Buffer grows by 512

// PreprocReader is a streaming io.Reader version of preprocessor
type PreprocReader struct {
	codec       *Codec
	scanner     *bufio.Scanner
	currentLine string
	err         error

	buffer     bytes.Buffer
	existsData bool // if we read all from scanner

	accumulatorScanner *AccumulatorScanner

	wroteAccSt time.Time
}

// NewPreprocReader creates reader that processes input line by line and filters/changes it according to codec settings
func NewPreprocReader(r io.Reader, c *Codec) *PreprocReader {
	return &PreprocReader{
		codec:      c,
		scanner:    bufio.NewScanner(r),
		existsData: true,
	}
}

// empty line or comment
func isIgnored(line []byte) bool {
	if len(line) < 1 || decodeRtype(line) == prefixComment {
		return true
	}
	return false
}

func writeLine(w io.Writer, line []byte) error {
	if _, err := w.Write(line); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// Scan pushes Reader to a new line, similar to bufio.Scanner
func (p *PreprocReader) Scan() bool {
	if p.Err() != nil {
		return false
	}

	for p.scanner.Scan() {
		line := p.scanner.Bytes()
		if isIgnored(line) {
			continue
		}

		rtype := decodeRtype(line)
		switch rtype {
		case prefixNet:
			// decode line so codec's accumulator is populated and we can expand networks later on
			_, err := p.codec.DecodeLn(line)
			if err != nil {
				p.err = fmt.Errorf("error decoding %s: %w", string(line), err)
				return false
			}
			if p.codec.NoRnetOutput {
				continue
			}
		case prefixSOA:
			// normalize SOA text representation to fix serial
			r, err := p.codec.DecodeLn(line)
			if err != nil {
				p.err = fmt.Errorf("error decoding %s: %w", string(line), err)
				return false
			}
			normalized, err := r.MarshalText()
			if err != nil {
				p.err = fmt.Errorf("error normalising %s: %w", string(line), err)
				return false
			}
			p.currentLine = string(normalized)
			// don't write original line
			return true
		}

		p.currentLine = string(line)
		return true
	}

	// we exhausted scanner, which means we read all of original input
	// check if scanner encountered errors, report if so
	if err := p.scanner.Err(); err != nil {
		p.err = err
		return false
	}

	if p.accumulatorScanner == nil {
		p.wroteAccSt = time.Now()

		var err error
		p.accumulatorScanner, err = p.codec.Acc.OpenScanner()
		if err != nil {
			p.err = fmt.Errorf("error encoding Acc: %w", err)
			return false
		}
	}

	result := p.accumulatorScanner.Scan()

	if !result {
		log.Debugf("Write Accum finished in: %v", time.Since(p.wroteAccSt))
	}

	return result
}

// Text returns current reader line
func (p *PreprocReader) Text() string {
	if p.accumulatorScanner != nil {
		return p.accumulatorScanner.Text()
	}

	return p.currentLine
}

// Err returns error if reader failed
func (p *PreprocReader) Err() error {
	if p.accumulatorScanner != nil {
		return p.accumulatorScanner.Err()
	}

	return p.err
}

// Read implements io.Reader interface
func (p *PreprocReader) Read(buf []byte) (int, error) {
	if p.Err() != nil {
		return 0, p.Err()
	}

	for p.existsData && p.buffer.Len() < bufferSizeLimit {
		if p.existsData = p.Scan(); !p.existsData {
			break
		}

		line := p.Text()
		if err := writeLine(&p.buffer, []byte(line)); err != nil {
			return 0, err
		}
	}

	return p.buffer.Read(buf)
}

// Preprocess reads textual data from r and writes an equivalent set - which includes the aggregated records - to w.
func (c *Codec) Preprocess(r io.Reader, w io.Writer) error {
	wrapped := NewPreprocReader(r, c)
	_, err := io.Copy(w, wrapped)
	return err
}
