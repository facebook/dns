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
	"strconv"
	"time"

	termbox "github.com/nsf/termbox-go"
	log "github.com/sirupsen/logrus"
)

const coldef = termbox.ColorDefault

const doc = `dnswatch - DNS snooping
SORTBY keys - m (PID), c (COMM), f (DNS), d (%DNSTRAFFIC), n (NXDOMAIN), o (NOERROR), s (SERVFAIL), a (A), b (AAAA), p (PTR)
SORTBY keys - < (MOVE SORTING COL LEFT), > (MOVE SORTING COL RIGHT)
TOGGLE keys - w (PID/COMM AGGREGATE), x (KEEP TERMINATED PROCESSES / DELETE OLD PROCESSES REQUEST)
NUMBERS AGGREGATED SINCE THE START OF THE RUN
PID | COMM(process Name) | DNS(proc traffic) | %DNS(% of DNS traffic) | %NXDOMAIN | %NOERROR | %SERVFAIL | %A | %AAAA |%PTR
`

const (
	tPID = iota
	tCOMM
	tDNSnr
	tDNS
	tNXDOM
	tNOERR
	tSERVF
	tA
	tAAAA
	tPTR
)

// percentField used to store pair value, percentage
type percentField struct {
	val int
	per float64
}

func (p *percentField) computePerc(total int) {
	p.per = float64(p.val) / float64(total) * 100
}

// ToplikeRow contains data about each row in toplike display
type ToplikeRow struct {
	PID  int
	Comm string

	DNS   percentField
	NXDOM percentField
	NOERR percentField
	SERVF percentField
	A     percentField
	AAAA  percentField
	PTR   percentField

	rTimestamp int64
}

// ToplikeData contains the entire toplike display table
type ToplikeData struct {
	// PID to row
	Rows  map[int]*ToplikeRow
	total int
	nxdom int
	noerr int
	servf int
	a     int
	aaaa  int
	ptr   int
}

// oldFilter keeps only the new DNS requests on screen
func (t *ToplikeData) oldFilter(per time.Duration) *ToplikeData {
	ret := *t
	newRows := make(map[int]*ToplikeRow)
	for _, v := range t.Rows {
		if v.rTimestamp >= time.Now().UnixNano()-per.Nanoseconds() {
			newRows[v.PID] = v
		}
	}
	ret.Rows = newRows
	return &ret
}

// aggregateComm to return a map aggregated by Comm, not by pid
func (t *ToplikeData) aggregateComm() *ToplikeData {
	ret := *t
	aux := make(map[string]*ToplikeRow)
	newRows := make(map[int]*ToplikeRow)
	for _, v := range t.Rows {
		if aux[v.Comm] == nil {
			aux[v.Comm] = &ToplikeRow{}
		}
		aux[v.Comm].Comm = v.Comm
		aux[v.Comm].PID = v.PID
		aux[v.Comm].DNS.val += v.DNS.val
		aux[v.Comm].NXDOM.val += v.NXDOM.val
		aux[v.Comm].NOERR.val += v.NOERR.val
		aux[v.Comm].SERVF.val += v.SERVF.val
		aux[v.Comm].A.val += v.A.val
		aux[v.Comm].AAAA.val += v.AAAA.val
		aux[v.Comm].PTR.val += v.PTR.val
	}
	for _, v := range aux {
		v.DNS.computePerc(t.total)
		v.NXDOM.computePerc(v.DNS.val)
		v.NOERR.computePerc(v.DNS.val)
		v.SERVF.computePerc(v.DNS.val)
		v.A.computePerc(v.DNS.val)
		v.AAAA.computePerc(v.DNS.val)
		v.PTR.computePerc(v.DNS.val)
		newRows[v.PID] = v
	}
	ret.Rows = newRows
	return &ret
}

// ToplikeState is the current state of the interactive env
type ToplikeState struct {
	data    *ToplikeData
	cliRows int

	sortBy  int
	pidShow bool
	oldShow bool

	startTime   time.Time
	lastRefresh time.Time
	refTime     time.Duration
}

func (t *ToplikeState) printDocumentation() {
	fmt.Printf("%v", doc)
}

func (t *ToplikeState) printAggregated() {
	dateFormat := "2006-01-02 15:04:05.000"
	fmt.Printf("\nSTART TIME: %10v, LAST REFRESH: %10v\n", t.startTime.Format(dateFormat), t.lastRefresh.Format(dateFormat))
	fmt.Printf("%-19v: %10v\n", "DNS TRAFFIC (Q-R)", t.data.total)
	fmt.Printf("%-19v: %10v, %-19v: %10v, %-19v: %10v\n", "A QUERIES", t.data.a, "AAAA QUERIES", t.data.aaaa, "PTR QUERIES", t.data.ptr)
	fmt.Printf("%-19v: %10v, %-19v: %10v, %-19v: %10v\n", "NXDOMAIN RESPONSES", t.data.nxdom, "NOERROR RESPONSES", t.data.noerr, "SERVFAIL RESPONSES", t.data.servf)
}

func (t *ToplikeState) printData(maxRows int) {
	formatHeader := "%-10v  %-15v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v  %-9v\n"
	formatRow := "%-10v  %-15.14v  %-9v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v  %-9.4v\n"
	fmt.Printf("%-10v  %-15v  %-20v  %-31v  %-31v\n", "", "", "<----TOTAL---->", "<------------RCODE------------>", "<----------QNAME--------->")
	fmt.Printf(formatHeader, "PID", "COMM", "DNS", "%DNS", " %NXDOMAIN", "%NOERROR", " %SERVFAIL", "%A", " %AAAA", "  %PTR")
	var topMap *ToplikeData
	topMap = t.data
	if !t.oldShow {
		topMap = topMap.oldFilter(t.refTime)
	}
	if !t.pidShow {
		topMap = topMap.aggregateComm()
	}

	if topMap.Rows != nil {
		vals := make([]*ToplikeRow, 0, len(topMap.Rows))
		for _, v := range topMap.Rows {
			vals = append(vals, v)
		}
		sort.Slice(vals, func(i, j int) bool {
			switch t.sortBy {
			case tPID:
				return vals[i].PID > vals[j].PID
			case tCOMM:
				return vals[i].Comm > vals[j].Comm
			case tDNSnr:
				return vals[i].DNS.val > vals[j].DNS.val
			case tDNS:
				return vals[i].DNS.per > vals[j].DNS.per
			case tNXDOM:
				return vals[i].NXDOM.per > vals[j].NXDOM.per
			case tNOERR:
				return vals[i].NOERR.per > vals[j].NOERR.per
			case tSERVF:
				return vals[i].SERVF.per > vals[j].SERVF.per
			case tA:
				return vals[i].A.per > vals[j].A.per
			case tAAAA:
				return vals[i].AAAA.per > vals[j].AAAA.per
			case tPTR:
				return vals[i].PTR.per > vals[j].PTR.per
			default:
				return vals[i].Comm > vals[j].Comm
			}
		})
		last := int(math.Min(float64(len(vals)-1), float64(maxRows)))
		for i := 0; i <= last; i++ {
			v := vals[i]
			pid := strconv.Itoa(v.PID)
			if v.PID <= 0 || !t.pidShow {
				pid = ""
			}
			fmt.Printf(formatRow, pid, v.Comm, v.DNS.val, v.DNS.per, v.NXDOM.per, v.NOERR.per, v.SERVF.per, v.A.per, v.AAAA.per, v.PTR.per)
		}
	}
}

func (t *ToplikeState) refreshScreen() {
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
	headerLines := 15
	t.printData(t.cliRows - headerLines)

	termbox.HideCursor()
	termbox.Flush()
}

// StartTopLike is the toplike stdout handler
func StartTopLike(refreshChan <-chan *ToplikeData, stopChan chan<- bool, refTime time.Duration) {
	rand.Seed(time.Now().UnixNano())

	err := termbox.Init()
	if err != nil {
		log.Error("failed to initialize screen")
	}
	defer termbox.Close()

	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	tState := &ToplikeState{
		data:        &ToplikeData{},
		startTime:   time.Now(),
		lastRefresh: time.Now(),
		pidShow:     true,
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
					if tState.sortBy < tPID {
						tState.sortBy = tPTR
					}
				case ev.Ch == '>':
					tState.sortBy++
					tState.sortBy = tState.sortBy % (tPTR + 1)
				case ev.Ch == 'm':
					tState.sortBy = tPID
				case ev.Ch == 'c':
					tState.sortBy = tCOMM
				case ev.Ch == 'f':
					tState.sortBy = tDNSnr
				case ev.Ch == 'd':
					tState.sortBy = tDNS
				case ev.Ch == 'n':
					tState.sortBy = tNXDOM
				case ev.Ch == 'o':
					tState.sortBy = tNOERR
				case ev.Ch == 's':
					tState.sortBy = tSERVF
				case ev.Ch == 'a':
					tState.sortBy = tA
				case ev.Ch == 'b':
					tState.sortBy = tAAAA
				case ev.Ch == 'p':
					tState.sortBy = tPTR
				case ev.Ch == 'w':
					tState.pidShow = !tState.pidShow
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
			tState.data = t
			tState.lastRefresh = time.Now()
			tState.refreshScreen()
		}
	}
}
