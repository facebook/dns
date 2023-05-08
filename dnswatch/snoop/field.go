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
	"fmt"
	"sort"
	"strings"
)

// FieldID label for field
type FieldID int

// Field constants
const (
	FieldPID = iota
	FieldPNAME
	FieldLAT
	FieldTYPE
	FieldQNAME
	FieldRCODE
	FieldRIP
	FieldQTIME
	FieldRTIME
	FieldTID
	FieldCMDLINE
	FieldQADDR
	FieldRADDR
)

// FieldMeta describes the data format for each field
type FieldMeta struct {
	Title  string
	Format string
}

// FieldToMeta maps fields to metadata
var FieldToMeta = map[FieldID]FieldMeta{
	FieldPID:     {"PID", "%-7v "},
	FieldPNAME:   {"PNAME", "%-15v "},
	FieldLAT:     {"LAT", "%-5v "},
	FieldTYPE:    {"TYPE", "%-5v "},
	FieldQNAME:   {"QNAME", "%-80v "},
	FieldRCODE:   {"RCODE", "%-8v "},
	FieldRIP:     {"RIP", "%-40v "},
	FieldQTIME:   {"QTIME", "%-16v "},
	FieldRTIME:   {"RTIME", "%-16v "},
	FieldTID:     {"TID", "%-7v "},
	FieldCMDLINE: {"CMDLINE", "%-120v "},
	FieldQADDR:   {"QADDR", "%-40v "},
	FieldRADDR:   {"RADDR", "%-40v "},
}

// AllFieldNames returns list of all acceptable field names
func AllFieldNames() []string {
	res := make([]string, 0, len(FieldToMeta))
	for _, fm := range FieldToMeta {
		res = append(res, fm.Title)
	}
	sort.Strings(res)
	return res
}

// FieldFromString returns FieldID from a string input
func FieldFromString(field string) (FieldID, error) {
	for k, v := range FieldToMeta {
		if v.Title == strings.ToUpper(field) {
			return k, nil
		}
	}
	return FieldID(-1), fmt.Errorf("field not found")
}

// ParseFields parses a comma separated string to a list of FieldID
// ex: "PNAME,PID,TYPE" -> [1,0,3]
func ParseFields(fieldString string) ([]FieldID, error) {
	fields := strings.Split(fieldString, ",")
	ret := make([]FieldID, 0, len(fields))
	for _, field := range fields {
		fID, err := FieldFromString(field)
		if err != nil {
			return nil, fmt.Errorf("incorrect string format: %w", err)
		}
		ret = append(ret, fID)
	}
	return ret, nil
}
