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

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/bpf"
)

const (
	// snapLen is the maximum length (bytes) of the raw packet
	snapLen = 65535
)

// Filter is the BPF filter + Packet MMAPer used to receive and read packets in UserSpace.
type Filter struct {
	Rule       string
	Interface  string
	Debug      bool
	RingSizeMB int

	tPacket *afpacket.TPacket
}

// computeRingSizes computes the blockSize and the numBlocks in such a way that the
// allocated mmap buffer is close to but smaller than ringTargetSize.
// The restriction is that the blockSize must be divisible by both the frame size and page size.
// Ring contains blocks. Blocks contain frames.
// Blocks are allocated with calls to the __get_free_pages().
func computeRingSizes(ringTargetSize int, snapLen int, pageSize int) (frameSize int, blockSize int, numBlocks int, err error) {
	// Make frameSize divisible by pageSize. Padding may be needed
	if snapLen < pageSize {
		frameSize = pageSize / (pageSize / snapLen)
	} else {
		frameSize = ((snapLen / pageSize) + 1) * pageSize
	}

	blockSize = frameSize * afpacket.DefaultNumBlocks
	numBlocks = ringTargetSize / blockSize

	if numBlocks == 0 {
		return 0, 0, 0, fmt.Errorf("ringSize is too small")
	}

	return frameSize, blockSize, numBlocks, nil
}

// SetBPFFilter translates a BPF filter string into BPF RawInstruction and applies them.
func SetBPFFilter(h *afpacket.TPacket, filter string, snapLen int) error {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, snapLen, filter)
	if err != nil {
		return err
	}

	bpfProgram := []bpf.RawInstruction{}
	for _, ins := range pcapBPF {
		// Each BPF raw instruction contains instruction code, jump true, jump false and k (immediate value)
		bpfInstr := bpf.RawInstruction{Op: ins.Code, Jt: ins.Jt, Jf: ins.Jf, K: ins.K}
		bpfProgram = append(bpfProgram, bpfInstr)
	}

	return h.SetBPF(bpfProgram)
}

// Setup used to set the BPF Filter and map the ring buffer in memory
func (f *Filter) Setup() error {
	frameSize, blockSize, numBlocks, err := computeRingSizes(MBtoB(f.RingSizeMB), snapLen, os.Getpagesize())
	if err != nil {
		return fmt.Errorf("unable to compute the size of the ring buffer: %w", err)
	}

	if f.Interface == "" {
		f.tPacket, err = afpacket.NewTPacket(afpacket.OptFrameSize(frameSize), afpacket.OptBlockSize(blockSize),
			afpacket.OptNumBlocks(numBlocks), afpacket.OptPollTimeout(pcap.BlockForever), afpacket.SocketRaw, afpacket.TPacketVersion3)
	} else {
		f.tPacket, err = afpacket.NewTPacket(afpacket.OptInterface(f.Interface), afpacket.OptFrameSize(frameSize), afpacket.OptBlockSize(blockSize),
			afpacket.OptNumBlocks(numBlocks), afpacket.OptPollTimeout(pcap.BlockForever), afpacket.SocketRaw, afpacket.TPacketVersion3)
	}
	if err != nil {
		return fmt.Errorf("unable to create new TPacket object : %w", err)
	}

	if err := SetBPFFilter(f.tPacket, f.Rule, snapLen); err != nil {
		return fmt.Errorf("unable to set BPF filter: %w", err)
	}
	return nil
}

// Run is used to start reading the packets received by BPF
func (f *Filter) Run(decoder RawDecoder, ch chan<- *FilterDTO) error {
	source := gopacket.ZeroCopyPacketDataSource(f.tPacket)
	defer f.tPacket.Close()

	for {
		data, capInfo, err := source.ZeroCopyReadPacketData()
		if err != nil {
			return fmt.Errorf("unable to read packet data from ring buffer: %w", err)
		}

		if err := decoder.Unmarshal(data); err != nil {
			if f.Debug {
				log.Printf("unable to unmarshal raw packet data: %v", err)
			}
			continue
		}

		if !decoder.Valid() {
			if f.Debug {
				log.Printf("packet received at %v is not valid", capInfo.Timestamp.UnixNano())
			}
			continue
		}

		if f.Debug {
			log.Printf("Timestamp = %v (unix nano)", capInfo.Timestamp.UnixNano())
			decoder.(*DNSDecoder).Print()
			continue
		}

		sPort, err := decoder.SrcPort()
		if err != nil {
			log.Warningf("unable to get packet srcport: %v", err)
			continue
		}
		sAddr, err := decoder.SrcAddr()
		if err != nil {
			log.Warningf("unable to get packet srcaddr: %v", err)
			continue
		}
		dPort, err := decoder.DstPort()
		if err != nil {
			log.Warningf("unable to get packet dstport: %v", err)
			continue
		}
		dAddr, err := decoder.DstAddr()
		if err != nil {
			log.Warningf("unable to get packet dstaddr: %v", err)
			continue
		}
		// deep copy must be used to ensure that the packet pointer is not in the ring buffer
		dAddrCopy := make(net.IP, len(dAddr))
		copy(dAddrCopy, dAddr)
		sAddrCopy := make(net.IP, len(sAddr))
		copy(sAddrCopy, sAddr)
		ch <- &FilterDTO{
			Timestamp: capInfo.Timestamp.UnixNano(),
			DstAddr:   dAddrCopy,
			DstPort:   uint16(dPort),
			SrcPort:   uint16(sPort),
			SrcAddr:   sAddrCopy,
			DNS:       deepCopyDNS(decoder.(*DNSDecoder).dns),
		}
	}
}

// MBtoB converts megabytes to bytes
func MBtoB(b int) int {
	return b * 1024 * 1024
}

func deepCopyBytes(in []byte) []byte {
	cpy := make([]byte, len(in))
	copy(cpy, in)
	return cpy
}

func deepCopyQuestions(in []layers.DNSQuestion) []layers.DNSQuestion {
	cpy := make([]layers.DNSQuestion, len(in))
	copy(cpy, in)
	for i := range in {
		cpy[i].Name = deepCopyBytes(in[i].Name)
	}
	return cpy
}

func deepCopyRRs(in []layers.DNSResourceRecord) []layers.DNSResourceRecord {
	cpy := make([]layers.DNSResourceRecord, len(in))
	copy(cpy, in)
	for i := range in {
		cpy[i].Name = deepCopyBytes(in[i].Name)
		cpy[i].Data = deepCopyBytes(in[i].Data)
		cpy[i].NS = deepCopyBytes(in[i].NS)
		cpy[i].CNAME = deepCopyBytes(in[i].CNAME)
		cpy[i].PTR = deepCopyBytes(in[i].PTR)
		cpy[i].IP = deepCopyBytes(in[i].IP)
	}
	return cpy
}

func deepCopyDNS(d *layers.DNS) *layers.DNS {
	ret := *d

	ret.Questions = deepCopyQuestions(d.Questions)
	ret.Answers = deepCopyRRs(d.Answers)
	ret.Authorities = deepCopyRRs(d.Authorities)
	ret.Additionals = deepCopyRRs(d.Additionals)

	return &ret
}
