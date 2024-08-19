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
	"bytes"
	_ "embed" // Embed needs to be imported for the []byte containing the embedded Bpf object
	"errors"

	"encoding/binary"

	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/aquasecurity/libbpfgo"
	log "github.com/sirupsen/logrus"
)

//go:embed out/dnswatch_bpf_probe_core.o
var bpfObjBuf []byte

// FnID identifier for function names
type FnID uint8

// FnID associated with each function
const (
	udpv6ID FnID = iota
	udpID
	tcpID
)

// used to cache pid to Comm translation
var pidToCommCache map[uint32][commLength]byte

// used to cache pid to CmdLine translation
var pidToCmdLineCache map[uint32][cmdlineLength]byte

// fnIDToFnName maps FnID to kernel function name
var fnIDToFnName = map[FnID]string{
	// kernel function name used to send udpv6 packets
	udpv6ID: "udpv6_sendmsg",

	// kernel function name used to send udpv4 packets
	udpID: "udp_sendmsg",

	// kernel function name used to send tcpv6 and tcpv4 packets
	tcpID: "tcp_sendmsg",
}

const (
	// maxChanSize is the max number of probe events on the channel
	maxChanSize = 10000

	// commLength is the max len for a task comm
	// must be a value >= TASK_COMM_LEN
	commLength = 80

	// cmdlineLength is the max len for cmdline
	cmdlineLength = 120

	// argLength is the max len of a single arg
	argLength = 30
)

// ProbeEventData is a struct populated with data from kernel
// It must match the struct in the BPF program
type ProbeEventData struct {
	// Tgid is the thread group id
	Tgid uint32
	// Pid is the process id
	Pid uint32
	// SockPortNr is the socket number used to send_msg
	SockPortNr int32
	// FnID is the identifier of the function
	FnID uint8
}

// EnhancedProbeData is an extended version of the ProbeEventData struct populated with data from kernel
// which is then later enhanced by comm and cmdline by the userspace code
type EnhancedProbeData struct {
	// Tgid is the thread group id
	Tgid uint32
	// Pid is the process id
	Pid uint32
	// // Comm is the task comm
	Comm [commLength]byte
	// // Cmdline is the process cmdline
	Cmdline [cmdlineLength]byte
	// SockPortNr is the socket number used to send_msg
	SockPortNr int32
	// FnID is the identifier of the function
	FnID uint8
}

// Probe is the BPF handler which attaches kprobes to kernel functions
// It receives kernel data each time one of these functions is called
type Probe struct {
	setupDone chan bool

	Port  int
	Debug bool
}

// cleanCmdline used to clean cmdline from kernel
func cleanCmdline(str [cmdlineLength]byte) [cmdlineLength]byte {
	var ret [cmdlineLength]byte
	copyval := true
	retIndex := 0
	for i, v := range str {
		// i in arg start position
		if i%argLength == 0 {
			copyval = true
		}
		// 0 terminated and we are inside an argument
		if v == 0 && copyval {
			ret[retIndex] = ' '
			retIndex++
			copyval = false
		}
		// if we are inside an arg, we copy the value
		if copyval {
			ret[retIndex] = v
			retIndex++
		}
	}
	return ret
}

func determineHostByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}

	return binary.BigEndian
}

// loadAndAttachProbes setups the probes
func (p *Probe) loadAndAttachProbes() (*libbpfgo.Module, error) {
	libbpfgo.SetLoggerCbs(libbpfgo.Callbacks{
		LogFilters: []func(libLevel int, msg string) bool{
			func(_ int, _ string) bool {
				return !p.Debug
			},
		},
	})

	bpfModule, err := libbpfgo.NewModuleFromBuffer(bpfObjBuf, "dnswatch_bpf_probe_core")
	if err != nil {
		return nil, err
	}
	err = bpfModule.BPFLoadObject()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	for _, kernelFnName := range fnIDToFnName {
		probeFnName := "dnswatch_kprobe_" + kernelFnName
		kprobe, err := bpfModule.GetProgram(probeFnName)
		if err != nil {
			return nil, fmt.Errorf("unable to load fentry/"+kernelFnName+": %w", err)
		}
		kprobelink, err := kprobe.AttachGeneric()
		if err != nil {
			return nil, fmt.Errorf("unable attaching fentry/"+kernelFnName+": %w", err)
		}
		if kprobelink.FileDescriptor() == 0 {
			return nil, errors.New("kprobe/" + kernelFnName + "not running.")
		}
	}
	return bpfModule, nil
}

// Run is used to setup and listen for probe events
// To avoid kernel memory leaks, the Run method will setup
// the bpfModule, and defer the Close method
func (p *Probe) Run(ch chan<- *ProbeDTO) error {
	pidToCommCache = make(map[uint32][commLength]byte)
	pidToCmdLineCache = make(map[uint32][cmdlineLength]byte)

	bpfModule, err := p.loadAndAttachProbes()
	if err != nil {
		return fmt.Errorf("unable to loadAndAttachProbes: %w", err)
	}
	defer bpfModule.Close()

	// Setup for capturing events. Table is used to store events from probe
	// and channel is used to get the table information

	channel := make(chan []byte, maxChanSize)
	perfMap, err := bpfModule.InitRingBuf("dnswatch_kprobe_output_events", channel)
	if err != nil {
		return fmt.Errorf("unable to init perf map: %w", err)
	}

	// Setup a clean up mechanism for kernel resources if SIGINT or SIGKILL is received
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		if p.Debug {
			log.Infof("%10v %10v %10v   %s   -(call)->   %s", "PID", "TGID", "SK_PORT_NR", "COMM", "FN_NAME")
		}
		for {
			data := <-channel

			var event ProbeEventData
			var enhanchedEvent EnhancedProbeData
			err := binary.Read(bytes.NewBuffer(data), determineHostByteOrder(), &event)
			if err != nil {
				if p.Debug {
					log.Printf("unable to read BPF data: %v", err)
				}
				continue
			}
			enhanchedEvent.Pid = event.Pid
			enhanchedEvent.Tgid = event.Tgid
			enhanchedEvent.FnID = event.FnID
			enhanchedEvent.SockPortNr = event.SockPortNr
			enhanchedEvent.Comm, err = getProcComm(event.Pid)

			if err != nil {
				if p.Debug {
					log.Printf("unable to read event cmd: %v", err)
				}
			}
			cmdLine, err := getProcCmdLine(event.Pid)
			if err != nil {
				if p.Debug {
					log.Printf("unable to read event cmd: %v", err)
				}
			}
			enhanchedEvent.Cmdline = cleanCmdline(cmdLine)

			if p.Debug {
				log.Printf("%10v %10v %10v   %s   -(call)->   %s %s", enhanchedEvent.Pid, enhanchedEvent.Tgid,
					enhanchedEvent.SockPortNr, enhanchedEvent.Comm[:15], fnIDToFnName[FnID(enhanchedEvent.FnID)], enhanchedEvent.Cmdline)
				continue
			}

			ch <- &ProbeDTO{
				ProbeData: enhanchedEvent,
			}
		}
	}()

	perfMap.Start()
	// signal that setup is done
	p.setupDone <- true
	<-sig
	perfMap.Stop()
	perfMap.Close()

	return nil
}

func getProcComm(pid uint32) ([commLength]byte, error) {
	cachedComm, found := pidToCommCache[pid]
	// If the key exists
	if found {
		return cachedComm, nil
	}
	path := fmt.Sprintf("/proc/%d/comm", pid)
	var retbuf [commLength]byte

	f, err := os.Open(path)
	if err != nil {
		return retbuf, err
	}
	defer f.Close()

	reader := io.LimitReader(f, commLength)
	buf, err := io.ReadAll(reader)
	if err != nil {
		return retbuf, err
	}
	copy(retbuf[:], buf)
	pidToCommCache[pid] = retbuf

	return retbuf, nil
}

func getProcCmdLine(pid uint32) ([cmdlineLength]byte, error) {
	cachedCmdLine, found := pidToCmdLineCache[pid]
	// If the key exists
	if found {
		return cachedCmdLine, nil
	}
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	var retbuf [cmdlineLength]byte

	f, err := os.Open(path)
	if err != nil {
		return retbuf, err
	}
	defer f.Close()

	reader := io.LimitReader(f, cmdlineLength)
	buf, err := io.ReadAll(reader)
	if err != nil {
		return retbuf, err
	}
	copy(retbuf[:], buf)
	pidToCmdLineCache[pid] = retbuf

	return retbuf, nil
}
