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
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata/quote"
	"github.com/facebookincubator/dns/dnsrocks/dnsdata/svcb"

	"github.com/golang/glog"
)

// TTL constants
const (
	LongTTL  = 86400  // the default TTL for most of the record types
	ShortTTL = 2560   // the default TTL for SOA records
	LinkTTL  = 259200 // the default TTL for NS records
)

// Record is the interface implemented by all record types
type Record interface {
	encoding.TextUnmarshaler
	encoding.TextMarshaler
	MapMarshaler
}

// CompositeRecord is the interface implemented by records which are mapped to
// 2 or more simple records
type CompositeRecord interface {
	DerivedRecords() []Record
}

// MapMarshaler is the interface used in parsing implemented by all record types
type MapMarshaler interface {
	MarshalMap() ([]MapRecord, error)
}

// Accum accumulates information about subnets we parsed
type Accum struct {
	NoPrefixSets bool // if set, disables emission of the prefix set records - use with Ranger.Enable()
	prefixset    big.Int
	v4prefixset  big.Int
	v6prefixset  big.Int
	Ranger       SubnetRanger
	mux          sync.Mutex
}

// MapRecord is our main record to represent parsed DNS record
type MapRecord struct {
	Key   []byte
	Value []byte
}

// Codec provides accumulator and serial to construct all records
type Codec struct {
	Serial       uint32    // default SOA serial
	Acc          Accum     // a meta-record which represents an accumulated state over the whole data set
	NoRnetOutput bool      // if set, disables Rnet ("%"-records) output in the output - use with Acc.Ranger.Enable()
	Features     Rfeatures // a meta-record with features supported by generated DB
}

// rshared is a struct with fields are available to the most of record types
type rshared struct {
	ttl uint32 // "time to live"
	lo  Loc    // client location; the line is ignored for clients outside that location

	dom        []byte // the host
	iswildcard bool   // imply "*." in front of dom
}

// Rsoa is "Z" - SOA record.
type Rsoa struct {
	rshared
	c   *Codec
	ns  []byte // the primary name server
	adm []byte // the contact address (with the first . converted to @)
	ser uint32 // the serial number	(default: mtime of data file)
	ref uint32 // the refresh time (default: 16384)
	ret uint32 // the retry time (default: 2048)
	exp uint32 // the expire time (default: 1048576)
	min uint32 // the minimum time (default: 2560)
}

// Rdot is [composite]  . → (NS, A, SOA)
type Rdot struct {
	c *Codec
	Rsoa
	Rns
}

// Rns is [composite]  & → (NS, A)
type Rns struct {
	c *Codec
	Rns1
	Raddr
}

// Rns1 is just NS
type Rns1 struct {
	rshared
	c  *Codec
	ns []byte // the name server
}

// Raddr is + → A/AAAA
type Raddr struct {
	rshared
	c      *Codec
	ip     net.IP // the address
	weight uint32 // weight
}

// Rpaddr is [composite]  = → (A/AAAA, PTR)
type Rpaddr Raddr

// Rmx is [composite]  @ → (MX, A)
type Rmx struct {
	c *Codec
	Rmx1
	Raddr
}

// Rmx1 is just MX
type Rmx1 struct {
	rshared
	mx   []byte // the MX prefix/server
	dist uint32 // the "distance" (default: 0)
	c    *Codec
}

// Rsrv is [composite] S → (SRV, A)
type Rsrv struct {
	c *Codec
	Rsrv1
	Raddr
}

// Rsrv1 is just SRV
type Rsrv1 struct {
	rshared
	srv    []byte // the service
	pri    uint16 // priority, 0-65535
	weight uint16 // weight, 0-65535
	port   uint16 // the service port
	c      *Codec
}

// Rptr is ^ → PTR
type Rptr struct {
	rshared
	host []byte // the host name
	c    *Codec
}

// Rcname is C → CNAME
type Rcname struct {
	rshared
	cname []byte // the canonical name
	c     *Codec
}

// Rtxt is ' → TXT
type Rtxt struct {
	rshared
	txt []byte // the text
	c   *Codec
}

// Raux is : → (anything else)
type Raux struct {
	rshared
	rtype WireType // record type
	rdata []byte   // raw record data
	c     *Codec
}

// Rnet is "%" - Client subnet-to-location record.
// Used internally by the server to group client/resolver's subnets into locations.
type Rnet struct {
	lo    Loc        // client location; the line is ignored for clients outside that location
	ipnet *net.IPNet // IP subnet which are considered in the location
	lmap  Lmap       // [FB-only] ID of the map; the map is chosen with "M" or "8".
	c     *Codec
}

// Ripmap is [FB-only] M - define a resolver-based map
type Ripmap struct {
	dom  []byte // the host
	lmap Lmap   // ID of the map
	c    *Codec
}

// Rcsmap is [FB-only] 8 - define an EDNS client subnet-based map
type Rcsmap Ripmap

// Rrangepoint [FB-only] - an internal record type generated from "%" records using the "rearrangement" process
type Rrangepoint struct {
	lmap Lmap
	pt   *RangePoint
	c    *Codec
}

// Rfeatures [FB-only] - meta record storing set of features supported by current data format
type Rfeatures struct {
	// if true:
	//  - reverse zone names in maps (.com.facebook instead of facebook.com.)
	//  - uses \x00o prefix for owner names
	//  - location is added as suffix to owner names, not as prefix
	UseV2Keys bool
}

// Rsvcb is SVCB (service binding) record
// other SVCB-like records share the same layout as Rsvcb's
type Rsvcb struct {
	rshared
	wtype    WireType       // resource type. for identifying SVCB/HTTPS/...
	tgtname  []byte         // target name. qname is the service alias of tgtname
	priority uint16         // aliasmode or servicemode (2 octets)
	params   svcb.ParamList // params is a *sorted* list of svcb parameters
	c        *Codec
}

// Rhttps is HTTPS record, which is exactly SVCB but for HTTPS info exchange
// Type number is different
type Rhttps Rsvcb

// Rtype is record type
type Rtype string

// Loc is location ID
type Loc []byte

// Lmap is map ID
type Lmap [2]byte

// WireType represent DNS wire types
type WireType uint16

const (
	// TypeA represents A record type
	TypeA WireType = 1
	// TypeNS represents NS record type
	TypeNS WireType = 2
	// TypeCNAME represents CNAME record type
	TypeCNAME WireType = 5
	// TypeSOA represents SOA record type
	TypeSOA WireType = 6
	// TypePTR represents PTR record type
	TypePTR WireType = 12
	// TypeMX represents MX record type
	TypeMX WireType = 15
	// TypeTXT represents TXT record type
	TypeTXT WireType = 16
	// TypeAAAA represents AAAA record type
	TypeAAAA WireType = 28
	// TypeSRV represents SRV record type
	TypeSRV WireType = 33
	// TypeSVCB represents SVCB record type
	// for SVCB/HTTPS, see https://datatracker.ietf.org/doc/html/draft-ietf-dnsop-svcb-https-08
	TypeSVCB WireType = 64
	// TypeHTTPS represents HTTPS record type
	TypeHTTPS WireType = 65
)

func (w WireType) String() string {
	switch w {
	case TypeA:
		return "A"
	case TypeNS:
		return "NS"
	case TypeCNAME:
		return "CNAME"
	case TypeSOA:
		return "SOA"
	case TypePTR:
		return "PTR"
	case TypeMX:
		return "MX"
	case TypeTXT:
		return "TXT"
	case TypeAAAA:
		return "AAAA"
	case TypeSRV:
		return "SRV"
	case TypeSVCB:
		return "SVCB"
	case TypeHTTPS:
		return "HTTPS"
	}

	return fmt.Sprintf("%d", w)
}

// record prefixes
const (
	prefixComment    Rtype = "#"
	prefixNet        Rtype = "%"
	prefixSOA        Rtype = "Z"
	prefixDot        Rtype = "."
	prefixNS         Rtype = "&"
	prefixAddr       Rtype = "+"
	prefixPAddr      Rtype = "="
	prefixMX         Rtype = "@"
	prefixSRV        Rtype = "S"
	prefixCName      Rtype = "C"
	prefixPTR        Rtype = "^"
	prefixTXT        Rtype = "'"
	prefixAUX        Rtype = ":"
	prefixIPMap      Rtype = "M"
	prefixCSMap      Rtype = "8"
	prefixRangePoint Rtype = "!"
	prefixSVCB       Rtype = "B"
	prefixHTTPS      Rtype = "H"
)

func decodeRtype(text []byte) Rtype {
	return Rtype(text[:1])
}

// ErrBadRType is an error returned when a record with an invalid type character encountered.
var ErrBadRType = errors.New("bad record type")

func (c *Codec) newRecord(t Rtype) (Record, error) {
	switch t {
	case prefixNet:
		return &Rnet{c: c}, nil
	case prefixSOA:
		return &Rsoa{c: c}, nil
	case prefixDot:
		return &Rdot{c: c}, nil
	case prefixNS:
		return &Rns{c: c}, nil
	case prefixAddr:
		return &Raddr{c: c}, nil
	case prefixPAddr:
		return &Rpaddr{c: c}, nil
	case prefixMX:
		return &Rmx{c: c}, nil
	case prefixSRV:
		return &Rsrv{c: c}, nil
	case prefixCName:
		return &Rcname{c: c}, nil
	case prefixPTR:
		return &Rptr{c: c}, nil
	case prefixTXT:
		return &Rtxt{c: c}, nil
	case prefixAUX:
		return &Raux{c: c}, nil
	case prefixIPMap:
		return &Ripmap{c: c}, nil
	case prefixCSMap:
		return &Rcsmap{c: c}, nil
	case prefixRangePoint:
		return &Rrangepoint{c: c}, nil
	case prefixSVCB:
		return &Rsvcb{c: c, wtype: TypeSVCB}, nil
	case prefixHTTPS:
		return &Rhttps{c: c, wtype: TypeHTTPS}, nil
	}
	return nil, ErrBadRType
}

func (c *Codec) decodeRecord(text []byte) (Record, error) {
	t := decodeRtype(text)
	r, err := c.newRecord(t)
	if err != nil {
		return nil, err
	}
	if err = r.UnmarshalText(text); err != nil {
		return nil, err
	}
	return r, nil
}

// DecodeLn parses a line without converting
func (c *Codec) DecodeLn(text []byte) (Record, error) {
	r, err := c.decodeRecord(text)
	if err != nil {
		return nil, err
	}
	err = c.Acc.update(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ConvertLn is the main function used to parse line into MapRecords
func (c *Codec) ConvertLn(text []byte) ([]MapRecord, error) {
	r, err := c.DecodeLn(text)
	if err != nil {
		return nil, err
	}
	out, err := r.MarshalMap()
	if err != nil {
		return nil, err
	}
	return out, nil
}

const (
	// NUMFIELDS is the number of fields to guarantee upon splitting
	NUMFIELDS = 15
)

// two variants of separators
var (
	SEP  = []byte(":") // original record separator
	NSEP = []byte(",") // [FB-only] IPv6-friendly record separator
)

// detect the separator in use
func detectSep(b []byte) []byte {
	iSEP := bytes.Index(b, SEP)
	iNSEP := bytes.Index(b, NSEP)
	if iSEP == -1 ||
		iNSEP != -1 && iNSEP < iSEP {
		return NSEP
	}
	return SEP
}

// the line tokenizer
func fields(text []byte) [][]byte {
	b := text[1:]
	sep := detectSep(b)
	f := bytes.SplitN(b, sep, NUMFIELDS)
	// SplitN allocates f with cap(f) == NUMFIELDS. Safely grow the margin:
	for len(f) < NUMFIELDS {
		f = append(f, nil)
	}
	return f
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rnet) UnmarshalText(text []byte) error {
	f := fields(text)
	var err error
	r.lo, err = getloc(f[0])
	if err != nil {
		return err
	}
	ipnet, err := parseipnet(string(f[1]))
	if err != nil {
		return err
	}
	ipnet.IP = ipnet.IP.To16()
	if ones, bits := ipnet.Mask.Size(); bits < 128 {
		ones += 128 - bits
		bits = 128
		ipnet.Mask = net.CIDRMask(ones, bits)
	}
	r.ipnet = ipnet
	r.lmap = getlmap(f[2])
	return nil
}

func parseipnet(s string) (ipnet *net.IPNet, err error) {
	_, ipnet, err = net.ParseCIDR(s)
	if err != nil {
		ip := net.ParseIP(s)
		if ip == nil {
			if s != "" {
				return nil, err
			}
			return &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, nil
		}
		if ip.To4() != nil {
			return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}, nil
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
	}
	return ipnet, err
}

// MarshalMap implements MapMarshaler
func (r *Rnet) MarshalMap() ([]MapRecord, error) {
	if r.c != nil && r.c.NoRnetOutput {
		return nil, nil
	}
	nbits, _ := r.ipnet.Mask.Size()
	nbytes := nbits / 8
	isV4 := r.ipnet.IP.To4() != nil

	m := make([]MapRecord, 0, 2)
	if isV4 && nbits >= 96 && nbits%8 == 0 {
		k := new(bytes.Buffer)
		k.Write([]byte("\000%"))
		putlmap(k, r.lmap)
		k.Write(r.ipnet.IP[12:nbytes])
		v := new(bytes.Buffer)
		putloc(v, r.lo)
		m = append(m, MapRecord{Key: k.Bytes(), Value: v.Bytes()})
	}

	k := new(bytes.Buffer)
	k.WriteString("\000%")
	putlmap(k, r.lmap)
	k.Write(r.ipnet.IP[0:16])
	k.Write([]byte{byte(nbits)})
	v := new(bytes.Buffer)
	putloc(v, r.lo)
	m = append(m, MapRecord{Key: k.Bytes(), Value: v.Bytes()})
	return m, nil
}

const (
	// RangePointKeyMarker is the prefix for the RangePoint keys
	RangePointKeyMarker = "\000\000\000!"
	// MlenNoLoc is the special value of the mask length byte indicating the location is not defined.
	// As it finds its way into the values of the keys, it must stay zero.
	MlenNoLoc = 0
	// FeaturesKey is a special key storing bitmap what format is used for other keys in DB
	FeaturesKey = "\x00o_features"
	// ResourceRecordsKeyMarker is the prefix for the resource record keys
	ResourceRecordsKeyMarker = "\000o"
)

// Feature is a bitmap representing different characteristics of DB data
type Feature uint32

const (
	// V1KeysFeature if set in features key means V2 keys are available in DB
	//  - natural order zone names used in maps (ex facebook.com.)
	//  - no additional prefix for owner name
	//  - location is added as prefix to owner names
	V1KeysFeature Feature = 1 << iota
	// V2KeysFeature if set in features key means V2 keys are available in DB
	//  - reverse zone names in maps (.com.facebook instead of facebook.com.)
	//  - uses \x00o prefix for owner names
	//  - location is added as suffix to owner names, not as prefix
	V2KeysFeature
)

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rrangepoint) UnmarshalText(text []byte) error {
	r.pt = &RangePoint{location: rangeLocation{}}

	f := fields(text)
	r.lmap = getlmap(f[0])
	ip := net.ParseIP(string(f[1]))
	r.pt.rangeStart = FromNetIP(ip)
	getuint8(f[2], &r.pt.location.maskLen)
	var err error
	locID, err := getloc(f[3])
	if err != nil {
		return err
	}
	copy(r.pt.location.locID[:], locID)

	r.pt.location.locIDIsNull = (locID == nil)
	if !r.pt.location.locIDIsNull && ip.To4() != nil {
		r.pt.location.maskLen += (net.IPv6len - net.IPv4len) * 8
	}
	return nil
}

// MarshalMap implements MapMarshaler
func (r *Rrangepoint) MarshalMap() ([]MapRecord, error) {
	k := new(bytes.Buffer)
	k.WriteString(RangePointKeyMarker)
	putlmap(k, r.lmap)
	pt := r.pt
	ip := pt.To16()
	k.Write(ip[0:16])

	v := new(bytes.Buffer)
	if pt.LocIsNull() {
		mlen := byte(MlenNoLoc)
		k.Write([]byte{mlen})
	} else {
		mlen := pt.MaskLen()
		k.Write([]byte{mlen})
		lo := Loc(pt.LocID())
		putloc(v, lo)
	}
	mr := MapRecord{Key: k.Bytes(), Value: v.Bytes()}
	return []MapRecord{mr}, nil
}

// MarshalMap implements MapMarshaler
func (r *Accum) MarshalMap() ([]MapRecord, error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	vm := r.marshalPrefixSets()
	mr, err := r.Ranger.MarshalMap()
	if err != nil {
		return nil, err
	}
	vm = append(vm, mr...)
	return vm, nil
}

// ErrBadMode is returned when a function is called with unexpected combination of operation settings
var ErrBadMode = errors.New("bad combination of mode/flags")

// MarshalText implements encoding.TextMarshaller
func (r *Accum) MarshalText() ([]byte, error) {
	if !r.NoPrefixSets {
		return nil, ErrBadMode
	}

	scanner, err := r.OpenScanner()
	if err != nil {
		return nil, err
	}

	b := []byte{}
	for scanner.Scan() {
		b = append(b, []byte(scanner.Text())...)
		b = append(b, []byte("\n")...)
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return b, nil
}

// OpenScanner creates scanner through accumulated records
// note that Accum will remain locked until all records are scanned (Scan returns false)
func (r *Accum) OpenScanner() (s *AccumulatorScanner, err error) {
	if !r.NoPrefixSets {
		return nil, ErrBadMode
	}

	r.mux.Lock()

	// scanner will release Accum record lock once read is finished
	accumulatorScanner := AccumulatorScanner{
		rangerScanner: r.Ranger.OpenScanner(),
		parent:        r,
	}

	return &accumulatorScanner, nil
}

// AccumulatorScanner allows lazy reading of lines generated by accumulator
type AccumulatorScanner struct {
	rangerScanner *SubnetRangerScanner
	parent        *Accum
}

// Scan pushes scanner to new line if one exists. In this case result will be
// true and that line could be read with Text() method
func (s AccumulatorScanner) Scan() bool {
	scanResult := s.rangerScanner.Scan()

	if !scanResult {
		// finished scanning
		s.parent.mux.Unlock()
	}

	return scanResult
}

// Text returns current line
func (s AccumulatorScanner) Text() string {
	return s.rangerScanner.Text()
}

// Err returns error if Scanner met any issues, nil otherwise
func (s AccumulatorScanner) Err() error {
	return s.rangerScanner.Err()
}

// accounts the record in the accumulator
func (r *Accum) update(s Record) error {
	if rnet, ok := s.(*Rnet); ok {
		r.mux.Lock()
		defer r.mux.Unlock()
		r.updatePrefixSet(rnet)
		return r.Ranger.addSubnet(rnet)
	}
	return nil
}

// called with r.mux held locked
func (r *Accum) updatePrefixSet(s *Rnet) {
	if r.NoPrefixSets {
		return
	}
	isV4 := s.ipnet.IP.To4() != nil
	nbits, _ := s.ipnet.Mask.Size()

	i := &r.prefixset
	i.SetBit(i, nbits, 1)
	if isV4 {
		i = &r.v4prefixset
		i.SetBit(i, nbits, 1)
	} else {
		i = &r.v6prefixset
		i.SetBit(i, nbits, 1)
	}
}

// called with r.mux held locked
func (r *Accum) marshalPrefixSets() []MapRecord {
	if r.NoPrefixSets {
		return nil
	}
	makeMapRecord := func(key []byte, prefixset big.Int) MapRecord {
		k := new(bytes.Buffer)
		k.Write(key)
		v := make([]byte, 0, 128)
		for i := 128; i >= 0; i-- {
			if prefixset.Bit(i) != 0 {
				v = append(v, byte(i))
			}
		}
		return MapRecord{Key: k.Bytes(), Value: v}
	}
	return []MapRecord{
		makeMapRecord([]byte("\000/"), r.prefixset),
		makeMapRecord([]byte("\0004"), r.v4prefixset),
		makeMapRecord([]byte("\0006"), r.v6prefixset),
	}
}

func getuint32(b []byte, out *uint32) {
	if x, err := strconv.ParseUint(string(b), 10, 32); err == nil {
		*out = uint32(x)
	}
}

func getuint16(b []byte, out *uint16) {
	if x, err := strconv.ParseUint(string(b), 10, 16); err == nil {
		*out = uint16(x)
	}
}

func getuint8(b []byte, out *uint8) {
	if x, err := strconv.ParseUint(string(b), 10, 8); err == nil {
		*out = uint8(x)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rsoa) UnmarshalText(text []byte) error {
	f := fields(text)
	r.loadDefaults()
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	r.ns, _ = quote.Bunquote(f[1])  // BUG: handle error
	r.adm, _ = quote.Bunquote(f[2]) // BUG: handle error
	getuint32(f[3], &r.ser)
	getuint32(f[4], &r.ref)
	getuint32(f[5], &r.ret)
	getuint32(f[6], &r.exp)
	getuint32(f[7], &r.min)
	getuint32(f[8], &r.ttl)
	var err error
	r.lo, err = getloc(f[10])
	return err
}

func (r *Rsoa) loadDefaults() {
	r.ttl = ShortTTL
	r.ref = 16384
	r.ret = 2048
	r.exp = 1048576
	r.min = 2560
	if r.c != nil {
		r.ser = r.c.Serial
	}
}

// MarshalMap implements MapMarshaler
func (r *Rsoa) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                    // BUG scale
	putrrhead(v, TypeSOA, r.ttl, r.lo, false) // BUG wildcard?
	putdom(v, r.ns)
	putdom(v, r.adm)
	err := binary.Write(v, binary.BigEndian, r.ser)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.ref)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.ret)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.exp)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.min)
	if err != nil {
		return nil, err
	}

	return []MapRecord{{Key: k, Value: v.Bytes()}}, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rdot) UnmarshalText(text []byte) error {
	soa := &r.Rsoa
	ns := &r.Rns
	soa.c = r.c
	ns.c = r.c
	ns.loadDefaults()
	if err := ns.UnmarshalText(text); err != nil {
		return err
	}
	ns1 := &r.Rns.Rns1
	soa.loadDefaults()
	if ns1.ttl == 0 {
		soa.ttl = 0
	}
	soa.dom = ns1.dom
	soa.ns = ns1.ns
	frag := [][]byte{[]byte("hostmaster"), ns1.dom}
	soa.adm = bytes.Join(frag, []byte("."))
	soa.lo = ns1.lo
	return nil
}

// MarshalMap implements MapMarshaler
func (r *Rdot) MarshalMap() ([]MapRecord, error) {
	records := r.DerivedRecords()
	return marshalMapv(records...)
}

func marshalMapv(vi ...Record) ([]MapRecord, error) {
	vm := make([]MapRecord, 0, len(vi))

	for _, i := range vi {
		m, err := i.MarshalMap()
		if err != nil {
			return nil, err
		}
		vm = append(vm, m...)
	}
	return vm, nil
}

// DerivedRecords implements CompositeRecord interface
func (r *Rdot) DerivedRecords() []Record {
	return []Record{&r.Rsoa, &r.Rns}
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rns) UnmarshalText(text []byte) error {
	r.loadDefaults()
	ns := &r.Rns1
	a := &r.Raddr
	ns.c = r.c
	a.c = r.c

	f := fields(text)
	if err := ns.unmarshalFields(f); err != nil {
		return err
	}

	a.ip = net.ParseIP(string(f[1]))
	a.dom = ns.ns
	a.iswildcard = false
	a.ttl = ns.ttl
	a.lo = ns.lo
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rns1) UnmarshalText(text []byte) error {
	r.loadDefaults()

	f := fields(text)
	return r.unmarshalFields(f)
}

func (r *Rns1) unmarshalFields(f [][]byte) error {
	r.loadDefaults()

	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	// skip IP - for composite parsing use Rns struct
	r.ns, _ = quote.Bunquote(f[2]) // BUG: handle error
	if !bytes.Contains(r.ns, []byte(".")) {
		frag := [][]byte{r.ns, []byte("ns"), r.dom}
		r.ns = bytes.Join(frag, []byte("."))
	}
	getuint32(f[3], &r.ttl)
	// f[4] ignored
	var err error
	r.lo, err = getloc(f[5])
	if err != nil {
		return err
	}

	return nil
}

func (r *Rns) loadDefaults() {
	r.Rns1.loadDefaults()
	r.Raddr.loadDefaults()
}

func (r *Rns1) loadDefaults() {
	r.ttl = LinkTTL
}

// MarshalMap implements MapMarshaler
func (r *Rns1) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                   // BUG scale
	putrrhead(v, TypeNS, r.ttl, r.lo, false) // BUG wildcard?
	putdom(v, r.ns)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// MarshalMap implements MapMarshaler
func (r *Rns) MarshalMap() ([]MapRecord, error) {
	records := r.DerivedRecords()
	return marshalMapv(records...)
}

// DerivedRecords implements CompositeRecord interface
func (r *Rns) DerivedRecords() []Record {
	records := make([]Record, 0, 2)
	records = append(records, &r.Rns1)

	if r.Raddr.ip != nil {
		records = append(records, &r.Raddr)
	}

	return records
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Raddr) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, r.iswildcard = getdom(f[0])
	r.ip = net.ParseIP(string(f[1]))
	getuint32(f[2], &r.ttl)
	// f[3] ignored
	var err error
	r.lo, err = getloc(f[4])
	getuint32(f[5], &r.weight)
	return err
}

func (r *Raddr) loadDefaults() {
	r.ttl = LongTTL
	r.weight = 1
}

// MarshalMap implements MapMarshaler
func (r *Raddr) MarshalMap() ([]MapRecord, error) {
	if r.ip == nil {
		return []MapRecord{}, nil
	}

	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)
	if ip4 := r.ip.To4(); ip4 != nil {
		putrrhead(v, TypeA, r.ttl, r.lo, r.iswildcard)
		err := binary.Write(v, binary.BigEndian, r.weight)
		if err != nil {
			return nil, err
		}
		v.Write(ip4)
	} else {
		putrrhead(v, TypeAAAA, r.ttl, r.lo, r.iswildcard)
		err := binary.Write(v, binary.BigEndian, r.weight)
		if err != nil {
			return nil, err
		}
		v.Write(r.ip)
	}
	x := MapRecord{Key: k, Value: v.Bytes()}
	return []MapRecord{x}, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rpaddr) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, r.iswildcard = getdom(f[0])
	r.ip = net.ParseIP(string(f[1]))
	getuint32(f[2], &r.ttl)
	// f[3] ignored
	var err error
	r.lo, err = getloc(f[4])
	return err
}

func (r *Rpaddr) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Rpaddr) MarshalMap() ([]MapRecord, error) {
	records := r.DerivedRecords()
	return marshalMapv(records...)
}

// DerivedRecords implements CompositeRecord interface
func (r *Rpaddr) DerivedRecords() []Record {
	a := new(Raddr)
	a.loadDefaults()
	a.dom = r.dom
	a.iswildcard = r.iswildcard
	a.ip = r.ip
	a.ttl = r.ttl
	a.lo = r.lo
	a.c = r.c

	ptr := new(Rptr)
	ptr.dom = Reverseaddr(r.ip)
	ptr.host = r.dom
	if r.iswildcard {
		ptr.host = append([]byte("*."), ptr.host...) // BUG replicate the bug in tinydns-data
	}
	ptr.ttl = r.ttl
	ptr.lo = r.lo
	ptr.c = r.c

	return []Record{a, ptr}
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rmx) UnmarshalText(text []byte) error {
	r.loadDefaults()
	mx := &r.Rmx1
	a := &r.Raddr
	mx.c = r.c
	a.c = r.c

	f := fields(text)
	if err := mx.unmarshalFields(f); err != nil {
		return err
	}

	a.dom = mx.mx
	a.ip = net.ParseIP(string(f[1]))
	a.ttl = mx.ttl
	a.lo = mx.lo

	return nil
}

func (r *Rmx) loadDefaults() {
	r.Rmx1.loadDefaults()
	r.Raddr.loadDefaults()
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rmx1) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	return r.unmarshalFields(f)
}

func (r *Rmx1) unmarshalFields(f [][]byte) error {
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	// skip ip
	r.mx, _ = quote.Bunquote(f[2]) // BUG: handle error
	if !bytes.Contains(r.mx, []byte(".")) {
		frag := [][]byte{r.mx, []byte("mx"), r.dom}
		r.mx = bytes.Join(frag, []byte("."))
	}
	getuint32(f[3], &r.dist)
	getuint32(f[4], &r.ttl)
	// f[5] ignored
	var err error
	r.lo, err = getloc(f[6])
	return err
}

func (r *Rmx1) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Rmx1) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                   // BUG scale
	putrrhead(v, TypeMX, r.ttl, r.lo, false) // BUG wildcard?
	err := binary.Write(v, binary.BigEndian, uint16(r.dist))
	if err != nil {
		return nil, err
	}
	putdom(v, r.mx)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// MarshalMap implements MapMarshaler
func (r *Rmx) MarshalMap() ([]MapRecord, error) {
	records := r.DerivedRecords()
	return marshalMapv(records...)
}

// DerivedRecords implements CompositeRecord interface
func (r *Rmx) DerivedRecords() []Record {
	records := make([]Record, 0, 2)
	records = append(records, &r.Rmx1)

	if r.Raddr.ip != nil {
		records = append(records, &r.Raddr)
	}

	return records
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rsrv) UnmarshalText(text []byte) error {
	r.loadDefaults()
	srv := &r.Rsrv1
	a := &r.Raddr
	srv.c = r.c
	a.c = r.c

	f := fields(text)
	if err := srv.unmarshalFields(f); err != nil {
		return err
	}

	a.dom = srv.srv
	a.ip = net.ParseIP(string(f[1]))
	a.ttl = srv.ttl
	a.lo = srv.lo

	return nil
}

func (r *Rsrv) loadDefaults() {
	r.Rsrv1.loadDefaults()
	r.Raddr.loadDefaults()
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rsrv1) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	return r.unmarshalFields(f)
}

func (r *Rsrv1) loadDefaults() {
	r.ttl = LongTTL
}

func (r *Rsrv1) unmarshalFields(f [][]byte) error {
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	// skip ip
	r.srv, _ = quote.Bunquote(f[2]) // BUG: handle error
	if !bytes.Contains(r.srv, []byte(".")) {
		frag := [][]byte{r.srv, []byte("srv"), r.dom}
		r.srv = bytes.Join(frag, []byte("."))
	}
	getuint16(f[3], &r.port)
	getuint16(f[4], &r.pri)
	getuint16(f[5], &r.weight)
	getuint32(f[6], &r.ttl)
	// f[7] ignored
	var err error
	r.lo, err = getloc(f[8])
	return err
}

// MarshalMap implements MapMarshaler
func (r *Rsrv1) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                    // BUG scale
	putrrhead(v, TypeSRV, r.ttl, r.lo, false) // BUG wildcard?
	err := binary.Write(v, binary.BigEndian, r.pri)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.weight)
	if err != nil {
		return nil, err
	}
	err = binary.Write(v, binary.BigEndian, r.port)
	if err != nil {
		return nil, err
	}
	putdom(v, r.srv)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// MarshalMap implements MapMarshaler
func (r *Rsrv) MarshalMap() ([]MapRecord, error) {
	records := r.DerivedRecords()
	return marshalMapv(records...)
}

// DerivedRecords implements CompositeRecord interface
func (r *Rsrv) DerivedRecords() []Record {
	records := make([]Record, 0, 2)
	records = append(records, &r.Rsrv1)

	if r.Raddr.ip != nil {
		records = append(records, &r.Raddr)
	}

	return records
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rcname) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, r.iswildcard = getdom(f[0])
	r.cname, _ = quote.Bunquote(f[1]) // BUG: handle error
	getuint32(f[2], &r.ttl)
	// f[3] ignored
	var err error
	r.lo, err = getloc(f[4])
	return err
}

func (r *Rcname) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Rcname) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer) // BUG scale
	putrrhead(v, TypeCNAME, r.ttl, r.lo, r.iswildcard)
	putdom(v, r.cname)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rptr) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, _ = quote.Bunquote(f[0])  // BUG: handle error
	r.host, _ = quote.Bunquote(f[1]) // BUG: handle error
	getuint32(f[2], &r.ttl)
	// f[3] ignored
	var err error
	r.lo, err = getloc(f[4])
	return err
}

func (r *Rptr) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Rptr) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                    // BUG scale
	putrrhead(v, TypePTR, r.ttl, r.lo, false) // BUG wildcard
	putdom(v, r.host)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rtxt) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, r.iswildcard = getdom(f[0])
	r.txt, _ = quote.Bunquote(f[1]) // BUG: handle error
	getuint32(f[2], &r.ttl)
	// f[3] ignored
	var err error
	r.lo, err = getloc(f[4])
	return err
}

func (r *Rtxt) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Rtxt) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer) // BUG scale
	putrrhead(v, TypeTXT, r.ttl, r.lo, r.iswildcard)
	for sofar := 0; sofar < len(r.txt); {
		n := len(r.txt) - sofar
		if n > 127 {
			n = 127
		}
		v.Write([]byte{byte(n)})
		v.Write(r.txt[sofar : sofar+n])
		sofar += n
	}
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Raux) UnmarshalText(text []byte) error {
	r.loadDefaults()
	f := fields(text)
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	var rtype uint32
	getuint32(f[1], &rtype)
	r.rtype = WireType(rtype)         // BUG validate input
	r.rdata, _ = quote.Bunquote(f[2]) // BUG: handle error
	getuint32(f[3], &r.ttl)
	// f[4] ignored
	var err error
	r.lo, err = getloc(f[5])
	return err
}

func (r *Raux) loadDefaults() {
	r.ttl = LongTTL
}

// MarshalMap implements MapMarshaler
func (r *Raux) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)
	v := new(bytes.Buffer)                    // BUG scale
	putrrhead(v, r.rtype, r.ttl, r.lo, false) // BUG wildcard
	v.Write(r.rdata)
	m := []MapRecord{{Key: k, Value: v.Bytes()}}
	return m, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Ripmap) UnmarshalText(text []byte) error {
	f := fields(text)
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	r.lmap = getlmap(f[1])
	return nil
}

// MarshalMap implements MapMarshaler
func (r *Ripmap) MarshalMap() ([]MapRecord, error) {
	k := makemapkey([]byte("\000M"), r.dom, r.c)
	v := new(bytes.Buffer) // BUG scale
	putlmap(v, r.lmap)

	return []MapRecord{{Key: k, Value: v.Bytes()}}, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rcsmap) UnmarshalText(text []byte) error {
	f := fields(text)
	r.dom, _ = quote.Bunquote(f[0]) // BUG: handle error
	r.lmap = getlmap(f[1])
	return nil
}

// MarshalMap implements MapMarshaler
func (r *Rcsmap) MarshalMap() ([]MapRecord, error) {
	k := makemapkey([]byte("\0008"), r.dom, r.c)
	v := new(bytes.Buffer) // BUG scale
	putlmap(v, r.lmap)

	return []MapRecord{{Key: k, Value: v.Bytes()}}, nil
}

// MarshalMap implements MapMarshaler
func (r *Rfeatures) MarshalMap() ([]MapRecord, error) {
	var features Feature

	if r.UseV2Keys {
		features |= V2KeysFeature
	} else {
		features |= V1KeysFeature
	}

	return []MapRecord{{Key: []byte(FeaturesKey), Value: encodeFeatures(features)}}, nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rsvcb) UnmarshalText(text []byte) error {
	f := fields(text)

	r.dom, r.iswildcard = getdom(f[0])
	r.tgtname, _ = getdom(f[1])

	getuint32(f[2], &r.ttl)

	var err error
	r.lo, err = getloc(f[3])
	if err != nil {
		return err
	}
	getuint16(f[4], &r.priority)

	return r.params.FromText(f[5])
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *Rhttps) UnmarshalText(text []byte) error {
	return (*Rsvcb)(r).UnmarshalText(text)
}

// MarshalMap implements MapMarshaler. It creates a map record for Rsvcb,
// based on the type of the caller. The map key includes the domain name
// and location, the value is "rrhead" + RDATA
func (r *Rsvcb) MarshalMap() ([]MapRecord, error) {
	k := makedomainkey(r.dom, r.lo, r.c)

	var buf bytes.Buffer
	putrrhead(&buf, r.wtype, r.ttl, r.lo, r.iswildcard)

	// marshal the RDATA part
	err := binary.Write(&buf, binary.BigEndian, r.priority)
	if err != nil {
		return nil, err
	}
	putdom(&buf, r.tgtname)
	err = r.params.ToWire(&buf)
	if err != nil {
		return nil, err
	}

	return []MapRecord{{Key: k, Value: buf.Bytes()}}, nil
}

// MarshalMap implements MapMarshaler
func (r *Rhttps) MarshalMap() ([]MapRecord, error) {
	return (*Rsvcb)(r).MarshalMap()
}

// encodeFeatures converts Feature object to be stored in DB
func encodeFeatures(f Feature) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(f))
	return data
}

// DecodeFeatures byte array stored in DB to a Feature
func DecodeFeatures(data []byte) Feature {
	return Feature(binary.LittleEndian.Uint32(data))
}

func getdom(text []byte) (dom []byte, iswildcard bool) {
	dom, _ = quote.Bunquote(text) // BUG: handle error
	iswildcard = bytes.HasPrefix(dom, []byte("*."))
	if iswildcard {
		dom = dom[2:]
	}
	return dom, iswildcard
}

func getloc(b []byte) (Loc, error) {
	q, err := quote.Bunquote(b)
	if err != nil {
		return Loc(nil), err
	}
	if len(q) != 2 {
		return Loc(nil), nil
	}
	return Loc(q), nil
}

func getlmap(b []byte) Lmap {
	q, _ := quote.Bunquote(b) // BUG: handle error
	var a [2]byte
	copy(a[:], q)
	return Lmap(a)
}

// write an FQDN in DNS wire format
func putdom(w io.Writer, a []byte) {
	f := bytes.Split(a, []byte("."))
	for _, s := range f {
		n := byte(len(s))
		if n > 0 {
			_, err := w.Write([]byte{n})
			if err != nil {
				glog.Errorf("%v", err)
			}
			_, err = w.Write(s[:n])
			if err != nil {
				glog.Errorf("%v", err)
			}
		}
	}
	_, err := w.Write([]byte{0})
	if err != nil {
		glog.Errorf("%v", err)
	}
}

func putreverseddom(w io.Writer, a []byte) {
	f := bytes.Split(a, []byte("."))
	for i := len(f) - 1; i >= 0; i-- {
		s := f[i]

		n := byte(len(s))
		if n > 0 {
			_, err := w.Write([]byte{n})
			if err != nil {
				glog.Errorf("%v", err)
			}
			_, err = w.Write(s)
			if err != nil {
				glog.Errorf("%v", err)
			}
		}
	}

	_, err := w.Write([]byte{0})
	if err != nil {
		glog.Errorf("%v", err)
	}
}

func putdomtext(w io.Writer, a []byte) {
	// this is for root domain
	if len(a) == 1 && a[0] == '.' {
		_, err := w.Write([]byte("."))
		if err != nil {
			glog.Errorf("%v", err)
		}
		return
	}
	quoted := quote.Bquote(a[:])
	// get rid of repeating dots and trailing dot
	f := bytes.Split(quoted, []byte("."))
	toWrite := make([][]byte, 0, len(f))
	for _, s := range f {
		n := byte(len(s))
		if n > 0 {
			toWrite = append(toWrite, s[:n])
		}
	}
	_, err := w.Write(bytes.Join(toWrite, []byte(".")))
	if err != nil {
		glog.Errorf("%v", err)
	}
}

// write a two-byte location ID
func putloc(w io.Writer, lo Loc) {
	var err error
	if len(lo) == 2 {
		_, err = w.Write(lo)
	} else {
		_, err = w.Write([]byte{0, 0})
	}
	if err != nil {
		glog.Errorf("%v", err)
	}
}

func putloctext(w io.Writer, lo Loc) {
	for _, b := range lo {
		fmt.Fprintf(w, "\\%03o", b)
	}
}

// write a two-byte location map ID
func putlmap(w io.Writer, m Lmap) {
	_, err := w.Write(m[:])
	if err != nil {
		glog.Errorf("%v", err)
	}
}

func putlmaptext(w io.Writer, m Lmap) {
	for _, b := range m {
		fmt.Fprintf(w, "\\%03o", b)
	}
}

func putquotedtext(w io.Writer, data []byte) {
	_, err := w.Write(quote.Bquote(data[:]))
	if err != nil {
		glog.Errorf("%v", err)
	}
}

func putrrhead(w io.Writer, t WireType, ttl uint32, loc Loc, iswildcard bool) {
	err := binary.Write(w, binary.BigEndian, uint16(t))
	if err != nil {
		glog.Errorf("%v", err)
	}
	if len(loc) != 2 || (loc[0] == 0 && loc[1] == 0) {
		if iswildcard {
			_, err = w.Write([]byte("*"))
		} else {
			_, err = w.Write([]byte("="))
		}
		if err != nil {
			glog.Errorf("%v", err)
		}
	} else {
		if iswildcard {
			_, err = w.Write([]byte("+"))
		} else {
			_, err = w.Write([]byte(">"))
		}
		if err != nil {
			glog.Errorf("%v", err)
		}
		_, err = w.Write(loc)
		if err != nil {
			glog.Errorf("%v", err)
		}
	}
	err = binary.Write(w, binary.BigEndian, ttl)
	if err != nil {
		glog.Errorf("%v", err)
	}
	_, err = w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0}) // ttd[8] is not used
	if err != nil {
		glog.Errorf("%v", err)
	}
}

// Convert unsigned integer to decimal string.
// (lifted from https://golang.org/src/net/parse.go?m=text)
func uitoa(val uint) string {
	if val == 0 { // avoid string allocation
		return "0"
	}
	var buf [20]byte // big enough for 64bit value base 10
	i := len(buf) - 1
	for val >= 10 {
		q := val / 10
		buf[i] = byte('0' + val - q*10)
		i--
		val = q
	}
	// val < 10
	buf[i] = byte('0' + val)
	return string(buf[i:])
}

// Reverseaddr returns the in-addr.arpa. or ip6.arpa. hostname of the IP
// address addr suitable for rDNS (PTR) record lookup or an error if it fails
// to parse the IP address.
// (lifted from https://golang.org/src/net/dnsclient.go?m=text)
func Reverseaddr(ip net.IP) []byte {
	if ip.To4() != nil {
		return []byte(uitoa(uint(ip[15])) + "." + uitoa(uint(ip[14])) + "." + uitoa(uint(ip[13])) + "." + uitoa(uint(ip[12])) + ".in-addr.arpa")
	}
	// Must be IPv6
	buf := make([]byte, 0, len(ip)*4+len("ip6.arpa"))
	// Add it, in reverse, to the buffer
	for i := len(ip) - 1; i >= 0; i-- {
		v := ip[i]
		buf = append(buf, "0123456789abcdef"[v&0xF])
		buf = append(buf, '.')
		buf = append(buf, "0123456789abcdef"[v>>4])
		buf = append(buf, '.')
	}
	// Append "ip6.arpa." and return (buf already has the final .)
	buf = append(buf, "ip6.arpa"...)
	return buf
}

func makedomainkey(domain []byte, lo Loc, codec *Codec) []byte {
	k := new(bytes.Buffer) // BUG scale
	k.Grow(len(domain) + 2)

	domain = bytes.ToLower(domain)

	if codec.Features.UseV2Keys {
		k.WriteString(ResourceRecordsKeyMarker)
		putreverseddom(k, domain)
		putloc(k, lo)
	} else {
		putloc(k, lo)
		putdom(k, domain)
	}

	return k.Bytes()
}

func makemapkey(mapID, domain []byte, codec *Codec) []byte {
	k := new(bytes.Buffer) // BUG scale
	k.Write(mapID)

	suffix := "="

	if bytes.HasPrefix(domain, []byte("*.")) {
		domain = domain[2:]
		suffix = "*"
	}

	domain = bytes.ToLower(domain)

	if codec.Features.UseV2Keys {
		putreverseddom(k, domain)
	} else {
		putdom(k, domain)
	}

	k.WriteString(suffix)

	return k.Bytes()
}
