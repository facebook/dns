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

// MarshalText implements encoding.TextMarshaler
func (r *Rsoa) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixSOA))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	putdomtext(w, r.ns)
	w.Write(NSEP)
	putdomtext(w, r.adm)
	w.Write(NSEP)
	if r.ser != 0 {
		fmt.Fprintf(w, "%d", r.ser)
	}
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ref)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ret)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.exp)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.min)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	// skip one unused field
	w.Write(NSEP)
	w.Write(NSEP)
	Putloctext(w, r.lo)

	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rnet) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixNet))
	Putloctext(w, r.lo)
	w.Write(NSEP)
	w.Write([]byte(r.ipnet.String()))
	w.Write(NSEP)
	Putlmaptext(w, r.lmap)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rns) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixNS))
	putdomtext(w, r.Rns1.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	putdomtext(w, r.ns)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.Rns1.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.Rns1.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rns1) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixNS))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	// no ip address
	w.Write(NSEP)
	putdomtext(w, r.ns)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Raddr) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixAddr))
	if r.iswildcard {
		w.WriteString("*.")
	}
	putdomtext(w, r.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.weight)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rpaddr) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixPAddr))
	if r.iswildcard {
		w.WriteString("*.")
	}
	putdomtext(w, r.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rmx) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixMX))
	putdomtext(w, r.Rmx1.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	putdomtext(w, r.mx)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.dist)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.Rmx1.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.Rmx1.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rmx1) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixMX))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	// skip ip
	w.Write(NSEP)
	putdomtext(w, r.mx)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.dist)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rsrv) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixSRV))
	putdomtext(w, r.Rsrv1.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	putdomtext(w, r.srv)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.port)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.pri)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.Rsrv1.weight)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.Rsrv1.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.Rsrv1.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rsrv1) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixSRV))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	// skip ip
	w.Write(NSEP)
	putdomtext(w, r.srv)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.port)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.pri)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.weight)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rcname) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixCName))
	if r.iswildcard {
		w.WriteString("*.")
	}
	putdomtext(w, r.dom)
	w.Write(NSEP)
	putdomtext(w, r.cname)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rptr) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixPTR))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	putdomtext(w, r.host)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rtxt) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixTXT))
	if r.iswildcard {
		w.WriteString("*.")
	}
	putdomtext(w, r.dom)
	w.Write(NSEP)
	putquotedtext(w, r.txt)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Raux) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixAUX))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.rtype)
	w.Write(NSEP)
	putquotedtext(w, r.rdata)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rdot) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixDot))
	putdomtext(w, r.Rns1.dom)
	w.Write(NSEP)
	b, err := r.ip.MarshalText()
	if err != nil {
		return nil, err
	}
	w.Write(b)
	w.Write(NSEP)
	putdomtext(w, r.Rns1.ns)
	w.Write(NSEP)
	fmt.Fprintf(w, "%d", r.Rns1.ttl)
	w.Write(NSEP)
	// skip one unused field
	w.Write(NSEP)
	Putloctext(w, r.Rns1.lo)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Ripmap) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixIPMap))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	Putlmaptext(w, r.lmap)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rcsmap) MarshalText() (text []byte, err error) {
	w := new(bytes.Buffer)
	w.WriteString(string(prefixCSMap))
	putdomtext(w, r.dom)
	w.Write(NSEP)
	Putlmaptext(w, r.lmap)
	return w.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rrangepoint) MarshalText() (text []byte, err error) {
	return r.pt.MarshalTextForLmap(r.lmap)
}

// MarshalText implements encoding.TextMarshaler
func (r *Rsvcb) MarshalText() (text []byte, err error) {
	buf := new(bytes.Buffer) // use new() to keep consistent with other MarshalText()
	switch r.wtype {
	case TypeSVCB:
		buf.WriteString(string(prefixSVCB))
	case TypeHTTPS:
		buf.WriteString(string(prefixHTTPS))
	default:
		return nil, fmt.Errorf("unknown wiretype for SVCB record")
	}
	putdomtext(buf, r.dom)
	buf.Write(NSEP)
	putdomtext(buf, r.tgtname)
	buf.Write(NSEP)
	fmt.Fprintf(buf, "%d", r.ttl)
	buf.Write(NSEP)
	Putloctext(buf, r.lo)
	buf.Write(NSEP)
	fmt.Fprintf(buf, "%d", r.priority)
	buf.Write(NSEP)
	r.params.ToText(buf)
	return buf.Bytes(), nil
}

// MarshalText implements encoding.TextMarshaler
func (r *Rhttps) MarshalText() (text []byte, err error) {
	return (*Rsvcb)(r).MarshalText()
}
