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

package dbdiff

import (
	"errors"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
)

// Op represents one of the available diff operations, "+" and "-".
type Op string

const (
	// AddOp is the operator to add
	AddOp Op = "+"
	// DelOp is the operator to delete
	DelOp Op = "-"
)

// Valid returns whether the op is one of the permitted operations
func (op Op) Valid() bool {
	switch op {
	default:
		return false
	case AddOp:
	case DelOp:
	}
	return true
}

// ErrShortInput is an error returned when the input data is too short
var ErrShortInput = errors.New("input too short")

// ErrBadOp is an error returned when the operation is not recognised
var ErrBadOp = errors.New("unrecognised operation")

func decodeOp(line []byte) (op Op, err error) {
	if len(line) < 1 {
		return op, ErrShortInput
	}
	op = Op(line[:1])
	if !op.Valid() {
		err = ErrBadOp
	}
	return
}

type Entry struct {
	Op      Op                  // the operation
	Bytes   []byte              // argument of the operation - accessible via Arg() or Bytes()
	Records []dnsdata.MapRecord // vector of map records
}

// Parse analyzes the provided input which must be a single entry
// (line) of a diff, and populates Op and Bytes fields accordingly.
// It may return ErrShortInput or ErrBadOp if the input is incomplete
// or isn't recognised.
func (d *Entry) Parse(s string) error {
	return d.ParseBytes([]byte(s))
}

// ParseBytes does the same as Parse() but takes in the []byte type.
func (d *Entry) ParseBytes(b []byte) error {
	op, err := decodeOp(b)
	if err != nil {
		return err
	}
	d.Op = op
	d.Bytes = b[1:] // BUG should copy?
	return nil
}

// Arg returns the operation argument (the line without the leading "+" or "-")
func (d *Entry) Arg() string {
	return string(d.Bytes)
}

// SetArg replaces the operation argument
func (d *Entry) SetArg(s string) {
	d.Bytes = []byte(s)
}

// String is a reverse of Parse()
func (d *Entry) String() string {
	return string(d.Op) + string(d.Bytes)
}

func (d *Entry) Convert(codec *dnsdata.Codec) error {
	v, err := codec.ConvertLn(d.Bytes)
	if err != nil {
		return err
	}
	d.Records = v
	return nil
}
