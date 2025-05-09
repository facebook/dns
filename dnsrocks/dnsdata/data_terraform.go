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

package dnsdata

import (
	"bytes"
	"fmt"
)

// TerraformRecord provides ability to extract value for terraform configuration
type TerraformRecord interface {
	WireRecord
	TerraformValue() (string, error)
}

// TerraformValue implements TerraformRecord interface
func (r *Raddr) TerraformValue() (string, error) {
	b, err := r.ip.MarshalText()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rns1) TerraformValue() (string, error) {
	w := new(bytes.Buffer)
	putdomtext(w, r.ns)
	return w.String(), nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rcname) TerraformValue() (string, error) {
	w := new(bytes.Buffer)
	putdomtext(w, r.cname)
	return w.String(), nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rptr) TerraformValue() (string, error) {
	w := new(bytes.Buffer)
	putdomtext(w, r.host)
	return w.String(), nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rmx1) TerraformValue() (string, error) {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "%d ", r.dist)
	putdomtext(w, r.mx)
	return w.String(), nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rtxt) TerraformValue() (string, error) {
	result := string(r.txt)

	if len(result) > 255 {
		result = result[:255] + "\"\"" + result[255:]
	}

	return result, nil
}

// TerraformValue implements TerraformRecord interface
func (r *Rsrv1) TerraformValue() (string, error) {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "%d %d %d ", r.pri, r.weight, r.port)
	putdomtext(w, r.srv)

	return w.String(), nil
}

// WireTypeToTerraformString converts WireType enum to Terraform type format
func WireTypeToTerraformString(t WireType) (string, error) {
	switch t {
	case TypeA:
		return "A", nil
	case TypeNS:
		return "NS", nil
	case TypeCNAME:
		return "CNAME", nil
	case TypeSOA:
		return "SOA", nil
	case TypePTR:
		return "PTR", nil
	case TypeMX:
		return "MX", nil
	case TypeTXT:
		return "TXT", nil
	case TypeAAAA:
		return "AAAA", nil
	case TypeSRV:
		return "SRV", nil
	case TypeSVCB:
		return "SVCB", nil
	case TypeHTTPS:
		return "HTTPS", nil
	}

	return "", fmt.Errorf("unknown wire type: %v", t)
}
