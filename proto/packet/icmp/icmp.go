// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package icmp implements ICMPv4 and ICMPv6 packet construction and MTU signaling (RFC 792, RFC 4443, RFC 1191, RFC 4884).
package icmp

import (
	"encoding/binary"
	"errors"
	"net/netip"

	"github.com/lemon4ksan/mach/proto/packet"
)

const (
	// IPv4MinMTU is the minimum MTU required by RFC 791 (68 octets).
	IPv4MinMTU = 68
	// IPv6MinMTU is the minimum MTU required by RFC 8200 (1280 octets).
	IPv6MinMTU = 1280

	// ICMPv4 Type and Code constants.
	ICMPv4TypeDestinationUnreachable = 3
	ICMPv4CodeFragmentationNeeded    = 4

	// ICMPv6 Type and Code constants.
	ICMPv6TypePacketTooBig = 2
	ICMPv6CodePacketTooBig = 0
)

var (
	ErrInvalidIPHeader = errors.New("net/packet/icmp: invalid ip packet header")
	ErrMTUTooSmall     = errors.New("net/packet/icmp: mtu below protocol minimum")
)

// BuildPacketTooBig4 constructs an ICMPv4 Fragmentation Needed packet (RFC 1191 / RFC 4884 / RFC 792 / RFC 9484).
func BuildPacketTooBig4(ipPacket []byte, nextHopMTU uint16) ([]byte, error) {
	if len(ipPacket) < 20 || (ipPacket[0]>>4) != 4 {
		return nil, ErrInvalidIPHeader
	}

	if nextHopMTU < IPv4MinMTU {
		return nil, ErrMTUTooSmall
	}

	ipHdrLen := int(ipPacket[0]&0x0f) * 4
	if ipHdrLen < 20 || len(ipPacket) < ipHdrLen {
		return nil, ErrInvalidIPHeader
	}

	originalLen := min(max(len(ipPacket), 128), 500)
	paddedLen := (originalLen + 3) &^ 3

	totalLen := 20 + 8 + paddedLen
	out := make([]byte, totalLen)

	out[0] = 0x45
	out[1] = 0x00
	binary.BigEndian.PutUint16(out[2:4], uint16(totalLen))
	out[8] = 64
	out[9] = 1

	copy(out[12:16], ipPacket[16:20])
	copy(out[16:20], ipPacket[12:16])

	out[20] = ICMPv4TypeDestinationUnreachable
	out[21] = ICMPv4CodeFragmentationNeeded
	out[22] = 0
	out[23] = 0
	out[24] = 0
	out[25] = byte(paddedLen / 4)
	binary.BigEndian.PutUint16(out[26:28], nextHopMTU)

	copy(out[28:], ipPacket[:min(len(ipPacket), originalLen)])

	binary.BigEndian.PutUint16(out[10:12], packet.CalculateInternetChecksum(out[:20]))
	binary.BigEndian.PutUint16(out[22:24], packet.CalculateInternetChecksum(out[20:]))

	return out, nil
}

// BuildPacketTooBig6 constructs an ICMPv6 Packet Too Big packet (RFC 4443 Section 3.2 / RFC 8200).
func BuildPacketTooBig6(ipPacket []byte, nextHopMTU uint32) ([]byte, error) {
	if len(ipPacket) < 40 || (ipPacket[0]>>4) != 6 {
		return nil, ErrInvalidIPHeader
	}

	if nextHopMTU < IPv6MinMTU {
		return nil, ErrMTUTooSmall
	}

	originalLen := min(len(ipPacket), 1200)
	totalLen := 40 + 8 + originalLen
	out := make([]byte, totalLen)

	out[0] = 0x60
	binary.BigEndian.PutUint16(out[4:6], uint16(8+originalLen))
	out[6] = 58
	out[7] = 64

	copy(out[8:24], ipPacket[24:40])
	copy(out[24:40], ipPacket[8:24])

	out[40] = ICMPv6TypePacketTooBig
	out[41] = ICMPv6CodePacketTooBig
	out[42] = 0
	out[43] = 0
	binary.BigEndian.PutUint32(out[44:48], nextHopMTU)

	copy(out[48:], ipPacket[:originalLen])

	srcAddr, _ := netip.AddrFromSlice(out[8:24])
	dstAddr, _ := netip.AddrFromSlice(out[24:40])

	csum := packet.CalculateICMPv6Checksum(srcAddr, dstAddr, out[40:])
	binary.BigEndian.PutUint16(out[42:44], csum)

	return out, nil
}
