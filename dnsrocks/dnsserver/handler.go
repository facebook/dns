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

package dnsserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/facebook/dns/dnsrocks/db"
	"github.com/facebook/dns/dnsrocks/dnsserver/test"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/edns"
	"github.com/coredns/coredns/request"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

const (
	// TypeToStatsPrefix is the prefix used for creating stats keys
	TypeToStatsPrefix              = "DNS_query"
	maxAnswer         maxAnswerKey = "maxans"
	// DefaultMaxAnswer is the default number of answer returned for A\AAAA query
	DefaultMaxAnswer = 1

	emptyLoc           = "\x00\x00"
	defaultLoc2        = "\x00\x01"
	defaultFallbackLoc = "\x00\x02"
	defaultLocN        = "@default"
)

var typeToStats = make(map[uint16]string)

type cacheEntry struct {
	expiration int64
	response   *dns.Msg
}

type maxAnswerKey string

// WithMaxAnswer set max ans in context
func WithMaxAnswer(ctx context.Context, masAns int) context.Context {
	return context.WithValue(ctx, maxAnswer, masAns)
}

// GetMaxAnswer is used to get max answer number from context
func GetMaxAnswer(ctx context.Context) (int, bool) {
	maxAns, ok := ctx.Value(maxAnswer).(int)
	return maxAns, ok
}

func init() {
	// initialize typeToStats map.
	for k, v := range dns.TypeToString {
		typeToStats[k] = fmt.Sprintf("%s.%s", TypeToStatsPrefix, v)
	}
}

func typeToStatsKey(qtype uint16) string {
	if t, ok := typeToStats[qtype]; ok {
		return t
	}
	return fmt.Sprintf("%s.TYPE%d", TypeToStatsPrefix, qtype)
}

// MakeOPTWithECS returns dns.OPT with a specified subnet EDNS0 option
// FIXME: Instead of returning a dns.OPT specifically for EDNS0
// Make this function more standardized to support all types of
// options (e.g: ecs, dns cookie..)
// see: https://tools.ietf.org/html/rfc6891#section-6
func MakeOPTWithECS(s string) (*dns.OPT, error) {
	o := new(dns.OPT)
	o.Hdr.Name = "."
	o.Hdr.Rrtype = dns.TypeOPT
	e := new(dns.EDNS0_SUBNET)
	e.Code = dns.EDNS0SUBNET
	ipaddr, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}

	if ipaddr.To4() != nil {
		e.Family = 1
		e.Address = ipaddr.To4()
	} else {
		e.Family = 2
		e.Address = ipaddr
	}
	msize, _ := ipnet.Mask.Size()
	e.SourceNetmask = uint8(msize)
	o.Option = append(o.Option, e)
	return o, nil
}

// writeAndLog writes the response to the network as well as log and bump stats
func (h *FBDNSDB) writeAndLog(state request.Request, resp *dns.Msg, ecs *dns.EDNS0_SUBNET, loc *db.Location, startTime time.Time) (int, error) {
	rcode := resp.Rcode

	state.SizeAndDo(resp)
	state.Scrub(resp)

	if h.handlerConfig.AlwaysCompress {
		// Compression should be set AFTER potential Truncate call inside Scrub
		// otherwise it could be reset
		resp.Compress = true
	}

	err := state.W.WriteMsg(resp)
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	h.logger.Log(state, resp, ecs, loc, time.Since(startTime).Microseconds())
	if !resp.Authoritative {
		h.stats.IncrementCounter("DNS_queries_notauthoritative")
	}
	if rcode == dns.RcodeNameError {
		h.stats.IncrementCounter("DNS_queries_nxdomain")
	} else if rcode == dns.RcodeRefused {
		h.stats.IncrementCounter("DNS_queries_refused")
	} else if rcode == dns.RcodeBadVers {
		h.stats.IncrementCounter("DNS_queries_badvers")
	} else if rcode == dns.RcodeSuccess && len(resp.Answer) == 0 {
		h.stats.IncrementCounter("DNS_queries_nodata")
	}

	return rcode, nil
}

func (h *FBDNSDB) chaseCNAME(reader db.Reader, localState request.Request, maxAns int, a *dns.Msg, ecs *dns.EDNS0_SUBNET) ([]dns.RR, bool, error) {
	var (
		packedQName = make([]byte, 255)
		// the location matching this requestor and target
		loc *db.Location
	)

	offset, err := dns.PackDomainName(localState.Name(), packedQName, 0, nil, false)
	if err != nil {
		h.stats.IncrementCounter("DNS_cname_chasing.pack_domain_fail")
		glog.Errorf("could not pack cname domain %s", localState.Name())
		dns.HandleFailed(localState.W, localState.Req)
		return nil, false, err
	}

	packedQName = packedQName[:offset]

	var prevScopePrefixLen uint8
	if ecs != nil {
		prevScopePrefixLen = ecs.SourceScope
	}

	if loc, err = reader.FindLocation(packedQName, ecs, localState.IP()); err != nil {
		glog.Errorf("%s: failed to find location: %v", localState.Name(), err)
		return nil, false, err
	}

	if loc == nil {
		// We could not find a location, not even the default one... potentially a bogus DB.
		h.stats.IncrementCounter("DNS_cname_chasing.location.nil")
		glog.Errorf("%s: nil location", localState.Name())
		return nil, false, fmt.Errorf("no location found, not even default one")
	}

	// Stop CNAME chasing if scope prefix length changes
	if ecs != nil && ecs.SourceScope != prevScopePrefixLen {
		glog.Errorf("ECS scope prefix length changed from %d to %d during CNAME chasing %s", prevScopePrefixLen, ecs.SourceScope, localState.Name())
		h.stats.IncrementCounter("DNS_cname_chasing.ecs_scope_changed")
		// Restore scope prefix length from previous RR
		ecs.SourceScope = prevScopePrefixLen
		return nil, false, fmt.Errorf("ecs scope prefix length changed")
	}

	_, auth, zoneCut, err := reader.IsAuthoritative(packedQName, loc.LocID)
	if err != nil {
		h.stats.IncrementCounter("DNS_cname_chasing.is_authoritative.error")
		dns.HandleFailed(localState.W, localState.Req)
		return nil, false, err
	}

	if !auth {
		h.stats.IncrementCounter("DNS_cname_chasing.not_authoritative")
		// nolint: nilerr
		return nil, false, nil
	}

	answerSizeBefore := len(a.Answer)
	weighted, _ := reader.FindAnswer(packedQName, zoneCut, localState.QName(), localState.QType(), loc.LocID, a, maxAns)

	newRecords := a.Answer[answerSizeBefore:]
	if len(newRecords) == 0 {
		h.stats.IncrementCounter("DNS_cname_chasing.qtype.not_found")
	}
	return newRecords, weighted, nil
}

// ServeDNSWithRCODE handles a dns query and with return the RCODE and eventual
// error that happen during processing.
func (h *FBDNSDB) ServeDNSWithRCODE(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	var (
		startTime = time.Now() // for measuring response time
		// the location matching this requestor
		loc *db.Location
		// EDNS Client subnet option
		ecs *dns.EDNS0_SUBNET
		o   *dns.OPT
		// packed lowercased version of the qname
		packedQName = make([]byte, 255)
		zoneCut     []byte
		// Used to track if the answer was using Weighted Random Sample or not. When
		// using WRS, we should not cache it.
		weighted = false
		// When caching is enabled, this will hold the cache key
		cacheKey string
	)
	h.stats.IncrementCounter("DNS_queries")

	reader, err := h.AcquireReader()
	if err != nil {
		h.stats.IncrementCounter("DNS_db.read_error")
		// We cannot acquire a reader, most likely because the DB couldn't be loaded.
		// dns.HandleFailed(w, r)
		// FIXME error code "E"
		// nolint: nilerr
		return dns.RcodeServerFailure, nil
	}
	defer reader.Close()
	// State carries important information about the current request.
	// It is also used to write the reply.
	state := request.Request{W: w, Req: r}

	if state.Do() {
		h.stats.IncrementCounter("DNS_queries.edns0.do_bit")
	}
	h.stats.IncrementCounter(typeToStatsKey(state.QType()))

	// Check if this is a supported edns version
	if a, err := edns.Version(state.Req); err != nil { // Wrong EDNS version, return at once.
		return h.writeAndLog(state, a, ecs, loc, startTime)
	}

	offset, err := dns.PackDomainName(state.Name(), packedQName, 0, nil, false)
	if err != nil {
		h.stats.IncrementCounter("DNS_error.pack_domain_fail")
		glog.Errorf("could not pack domain %s", state.Name())
		dns.HandleFailed(state.W, state.Req)
		h.logger.LogFailed(state, ecs, loc, time.Since(startTime).Microseconds())
		// nolint: nilerr
		return dns.RcodeServerFailure, nil
	}

	packedQName = packedQName[:offset]

	ecs = db.FindECS(state.Req)
	if loc, err = reader.FindLocation(packedQName, ecs, state.IP()); err != nil {
		glog.Errorf("%s: failed to find location: %v", state.Name(), err)
		h.logger.LogFailed(state, ecs, loc, time.Since(startTime).Microseconds())
		return dns.RcodeServerFailure, nil
	}

	if loc == nil {
		// We could not find a location, not even the default one... potentially a bogus DB.
		h.stats.IncrementCounter("DNS_location.nil")
		glog.Errorf("%s: nil location", state.Name())
		h.logger.LogFailed(state, ecs, loc, time.Since(startTime).Microseconds())
		return dns.RcodeServerFailure, nil
	}

	if string(loc.LocID) == emptyLoc {
		h.stats.IncrementCounter("DNS_location.empty")
	} else {
		if loc.Mask > 0 {
			h.stats.IncrementCounter("DNS_location.ecs")
		} else if string(loc.LocID) == defaultLoc2 || string(loc.LocID) == defaultLocN {
			h.stats.IncrementCounter("DNS_location.default")
		} else if string(loc.LocID) == defaultFallbackLoc {
			h.stats.IncrementCounter("DNS_location.fallback_default")
		} else {
			h.stats.IncrementCounter("DNS_location.resolver")
		}

		if len(loc.LocID) == 2 {
			h.stats.IncrementCounter("DNS_location.short")
		} else {
			h.stats.IncrementCounter("DNS_location.long")
		}
	}

	if h.cacheConfig.Enabled {
		cacheKey = fmt.Sprintf("%.3d%.3d%.3d%s", loc.LocID, state.QType(), state.QClass(), state.Name())
		if v, ok := h.lru.Get(cacheKey); ok {
			t := v.(cacheEntry).expiration
			if t < time.Now().Unix() {
				// evict answer
				h.stats.IncrementCounter("DNS_cache.expired")
				h.lru.Remove(cacheKey)
			} else {
				h.stats.IncrementCounter("DNS_cache.hit")
				resp := v.(cacheEntry).response.Copy()
				// SetReply sets rcode to RcodeSuccess...
				rcode := resp.Rcode
				resp.SetReply(state.Req)
				resp.Rcode = rcode
				if state.Req.IsEdns0() != nil {
					o = new(dns.OPT)
					o.Hdr.Name = "."
					o.Hdr.Rrtype = dns.TypeOPT

					if ecs != nil {
						o.Option = append(o.Option, ecs)
					}

					resp.Extra = append([]dns.RR{o}, resp.Extra...)
				}
				return h.writeAndLog(state, resp, ecs, loc, startTime)
			}
		} else {
			h.stats.IncrementCounter("DNS_cache.missed")
		}
	}

	// Set default answer payload
	a := new(dns.Msg)
	a.SetReply(state.Req)
	a.Compress = true
	a.Authoritative = true

	// Check if we are authoritative for this domain or if at least we know about
	// its name servers. The domain returned is the one for which we found
	// matching SOA or NS
	ns, auth, zoneCut, err := reader.IsAuthoritative(packedQName, loc.LocID)

	if err != nil {
		h.stats.IncrementCounter("DNS_error.is_authoritative")
		dns.HandleFailed(state.W, state.Req)
		return dns.RcodeServerFailure, err
	}

	if !ns && !auth {
		h.stats.IncrementCounter("DNS_response.refused")
		m := new(dns.Msg)
		m.SetRcode(state.Req, dns.RcodeRefused)
		// We can use Extended DNS Errors to indicate that the server is not authoritative for certain Query
		// instead of just returning a REFUSED
		if state.Req.IsEdns0() != nil {
			m.SetEdns0(4096, true)
			ede := dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeNotAuthoritative}
			m.IsEdns0().Option = append(m.IsEdns0().Option, &ede)
		}
		// does not matter if this write fails
		return h.writeAndLog(state, m, ecs, loc, startTime)
	}

	// Not authoritative but we have NS (implicit or we would not have passed the
	// previous check) and requested type is DS, this is handled at the parent
	// zone. We should return a negative answer (unless we support DNSSEC).
	// Pop a label and find the authority below.
	// https://tools.ietf.org/html/rfc3658#section-2.2.1.1
	// https://lists.isc.org/pipermail/bind-users/2018-September/100668.html
	if !auth && state.QType() == dns.TypeDS {
		_, auth, zoneCut, err = reader.IsAuthoritative(packedQName[packedQName[0]+1:], loc.LocID)
		if err != nil {
			h.stats.IncrementCounter("DNS_error.is_authoritative")
			dns.HandleFailed(state.W, state.Req)
			return dns.RcodeServerFailure, err
		}
	}

	// If we are not authoritative, mark it so
	// otherwise, look for matches qname matches.
	if !auth {
		// q is in child zone
		a.Authoritative = false
		h.stats.IncrementCounter("DNS_response.not_authoritative")
	} else {
		h.stats.IncrementCounter("DNS_response.authoritative")

		maxAns, ok := GetMaxAnswer(ctx)
		if !ok {
			maxAns = DefaultMaxAnswer
			// log something
		}
		weighted, a.Rcode = reader.FindAnswer(packedQName, zoneCut, state.QName(), state.QType(), loc.LocID, a, maxAns)

		// Don't do CNAME chasing for ECS queries (https://fburl.com/8lamzlu1) or queries
		// of type CNAME or ANY: https://datatracker.ietf.org/doc/html/rfc1034#section-3.6.2.
		// An exception for ECS queries is if the ECS scope prefix length is 0 (denoting global).
		if h.handlerConfig.CNAMEChasing && (ecs == nil || ecs.SourceScope == 0) && state.QType() != dns.TypeCNAME && state.QType() != dns.TypeANY {
			newRecords := a.Answer
			var (
				iterCount    = 0
				maxCNAMEHops = h.handlerConfig.MaxCNAMEHops
			)

			for len(newRecords) == 1 && newRecords[0].Header().Rrtype == dns.TypeCNAME {
				if iterCount == maxCNAMEHops {
					h.stats.IncrementCounter("DNS_cname_chasing.max_hops")
					glog.Errorf("Max hops (%d) reached for CNAME chasing for qname: %s", maxCNAMEHops, state.Name())
					break
				}

				iterCount++
				target := newRecords[0].(*dns.CNAME).Target

				// CNAME cycle detection.
				// a.Answer contains all records we've seen until now. So by comparing target
				// with all records in a.Answer, we ensure that *any* CNAME cycle will be detected.
				for _, record := range a.Answer {
					if record.Header().Name == target {
						h.stats.IncrementCounter("DNS_cname_chasing.cname_cycle")
						glog.Errorf("CNAME cycle detected: %s", target)
						return dns.RcodeServerFailure, nil
					}
				}

				updatedState := state.NewWithQuestion(target, state.QType())
				newRecords, weighted, err = h.chaseCNAME(reader, updatedState, maxAns, a, ecs)
				if err != nil {
					glog.Errorf("Failed to chase CNAME for domain: %s, target: %s, error: %v", state.Name(), target, err)
					break
				}
			}
			// only increment counters if we did CNAME chasing
			if iterCount > 0 {
				h.stats.IncrementCounter("DNS_cname_chasing.queries")
				h.stats.AddSample("DNS_cname_chasing.hop_count", int64(iterCount))
			}
		}
	}

	unpackedControlDomain, _, err := dns.UnpackDomainName(zoneCut, 0)
	if err != nil {
		glog.Errorf("Failed to unpack control domain name %s", err)
		dns.HandleFailed(state.W, state.Req)
		h.logger.Log(state, state.Req, ecs, loc, time.Since(startTime).Microseconds())
		return dns.RcodeServerFailure, nil
	}

	// If we do not have any answer and we are authoritative, add SOA
	// otherwise, if we are not authoritative, we have a delegation (we
	// previously returned REFUSED if !(auth || ns)).
	// Add NS RRset to Authority section if we don't have NS RRset in Answer
	// section.
	if auth && len(a.Answer) == 0 {
		db.FindSOA(reader, zoneCut, unpackedControlDomain, loc.LocID, a)
	} else if !auth && !db.HasRecord(a, unpackedControlDomain, dns.TypeNS) {
		ns, err := db.GetNs(reader, zoneCut, unpackedControlDomain, state.QClass(), loc.LocID)
		if err != nil {
			glog.Errorf("%v", err)
		}
		if err == nil && len(ns) > 0 {
			a.Ns = append(a.Ns, ns...)
		}
	}

	// Additional section
	weighted = db.AdditionalSectionForRecords(reader, a, loc.LocID, state.QClass(), a.Answer) || weighted
	weighted = db.AdditionalSectionForRecords(reader, a, loc.LocID, state.QClass(), a.Ns) || weighted

	if h.cacheConfig.Enabled {
		// Cache answer before we add ECS/options
		var timeout int64
		if !weighted {
			// FIXME: we can leave this in cache until it get flushed (via DB reload)
			timeout = time.Now().Unix() + 1000
			h.lru.Add(cacheKey, cacheEntry{expiration: timeout, response: a.Copy()})
		} else if h.cacheConfig.WRSTimeout > 0 {
			timeout = time.Now().Unix() + h.cacheConfig.WRSTimeout
			h.lru.Add(cacheKey, cacheEntry{expiration: timeout, response: a.Copy()})
		}
	}

	if state.Req.IsEdns0() != nil {
		o = new(dns.OPT)
		o.Hdr.Name = "."
		o.Hdr.Rrtype = dns.TypeOPT

		if ecs != nil {
			o.Option = append(o.Option, ecs)
		}

		a.Extra = append([]dns.RR{o}, a.Extra...)
	}

	return h.writeAndLog(state, a, ecs, loc, startTime)
}

// ServeDNS implements the plugin.Handler interface.
func (h *FBDNSDB) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	requestStartTime := time.Now()
	rcode, err := h.ServeDNSWithRCODE(ctx, w, r)
	h.stats.AddSample("DNS.responsetime_us", time.Since(requestStartTime).Microseconds())
	return rcode, err
}

// Name returns tinydnscdb.
func (h *FBDNSDB) Name() string { return "tinydnscdb" }

func rrTypeToUnit(qType string) (uint16, error) {
	if val, ok := dns.StringToType[strings.ToUpper(qType)]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("unknown QTYPE %s", qType)
}

// QuerySingle queries dns server for a query, returning single answer if possible
func (h *FBDNSDB) QuerySingle(rtype, record, remoteIP, subnet string, maxAns int) (*dnstest.Recorder, error) {
	req := new(dns.Msg)
	qt, err := rrTypeToUnit(rtype)
	if err != nil {
		return nil, fmt.Errorf("could not find Rrtype, error: %w, aborting", err)
	}
	req.SetQuestion(dns.Fqdn(record), qt)
	if subnet != "" {
		o, err := MakeOPTWithECS(subnet)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ECS option for %s %w", subnet, err)
		}
		req.Extra = []dns.RR{o}
	}

	ctx := context.TODO()
	ctx = WithMaxAnswer(ctx, maxAns)

	rec := dnstest.NewRecorder(&test.ResponseWriterCustomRemote{RemoteIP: remoteIP})
	_, err = h.ServeDNSWithRCODE(ctx, rec, req)
	if err != nil {
		return nil, err
	}
	return rec, nil
}
