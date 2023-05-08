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
	"net"
	"strconv"

	"github.com/google/gopacket/layers"
	mkdns "github.com/miekg/dns"
)

// UNK string to be displayed for unknown info
const UNK = "UNK"

// DisplayInfo stores data about a complete match
// between (DNS query, DNS response, calling process)
type DisplayInfo struct {
	ProcInfo
	qTimestamp   int64
	rTimestamp   int64
	query        *layers.DNS
	response     *layers.DNS
	queryAddr    net.IP
	responseAddr net.IP
	fields       []FieldID
}

// DisplayHeader displays the header the field list
func DisplayHeader(fields []FieldID) string {
	var s string
	for _, field := range fields {
		s += fmt.Sprintf(FieldToMeta[field].Format, FieldToMeta[field].Title)
	}
	return s
}

// FieldValue returns the string of a field in displayinfo
func (d *DisplayInfo) FieldValue(field FieldID) string {
	s := UNK
	switch field {
	case FieldPID:
		if d.pid != 0 {
			s = strconv.Itoa(d.pid)
		}
	case FieldTID:
		if d.tid != 0 {
			s = strconv.Itoa(d.tid)
		}
	case FieldPNAME:
		if d.pid != 0 {
			s = d.pName
		}
	case FieldLAT:
		if d.qTimestamp != 0 && d.rTimestamp != 0 {
			s = strconv.Itoa(int(d.rTimestamp-d.qTimestamp) / 1000)
		}
	case FieldTYPE:
		if d.query != nil && len(d.query.Questions) > 0 {
			s = d.query.Questions[0].Type.String()
		} else if d.response != nil && len(d.response.Questions) > 0 {
			s = d.response.Questions[0].Type.String()
		}
	case FieldQNAME:
		if d.query != nil && len(d.query.Questions) > 0 {
			s = string(d.query.Questions[0].Name)
		} else if d.response != nil && len(d.response.Questions) > 0 {
			s = string(d.response.Questions[0].Name)
		}
	case FieldRCODE:
		if d.response != nil {
			s = mkdns.RcodeToString[int(d.response.ResponseCode)]
		}
	case FieldRIP:
		if d.response != nil {
			for _, v := range d.response.Answers {
				if d.query != nil && len(d.query.Questions) > 0 && d.query.Questions[0].Type.String() == v.Type.String() {
					s = v.IP.String()
				}
			}
		}
	case FieldQTIME:
		if d.query != nil {
			s = strconv.Itoa(int(d.qTimestamp / 1000))
		}
	case FieldRTIME:
		if d.response != nil {
			s = strconv.Itoa(int(d.rTimestamp / 1000))
		}
	case FieldCMDLINE:
		s = d.cmdline
	case FieldQADDR:
		s = d.queryAddr.String()
	case FieldRADDR:
		s = d.responseAddr.String()
	}
	return fmt.Sprintf(FieldToMeta[field].Format, s)
}

// String returns the string containing only the fields specified
func (d *DisplayInfo) String() string {
	var s string
	for _, v := range d.fields {
		s += d.FieldValue(v)
	}
	return s
}

func stringDNSQuestion(q *layers.DNSQuestion) string {
	s := ";" + string(q.Name) + "\t"
	s += q.Class.String() + "\t"
	s += " " + q.Type.String()
	return s
}

func stringRR(r *layers.DNSResourceRecord) string {
	var s string

	if r.Type == layers.DNSTypeOPT {
		s = ";"
	}

	s += string(r.Name) + "\t"
	s += strconv.FormatInt(int64(r.TTL), 10) + "\t"
	s += r.Class.String() + "\t"
	s += r.Type.String() + "\t"
	switch r.Type {
	case layers.DNSTypeA:
		s += r.IP.String() + "\t"
	case layers.DNSTypeAAAA:
		s += r.IP.String() + "\t"
	case layers.DNSTypeNS:
		s += string(r.NS) + "\t"
	case layers.DNSTypeCNAME:
		s += string(r.CNAME) + "\t"
	case layers.DNSTypePTR:
		s += string(r.PTR) + "\t"
	}
	return s
}

// DetailedString returns a dig like string
func (d *DisplayInfo) DetailedString() string {
	s := "\n"
	if d == nil {
		return "Packet nil"
	}
	dnsPkt := &layers.DNS{}
	s += ";; pid: "
	if d.pid != 0 {
		s += strconv.Itoa(d.pid)
		s += ", process name: " + d.pName
	} else {
		s += UNK
		s += ", process name: " + UNK
	}
	s += "\n"

	s += ";; query: "
	if d.query != nil {
		s += "true"
		dnsPkt = d.query
	} else {
		s += "false"
	}
	s += ", response: "
	if d.response != nil {
		s += "true"
		dnsPkt = d.response
	} else {
		s += "false"
	}
	s += ", latency(microsec): "
	if d.qTimestamp != 0 && d.rTimestamp != 0 {
		s += strconv.Itoa(int(d.rTimestamp-d.qTimestamp) / 1000)
	} else {
		s += UNK
	}
	s += "\n"

	s += ";; opcode: " + dnsPkt.OpCode.String()
	s += ", status: " + dnsPkt.ResponseCode.String()
	s += ", id: " + strconv.Itoa(int(dnsPkt.ID)) + "\n"

	s += ";; flags:"
	if dnsPkt.QR {
		s += " qr"
	}
	if dnsPkt.AA {
		s += " aa"
	}
	if dnsPkt.TC {
		s += " tc"
	}
	if dnsPkt.RD {
		s += " rd"
	}
	if dnsPkt.RA {
		s += " ra"
	}
	if dnsPkt.Z != 0 {
		s += " z"
	}
	s += "; "
	s += "QUERY: " + strconv.Itoa(len(dnsPkt.Questions)) + ", "
	s += "ANSWER: " + strconv.Itoa(len(dnsPkt.Answers)) + ", "
	s += "AUTHORITY: " + strconv.Itoa(len(dnsPkt.Authorities)) + ", "
	s += "ADDITIONAL: " + strconv.Itoa(len(dnsPkt.Additionals)) + "\n"
	if len(dnsPkt.Questions) > 0 {
		s += "\n;; QUESTION SECTION:\n"
		for _, q := range dnsPkt.Questions {
			s += stringDNSQuestion(&q) + "\n"
		}
	}
	if len(dnsPkt.Answers) > 0 {
		s += "\n;; ANSWER SECTION:\n"
		for _, r := range dnsPkt.Answers {
			s += stringRR(&r) + "\n"
		}
	}
	if len(dnsPkt.Authorities) > 0 {
		s += "\n;; AUTHORITY SECTION:\n"
		for _, r := range dnsPkt.Authorities {
			s += stringRR(&r) + "\n"
		}
	}
	if len(dnsPkt.Additionals) > 0 {
		s += "\n;; ADDITIONAL SECTION:\n"
		for _, r := range dnsPkt.Additionals {
			s += stringRR(&r) + "\n"
		}
	}
	s += "\n"
	return s
}
