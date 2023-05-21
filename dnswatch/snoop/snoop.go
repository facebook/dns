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
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/google/gopacket/layers"
	mkdns "github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	// queueSize is the queue size for filterQueue and probeQueue
	queueSize = 1000
)

// PortNr is a type for port number
type PortNr uint16

// PortNr associates identifier with port number
const (
	dns PortNr = 53
)

// PortToProtocol maps port to protocol string
var PortToProtocol = map[PortNr]string{
	dns: "dns",
}

// HostData stores data about the host machine
type HostData struct {
	Name   string
	Prefix string
}

// Config contains data needed for setup
type Config struct {
	Host           HostData
	LogLevel       string
	Interface      string
	Port           int
	FilterDebug    bool
	ProbeDebug     bool
	RingSizeMB     int
	CleanPeriod    time.Duration
	Fields         string
	ExporterListen string
	Exporter       bool
	Detailed       bool
	Toplike        bool
	NetTop         bool
	Sqllike        bool
	Csv            string
	Where          string
	Orderby        string
	Groupby        string
}

// Run setups and starts BPF filter and BPF probe
func Run(c *Config) error {
	var err error
	c.Host.Name, err = os.Hostname()
	if err != nil {
		return fmt.Errorf("unable to get hostname: %w", err)
	}
	re := regexp.MustCompile(`^[[:alpha:]]*`)
	c.Host.Prefix = string(re.Find([]byte(c.Host.Name)))

	decoder, err := RawDecoderByType(PortToProtocol[PortNr(c.Port)])
	if err != nil {
		return fmt.Errorf("unable to get decoder: %w", err)
	}

	bpfProbe := &Probe{
		Port:      c.Port,
		Debug:     c.ProbeDebug,
		setupDone: make(chan bool, 1),
	}

	bpfFilter := &Filter{
		Rule:       decoder.BPFrule(),
		Debug:      c.FilterDebug,
		Interface:  c.Interface,
		RingSizeMB: c.RingSizeMB,
	}
	if err := bpfFilter.Setup(); err != nil {
		return fmt.Errorf("unable to setup BPF filter: %w", err)
	}

	fields, err := ParseFields(c.Fields)
	if err != nil {
		return fmt.Errorf("unable to parse fields string: %w", err)
	}

	consumer := &Consumer{
		Config: c,
		Fields: fields,
	}
	consumer.Setup()

	// if ANY goroutine finishes wg will unblock
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// if we want to isolate debug filter or probe, do not run consumer
		if !c.FilterDebug && !c.ProbeDebug {
			defer wg.Done()
			consumer.Watch()
		}
	}()
	go func() {
		defer wg.Done()
		if err := bpfProbe.Run(consumer.probeQueue); err != nil {
			log.Errorf("unable to run BPF probe: %v\n", err)
		}
	}()
	go func() {
		defer wg.Done()
		// wait for the probe setup
		<-bpfProbe.setupDone
		if err := bpfFilter.Run(decoder, consumer.filterQueue); err != nil {
			log.Errorf("unable to run BPF filter: %v\n", err)
		}
	}()
	wg.Wait()
	// quick-and-dirty hack to stop dnswatch immediately.
	// sadly there is some softlock condition with all the queues
	// when we exit during high DNS packet throughput, and fixing it
	// requires proper refactoring with contexts and stuff.
	os.Exit(0)
	return nil
}

// FilterDTO is a data transfer object used to communicate between filter and consumer
type FilterDTO struct {
	Timestamp int64
	SrcPort   uint16
	SrcAddr   net.IP
	DstPort   uint16
	DstAddr   net.IP
	DNS       *layers.DNS
}

// ProbeDTO is a data transfer object used to communicate between probe and consumer
type ProbeDTO struct {
	ProbeData ProbeEventData
}

// ProcInfo stores data about a process
type ProcInfo struct {
	pid     int
	tid     int
	pName   string
	cmdline string
}

// UniqueDNS uniquely identifies DNS pair query-response
type UniqueDNS struct {
	port  PortNr
	dnsID int
}

// Consumer listens on both probe and filter queues
type Consumer struct {
	Config *Config
	Fields []FieldID

	probeQueue  chan *ProbeDTO
	filterQueue chan *FilterDTO

	portToProcess map[PortNr]*ProcInfo
	displayMap    map[UniqueDNS]*DisplayInfo

	toplikeQueue  chan *ToplikeData
	netTopQueue   chan map[UniqueDNS]*DisplayInfo
	exporterQueue chan *ToplikeData

	toplikeStop chan bool
	sqllikeStop chan bool
}

// Setup initializes Consumer queues and maps
func (c *Consumer) Setup() {
	c.filterQueue = make(chan *FilterDTO, queueSize)
	c.probeQueue = make(chan *ProbeDTO, queueSize)

	c.displayMap = make(map[UniqueDNS]*DisplayInfo)
	c.portToProcess = make(map[PortNr]*ProcInfo)

	c.toplikeQueue = make(chan *ToplikeData, 1)
	c.netTopQueue = make(chan map[UniqueDNS]*DisplayInfo, 1)
	c.exporterQueue = make(chan *ToplikeData, 1)

	c.toplikeStop = make(chan bool, 1)
	c.sqllikeStop = make(chan bool, 1)
}

// handleProcessData populates portToProcess map
func (c *Consumer) handleProcessData(pData *ProbeDTO) {
	c.portToProcess[PortNr(pData.ProbeData.SockPortNr)] = &ProcInfo{
		pid:     int(pData.ProbeData.Tgid),
		tid:     int(pData.ProbeData.Pid),
		pName:   unix.ByteSliceToString(pData.ProbeData.Comm[:]),
		cmdline: unix.ByteSliceToString(pData.ProbeData.Cmdline[:]),
	}
}

// handleDNSData called to match and insert into displayMap
// (DNS query, DNS response, calling process)
func (c *Consumer) handleDNSData(fData *FilterDTO) {
	uniq := UniqueDNS{
		dnsID: int(fData.DNS.ID),
		port:  PortNr(fData.DstPort),
	}
	if int(uniq.port) == c.Config.Port {
		uniq.port = PortNr(fData.SrcPort)
	}

	if c.displayMap[uniq] == nil {
		c.displayMap[uniq] = &DisplayInfo{
			fields: c.Fields,
		}
	}

	// Query
	if !fData.DNS.QR {
		c.displayMap[uniq].qTimestamp = fData.Timestamp
		c.displayMap[uniq].query = fData.DNS
		if fData.SrcAddr != nil && c.displayMap[uniq].queryAddr == nil {
			c.displayMap[uniq].queryAddr = fData.SrcAddr
		}
		if fData.DstAddr != nil && c.displayMap[uniq].responseAddr == nil {
			c.displayMap[uniq].responseAddr = fData.DstAddr
		}
	} else { // response
		c.displayMap[uniq].rTimestamp = fData.Timestamp
		c.displayMap[uniq].response = fData.DNS
		if fData.SrcAddr != nil && c.displayMap[uniq].responseAddr == nil {
			c.displayMap[uniq].responseAddr = fData.SrcAddr
		}
		if fData.DstAddr != nil && c.displayMap[uniq].queryAddr == nil {
			c.displayMap[uniq].queryAddr = fData.DstAddr
		}
	}
	if c.portToProcess[uniq.port] != nil {
		c.displayMap[uniq].ProcInfo = *c.portToProcess[uniq.port]
	}
	if c.displayMap[uniq].response != nil &&
		c.displayMap[uniq].query != nil &&
		c.portToProcess[uniq.port] != nil {
		c.handlePrint(c.displayMap[uniq])

		// exporter, toplike and sqllike print data only on refresh
		if !c.Config.Toplike && !c.Config.Sqllike && !c.Config.NetTop && !c.Config.Exporter {
			delete(c.displayMap, uniq)
		}
	}
}

// Watch listens on Consumer queues
func (c *Consumer) Watch() {
	c.handleOutputSetup()

	ticker := time.NewTicker(c.Config.CleanPeriod)
	defer ticker.Stop()
	for {
		select {
		case pData := <-c.probeQueue:
			c.handleProcessData(pData)
		case fData := <-c.filterQueue:
			c.handleDNSData(fData)
		case <-ticker.C:
			c.handleRefresh()
		case <-c.toplikeStop:
			return
		case <-c.sqllikeStop:
			return
		}
	}
}

func copyDisplayMap(dm map[UniqueDNS]*DisplayInfo) map[UniqueDNS]*DisplayInfo {
	newDM := make(map[UniqueDNS]*DisplayInfo)
	for k, v := range dm {
		newDM[k] = v
	}
	return newDM
}

// CleanDisplayMap displays to stdout the attribute displayMap
func (c *Consumer) CleanDisplayMap() {
	for k := range c.displayMap {
		if c.portToProcess[k.port] != nil {
			c.displayMap[k].ProcInfo = *c.portToProcess[k.port]
		}
		c.handlePrint(c.displayMap[k])
	}
	// Clear only the displayMap, because the ports can be reused
	c.displayMap = make(map[UniqueDNS]*DisplayInfo)
}

func (c *Consumer) handleOutputSetup() {
	if c.Config.Sqllike {
		return
	}
	if c.Config.Toplike {
		go StartTopLike(c.toplikeQueue, c.toplikeStop, c.Config.CleanPeriod)
		return
	}
	if c.Config.NetTop {
		go StartNetTop(c.netTopQueue, c.toplikeStop, c.Config.CleanPeriod)
		return
	}
	if c.Config.Detailed {
		log.Infof("DETAILED DIG LIKE DISPLAY")
		return
	}
	if c.Config.Exporter {
		go startPrometheusExporter(c.exporterQueue, c.Config.ExporterListen)
		return
	}
	log.Infof(DisplayHeader(c.Fields))
}

func (c *Consumer) handlePrint(d *DisplayInfo) {
	// toplike and sqllike print on refresh
	if c.Config.Toplike || c.Config.Sqllike || c.Config.NetTop || c.Config.Exporter {
		return
	}
	if c.Config.Detailed {
		log.Infof(d.DetailedString())
		return
	}
	log.Infof(d.String())
}

func (c *Consumer) handleRefresh() {
	if c.Config.Sqllike {
		c.computeDataframe()
		return
	}
	if c.Config.Exporter {
		c.exporterQueue <- c.displayMapToToplike()
		c.CleanDisplayMap()
		return
	}
	if c.Config.Toplike {
		c.toplikeQueue <- c.displayMapToToplike()
		return
	}
	if c.Config.NetTop {
		c.netTopQueue <- copyDisplayMap(c.displayMap)
		return
	}
	c.CleanDisplayMap()
}

// displayMapToToplike populates a ToplikeData struct with displayMap data
func (c *Consumer) displayMapToToplike() *ToplikeData {
	ret := &ToplikeData{}
	ret.Rows = make(map[int]*ToplikeRow)

	for _, v := range c.displayMap {
		ret.total++
		pName := v.pName
		pid := v.pid
		if v.pid == 0 {
			pid = -1
			pName = UNK
		}
		if ret.Rows[pid] == nil {
			ret.Rows[pid] = &ToplikeRow{
				PID:  pid,
				Comm: pName,
			}
		}
		ret.Rows[pid].rTimestamp = v.rTimestamp
		ret.Rows[pid].DNS.val++
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeNXDomain {
			ret.Rows[pid].NXDOM.val++
			ret.nxdom++
		}
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeNoErr {
			ret.Rows[pid].NOERR.val++
			ret.noerr++
		}
		if v.response != nil && v.response.ResponseCode == layers.DNSResponseCodeServFail {
			ret.Rows[pid].SERVF.val++
			ret.servf++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypeA {
			ret.Rows[pid].A.val++
			ret.a++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypeAAAA {
			ret.Rows[pid].AAAA.val++
			ret.aaaa++
		}
		if v.query != nil && len(v.query.Questions) > 0 && v.query.Questions[0].Type == layers.DNSTypePTR {
			ret.Rows[pid].PTR.val++
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

// computeDataframe populates and prints the Dataframe of sql subcommand
func (c *Consumer) computeDataframe() {
	df := &SqllikeData{
		Where:   c.Config.Where,
		Orderby: c.Config.Orderby,
		Groupby: c.Config.Groupby,
	}

	loadMaps := make([]map[string]interface{}, 0)
	for _, d := range c.displayMap {
		pid, pName, lat, qType, qName, rIP, rCode := 0, UNK, 0, UNK, UNK, UNK, UNK
		if d.pid != 0 {
			pName = d.pName
			pid = d.pid
		}
		if d.qTimestamp != 0 && d.rTimestamp != 0 {
			lat = int(d.rTimestamp-d.qTimestamp) / 1000
		}
		if d.query != nil && len(d.query.Questions) > 0 {
			qType = d.query.Questions[0].Type.String()
			qName = string(d.query.Questions[0].Name)
		} else if d.response != nil && len(d.response.Questions) > 0 {
			qType = d.response.Questions[0].Type.String()
			qName = string(d.response.Questions[0].Name)
		}
		if d.response != nil {
			for _, v := range d.response.Answers {
				if qType == v.Type.String() && v.IP != nil {
					rIP = v.IP.String()
				}
			}
			rCode = mkdns.RcodeToString[int(d.response.ResponseCode)]
		}
		lMap := map[string]interface{}{
			"PID":     pid,
			"PNAME":   pName,
			"LATENCY": lat,
			"QTYPE":   qType,
			"QNAME":   qName,
			"RIP":     rIP,
			"RCODE":   rCode,
			"QADDR":   d.queryAddr.String(),
			"RADDR":   d.responseAddr.String(),
		}
		loadMaps = append(loadMaps, lMap)
	}
	df.Setup(loadMaps)
	df.SolveWhere()
	df.SolveOrderby()
	df.SolveGroupby()
	df.Print(c.Config.Csv)
	c.sqllikeStop <- true
}
