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
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/google/gopacket/layers"
	termbox "github.com/nsf/termbox-go"
	log "github.com/sirupsen/logrus"
)

const ntdoc = `dnswatch - DNS snooping
SORTBY keys - m (ADDR), f (DNS), d (%DNSTRAFFIC), n (NXDOMAIN), o (NOERROR), s (SERVFAIL), a (A), b (AAAA), p (PTR)
SORTBY keys - < (MOVE SORTING COL LEFT), > (MOVE SORTING COL RIGHT)
TOGGLE keys - w (QUERY/RESPONSE ADDR AGGREGATE), x (KEEP TERMINATED PROCESSES / DELETE OLD PROCESSES REQUEST)
NUMBERS AGGREGATED SINCE THE START OF THE RUN
`

const (
	ntDNSnr = iota
	ntADDR
	ntDNS
	ntNXDOM
	ntNOERR
	ntSERVF
	ntA
	ntAAAA
	ntPTR
)

// NetTopRow contains data about each row in nettop display
type NetTopRow struct {
	QueryAddr    string
	ResponseAddr string

	DNS   percentField
	NXDOM percentField
	NOERR percentField
	SERVF percentField
	A     percentField
	AAAA  percentField
	PTR   percentField

	rTimestamp int64
}

// NetTopData contains the entire nettop display table
type NetTopData struct {
	// addr to row
	Rows  map[string]*NetTopRow
	total int
	nxdom int
	noerr int
	servf int
	a     int
	aaaa  int
	ptr   int
}

// oldFilter keeps only the new DNS requests on screen
func (t *NetTopData) oldFilter(per time.Duration, byQueryAddr bool) *NetTopData {
	ret := *t
	newRows := make(map[string]*NetTopRow)
	for _, v := range t.Rows {
		if v.rTimestamp >= time.Now().UnixNano()-per.Nanoseconds() {
			if byQueryAddr {
				newRows[v.QueryAddr] = v
			} else {
				newRows[v.ResponseAddr] = v
			}
		}
	}
	ret.Rows = newRows
	return &ret
}

func displayMapToNetTop(displayMap map[UniqueDNS]*DisplayInfo, byQueryAddr bool) *NetTopData {
	ret := &NetTopData{}
	ret.Rows = make(map[string]*NetTopRow)

	for _, v := range displayMap {
		ret.total++
		// aggregate by response addr by default
		id := v.responseAddr.String()
		if byQueryAddr {
			id = v.queryAddr.String()
		}
		if ret.Rows[id] == nil {
			ret.Rows[id] = &NetTopRow{
				QueryAddr:    v.queryAddr.String(),
				ResponseAddr: v.responseAddr.String(),
			}
		}
		ret.Rows[id].rTimestamp = v.rTimestamp
		ret.Rows[id].DNS.val++
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeNXDomain {
			ret.Rows[id].NXDOM.val++
			ret.nxdom++
		}
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeNoErr {
			ret.Rows[id].NOERR.val++
			ret.noerr++
		}
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeServFail {
			ret.Rows[id].SERVF.val++
			ret.servf++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypeA {
			ret.Rows[id].A.val++
			ret.a++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypeAAAA {
			ret.Rows[id].AAAA.val++
			ret.aaaa++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypePTR {
			ret.Rows[id].PTR.val++
			ret.ptr++
		}
	}
	for _, v := range ret.Rows {
		v.DNS.computePerc(ret.total)
		v.NXDOM.computePerc(v.DNS.val)
		v.NOERR.computePerc(v.DNS.val)
		v.SERVF.computePerc(v.DNS.val)
		v.A.computePerc(v.DNS.val)
		v.AAAA.computePerc(v.DNS.val)
		v.PTR.computePerc(v.DNS.val)
	}
	return ret
}

// NetTopState is the current state of the interactive env
type NetTopState struct {
	data    *NetTopData
	rawData map[UniqueDNS]*DisplayInfo
	cliRows int

	sortBy      int
	byQueryAddr bool
	oldShow     bool

	startTime   time.Time
	lastRefresh time.Time
	refTime     time.Duration
}

func (t *NetTopState) printDocumentation() {
	fmt.Printf("%v", ntdoc)
	agg := "RESPONSE"
	if t.byQueryAddr {
		agg = "QUERY"
	}
	fmt.Printf("Aggregating by %s addr\n", agg)
}

func (t *NetTopState) printAggregated() {
	dateFormat := "2006-01-02 15:04:05.000"
	fmt.Printf("\nSTART TIME: %10v, LAST REFRESH: %10v\n", t.startTime.Format(dateFormat), t.lastRefresh.Format(dateFormat))
	fmt.Printf("%-19v: %10v\n", "DNS TRAFFIC (Q-R)", t.data.total)
	fmt.Printf("%-19v: %10v, %-19v: %10v, %-19v: %10v\n", "A QUERIES", t.data.a, "AAAA QUERIES", t.data.aaaa, "PTR QUERIES", t.data.ptr)
	fmt.Printf("%-19v: %10v, %-19v: %10v, %-19v: %10v\n", "NXDOMAIN RESPONSES", t.data.nxdom, "NOERROR RESPONSES", t.data.noerr, "SERVFAIL RESPONSES", t.data.servf)
}

func (t *NetTopState) printData(maxRows int) {
	agg := "RESPONSE"
	if t.byQueryAddr {
		agg = "QUERY"
	}
	formatHeader := "%-40v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v\n"
	formatRow := "%-40v  %-9v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v\n"
	fmt.Printf("%-10v  %-15v  %-20v  %-31v  %-31v\n", "", "", "<----TOTAL---->", "<------------RCODE------------>", "<----------QNAME--------->")
	fmt.Printf(formatHeader, fmt.Sprintf("%s ADDR", agg), "DNS", "%DNS", " %NXDOMAIN", "%NOERROR", " %SERVFAIL", "%A", " %AAAA", "  %PTR")

	var topMap *NetTopData
	topMap = t.data
	if !t.oldShow {
		topMap = topMap.oldFilter(t.refTime, t.byQueryAddr)
	}

	if t != nil && topMap.Rows != nil {
		vals := make([]*NetTopRow, 0, len(topMap.Rows))
		for _, v := range topMap.Rows {
			vals = append(vals, v)
		}
		sort.Slice(vals, func(i, j int) bool {
			switch t.sortBy {
			case ntADDR:
				if t.byQueryAddr {
					return vals[i].QueryAddr > vals[j].QueryAddr
				}
				return vals[i].ResponseAddr > vals[j].ResponseAddr
			case ntDNSnr:
				return vals[i].DNS.val > vals[j].DNS.val
			case ntDNS:
				return vals[i].DNS.per > vals[j].DNS.per
			case ntNXDOM:
				return vals[i].NXDOM.per > vals[j].NXDOM.per
			case ntNOERR:
				return vals[i].NOERR.per > vals[j].NOERR.per
			case ntSERVF:
				return vals[i].SERVF.per > vals[j].SERVF.per
			case ntA:
				return vals[i].A.per > vals[j].A.per
			case ntAAAA:
				return vals[i].AAAA.per > vals[j].AAAA.per
			case ntPTR:
				return vals[i].PTR.per > vals[j].PTR.per
			default:
				return vals[i].DNS.val > vals[j].DNS.val
			}
		})
		last := int(math.Min(float64(len(vals)-1), float64(maxRows)))
		for i := 0; i <= last; i++ {
			v := vals[i]
			addr := v.ResponseAddr
			if t.byQueryAddr {
				addr = v.QueryAddr
			}
			fmt.Printf(formatRow, addr, v.DNS.val, v.DNS.per, v.NXDOM.per, v.NOERR.per, v.SERVF.per, v.A.per, v.AAAA.per, v.PTR.per)
		}
	}
}

func (t *NetTopState) refreshScreen() {
	// \033[2J and termbox.Clear are used to clean the screen
	fmt.Printf("\033[2J")
	err := termbox.Clear(coldef, coldef)
	if err != nil {
		log.Error("failed to clear screen")
	}
	termbox.SetCursor(0, 0)

	_, t.cliRows = termbox.Size()

	t.printDocumentation()
	t.printAggregated()
	t.printData(t.cliRows - 15)

	termbox.HideCursor()
	termbox.Flush()
}

// StartNetTop is the nettop stdout handler
func StartNetTop(refreshChan <-chan map[UniqueDNS]*DisplayInfo, stopChan chan<- bool, refTime time.Duration) {
	rand.Seed(time.Now().UnixNano())

	err := termbox.Init()
	if err != nil {
		log.Error("failed to set up screen")
	}
	defer termbox.Close()

	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	tState := &NetTopState{
		data:        &NetTopData{},
		startTime:   time.Now(),
		lastRefresh: time.Now(),
		byQueryAddr: true,
		oldShow:     true,
		refTime:     refTime,
	}

	tState.refreshScreen()

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey {
				switch {
				case ev.Ch == '<':
					tState.sortBy--
					if tState.sortBy < ntADDR {
						tState.sortBy = tPTR
					}
				case ev.Ch == '>':
					tState.sortBy++
					tState.sortBy = tState.sortBy % (tPTR + 1)
				case ev.Ch == 'm':
					tState.sortBy = ntADDR
				case ev.Ch == 'f':
					tState.sortBy = ntDNSnr
				case ev.Ch == 'd':
					tState.sortBy = ntDNS
				case ev.Ch == 'n':
					tState.sortBy = ntNXDOM
				case ev.Ch == 'o':
					tState.sortBy = ntNOERR
				case ev.Ch == 's':
					tState.sortBy = ntSERVF
				case ev.Ch == 'a':
					tState.sortBy = ntA
				case ev.Ch == 'b':
					tState.sortBy = ntAAAA
				case ev.Ch == 'p':
					tState.sortBy = ntPTR
				case ev.Ch == 'w':
					tState.byQueryAddr = !tState.byQueryAddr
					tState.data = displayMapToNetTop(tState.rawData, tState.byQueryAddr)
				case ev.Ch == 'x':
					tState.oldShow = !tState.oldShow
				case ev.Ch == 'q' || ev.Key == termbox.KeyCtrlC || ev.Key == termbox.KeyCtrlZ:
					termbox.Close()
					stopChan <- true
					return
				}
				tState.refreshScreen()
			}
		case t := <-refreshChan:
			tState.rawData = t
			tState.data = displayMapToNetTop(t, tState.byQueryAddr)
			tState.lastRefresh = time.Now()
			tState.refreshScreen()
		}
	}
}
