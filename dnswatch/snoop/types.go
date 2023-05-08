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

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	// maxNrLayers is the maximum number of headers on a packet
	maxNrLayers = 10
)

// RawDecoderByType factory method used to return a specific type of RawDecoder
// To add a new packet type: add a new case & add a new struct that implements RawDecoder methods
func RawDecoderByType(pktType string) (RawDecoder, error) {
	switch pktType {
	case "dns":
		return &DNSDecoder{}, nil
	default:
		return nil, fmt.Errorf("No Decoder associated with this type of packet")
	}
}

// RawDecoder used to decode raw packets, starting with Eth layer
// DstPort and SrcPort are used to compute latency on a specific port
// Header and Row are used to print specific data for different protocols
type RawDecoder interface {
	// Unmarshal populates the struct of the specific packet
	Unmarshal([]byte) error

	// DstPort returns destination port of the packet
	DstPort() (int, error)

	// DstAddr returns destination address of the packet
	DstAddr() (net.IP, error)

	// SrcPort returns source port of the packet
	SrcPort() (int, error)

	// SrcAddr returns source address of the packet
	SrcAddr() (net.IP, error)

	// Header returns the titles for a specific port
	Header() []string

	// Row returns data about the packet in the same order as Header
	Row() ([]string, error)

	// BPFrule returns the bpf filter in bpf format
	BPFrule() string

	// Valid returns true if the packet contains valid data, false otherwise
	Valid() bool
}

// DNSDecoder used to decode DNS raw packets
type DNSDecoder struct {
	eth *layers.Ethernet
	ip4 *layers.IPv4
	ip6 *layers.IPv6
	tcp *layers.TCP
	udp *layers.UDP
	dns *layers.DNS

	layers []gopacket.LayerType
}

// BPFrule for DNS
func (d *DNSDecoder) BPFrule() string {
	return "src port 53 or dst port 53"
}

// Unmarshal populates the struct with specific DNS data
func (d *DNSDecoder) Unmarshal(data []byte) error {
	var eth layers.Ethernet
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var tcp layers.TCP
	var udp layers.UDP
	var dns layers.DNS
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ip4, &ip6, &tcp, &udp, &dns)
	d.eth = &eth
	d.ip4 = &ip4
	d.ip6 = &ip6
	d.tcp = &tcp
	d.udp = &udp
	d.dns = &dns
	d.layers = make([]gopacket.LayerType, 0, maxNrLayers)

	err := parser.DecodeLayers(data, &d.layers)
	if err != nil {
		return fmt.Errorf("unable to decode DNS packet: %w", err)
	}
	return nil
}

// Valid is true if the DNSDecoder contains a DNS packet
func (d *DNSDecoder) Valid() bool {
	if d == nil || d.dns == nil || d.dns.ID == 0 {
		return false
	}
	return true
}

// DstPort returns destination port of the packet
func (d *DNSDecoder) DstPort() (int, error) {
	if d.udp == nil && d.tcp == nil {
		return 0, fmt.Errorf("packet not decoded")
	}
	if d.udp != nil {
		return int(d.udp.DstPort), nil
	}
	return int(d.tcp.DstPort), nil
}

// DstAddr returns destination address of the packet
func (d *DNSDecoder) DstAddr() (net.IP, error) {
	if d.udp == nil && d.tcp == nil {
		return nil, fmt.Errorf("packet not decoded")
	}
	if d.ip4 != nil && d.ip4.DstIP != nil {
		return d.ip4.DstIP, nil
	}
	return d.ip6.DstIP, nil
}

// SrcPort returns source port of the packet
func (d *DNSDecoder) SrcPort() (int, error) {
	if d.udp == nil && d.tcp == nil {
		return 0, fmt.Errorf("packet not decoded")
	}
	if d.udp != nil {
		return int(d.udp.SrcPort), nil
	}
	return int(d.tcp.SrcPort), nil
}

// SrcAddr returns source address of the packet
func (d *DNSDecoder) SrcAddr() (net.IP, error) {
	if d.udp == nil && d.tcp == nil {
		return nil, fmt.Errorf("packet not decoded")
	}
	if d.ip4 != nil && d.ip4.SrcIP != nil {
		return d.ip4.SrcIP, nil
	}
	return d.ip6.SrcIP, nil
}

// Header returns DNS specific data headers
func (d *DNSDecoder) Header() []string {
	var ret []string
	// First question name
	ret = append(ret, "DNS Qry Name")
	// First answer IP
	ret = append(ret, "DNS Resp IP")
	return ret
}

// Row returns values ordered by fields in Header
func (d *DNSDecoder) Row() ([]string, error) {
	var ret []string
	qryName := ""
	rspIP := ""
	if d.dns == nil {
		return nil, fmt.Errorf("packet has no DNS data")
	}
	if len(d.dns.Questions) > 0 {
		qryName = string(d.dns.Questions[0].Name)
	}
	if len(d.dns.Answers) > 0 {
		rspIP = d.dns.Answers[0].IP.String()
	}
	ret = append(ret, qryName)
	ret = append(ret, rspIP)
	return ret, nil
}

// Print displays on stdout info about the packet
// Debug purposes
func (d *DNSDecoder) Print() {
	fmt.Printf("Layers: %v \n", d.layers)
	for _, typ := range d.layers {
		switch typ {
		case layers.LayerTypeEthernet:
			fmt.Println("  Ethernet type", d.eth.EthernetType)
			fmt.Println("  Ethernet SrcMAC", d.eth.SrcMAC.String())
			fmt.Println("  Ethernet DstMAC", d.eth.DstMAC.String())
		case layers.LayerTypeIPv4:
			fmt.Println("    Ipv4 SrcIp: ", d.ip4.SrcIP.String())
			fmt.Println("    Ipv4 DstIp: ", d.ip4.DstIP.String())
			fmt.Println("    Ipv4 Version: ", int(d.ip4.Version))
			fmt.Println("    Ipv4 Protocol: ", d.ip4.Protocol.String())
		case layers.LayerTypeIPv6:
			fmt.Println("    Ipv6 SrcIp: ", d.ip6.SrcIP.String())
			fmt.Println("    Ipv6 DstIp: ", d.ip6.DstIP.String())
			fmt.Println("    Ipv6 NextHeader: ", d.ip6.NextHeader.String())
		case layers.LayerTypeUDP:
			fmt.Println("      UDP SrcPort: ", d.udp.SrcPort.String())
			fmt.Println("      UDP DstPort: ", d.udp.DstPort.String())
		case layers.LayerTypeTCP:
			fmt.Println("      TCP SrcPort: ", d.tcp.SrcPort.String())
			fmt.Println("      TCP DstPort: ", d.tcp.DstPort.String())
		case layers.LayerTypeDNS:
			fmt.Println("        DNS OpCode: ", int(d.dns.OpCode))
			fmt.Println("        DNS ID: ", int(d.dns.ID))
			fmt.Println("        DNS QR (false-query, true-response): ", d.dns.QR)
			fmt.Println("        DNS ResponseCode: ", int(d.dns.ResponseCode))

			fmt.Println("        DNS Questions Nr: ", len(d.dns.Questions))
			for j, dnsQuestion := range d.dns.Questions {
				fmt.Println("          DNS Question # ", j)
				fmt.Println("          DNS Question Name ", string(dnsQuestion.Name))
				fmt.Println("          DNS Question Type", dnsQuestion.Type.String())
				fmt.Println("          DNS Question Class", dnsQuestion.Class.String())
			}
			fmt.Println("        DNS Answers Nr: ", len(d.dns.Answers))
			for j, dnsAnswer := range d.dns.Answers {
				fmt.Println("          DNS Answer # ", j)
				fmt.Println("          DNS Answer Name ", string(dnsAnswer.Name))
				fmt.Println("          DNS Answer Type ", dnsAnswer.Type.String())
				fmt.Println("          DNS Answer Class ", dnsAnswer.Class.String())
				fmt.Println("          DNS Answer IP ", dnsAnswer.IP.String())
			}
		}
	}
}
