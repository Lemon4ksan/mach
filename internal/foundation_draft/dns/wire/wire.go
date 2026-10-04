// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wire provides low-level utilities, constants, encoders, and decoders
// for standard RFC 1035, RFC 2181, RFC 3596, RFC 6891, and RFC 9460 DNS wire messages.
package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

var (
	// ErrTruncatedDNSMessage indicates that DNS message is shorter than the required header or payload length.
	ErrTruncatedDNSMessage = errors.New("foundation/net/dns/wire: truncated or malformed dns message")
	// ErrDNSResponseCode indicates that the DNS server returned a non-zero RCODE (e.g. NXDOMAIN or SERVFAIL).
	ErrDNSResponseCode = errors.New("foundation/net/dns/wire: server returned error response code")
	// ErrInvalidDomain indicates an invalid or empty domain name is provided.
	ErrInvalidDomain = errors.New("foundation/net/dns/wire: invalid or empty domain name")
	// ErrInvalidIPVersion indicates an unsupported IP version is provided for EDNS Client Subnet.
	ErrInvalidIPVersion = errors.New("foundation/net/dns/wire: unsupported IP version for EDNS Client Subnet")
	// ErrECHConfigNotFound indicates that the ECH config parameter was not found in the HTTPS record.
	ErrECHConfigNotFound = errors.New("foundation/net/dns/wire: ech config parameter not found in https record")
)

// Standard DNS limits defined in RFC 1035 §2.3.4, RFC 2181 §8, and RFC 8767 §4.
const (
	// MaxLabelLength is the maximum allowed octet length of an individual DNS label (RFC 1035 §2.3.4).
	MaxLabelLength = 63

	// MaxDomainNameLength is the maximum allowed octet length of a fully-qualified domain name (RFC 1035 §2.3.4).
	MaxDomainNameLength = 255

	// MaxUDPMessageSize is the legacy maximum size of unextended UDP DNS messages (RFC 1035 §2.3.4).
	MaxUDPMessageSize = 512

	// MaxRFC2181TTL is the maximum numeric value of a 31-bit TTL (2147483647 seconds / 0x7FFFFFFF) per RFC 2181 §8.
	MaxRFC2181TTL uint32 = 0x7FFFFFFF

	// MaxTTLCap is the maximum recommended authoritative TTL cap of 7 days (RFC 8767 §4).
	MaxTTLCap = 7 * 24 * time.Hour
)

// Standard DNS Resource Record (RR) TYPEs defined in RFC 1035 §3.2.2, RFC 3596 §2.1, RFC 6891, and RFC 9460.
const (
	TypeA     uint16 = 1  // a host address (IPv4) per RFC 1035 §3.4.1
	TypeNS    uint16 = 2  // an authoritative name server per RFC 1035 §3.3.11
	TypeMD    uint16 = 3  // a mail destination (Obsolete - use MX) per RFC 1035 §3.3.4
	TypeMF    uint16 = 4  // a mail forwarder (Obsolete - use MX) per RFC 1035 §3.3.5
	TypeCNAME uint16 = 5  // the canonical name for an alias per RFC 1035 §3.3.1
	TypeSOA   uint16 = 6  // marks the start of a zone of authority per RFC 1035 §3.3.13
	TypeMB    uint16 = 7  // a mailbox domain name (EXPERIMENTAL) per RFC 1035 §3.3.3
	TypeMG    uint16 = 8  // a mail group member (EXPERIMENTAL) per RFC 1035 §3.3.6
	TypeMR    uint16 = 9  // a mail rename domain name (EXPERIMENTAL) per RFC 1035 §3.3.8
	TypeNULL  uint16 = 10 // a null RR (EXPERIMENTAL) per RFC 1035 §3.3.10
	TypeWKS   uint16 = 11 // a well known service description per RFC 1035 §3.4.2
	TypePTR   uint16 = 12 // a domain name pointer per RFC 1035 §3.3.12
	TypeHINFO uint16 = 13 // host information per RFC 1035 §3.3.2
	TypeMINFO uint16 = 14 // mailbox or mail list information per RFC 1035 §3.3.7
	TypeMX    uint16 = 15 // mail exchange per RFC 1035 §3.3.9
	TypeTXT   uint16 = 16 // text strings per RFC 1035 §3.3.14
	TypeAAAA  uint16 = 28 // an IPv6 host address per RFC 3596 §2.1
	TypeOPT   uint16 = 41 // OPT pseudo-record per RFC 6891
	TypeSVCB  uint16 = 64 // Service Binding record per RFC 9460
	TypeHTTPS uint16 = 65 // HTTPS service binding record per RFC 9460
)

// Standard DNS Question Types (QTYPEs) defined in RFC 1035 §3.2.3.
const (
	QTypeAXFR  uint16 = 252 // A request for a transfer of an entire zone (RFC 1035 §3.2.3)
	QTypeMAILB uint16 = 253 // A request for mailbox-related records (MB, MG or MR) (RFC 1035 §3.2.3)
	QTypeMAILA uint16 = 254 // A request for mail agent RRs (Obsolete - see MX) (RFC 1035 §3.2.3)
	QTypeALL   uint16 = 255 // A request for all records (*) (RFC 1035 §3.2.3)
)

// Standard DNS CLASS and QCLASS values defined in RFC 1035 §3.2.4 & §3.2.5.
const (
	ClassIN   uint16 = 1   // the Internet (RFC 1035 §3.2.4)
	ClassCS   uint16 = 2   // the CSNET class (Obsolete) (RFC 1035 §3.2.4)
	ClassCH   uint16 = 3   // the CHAOS class (RFC 1035 §3.2.4)
	ClassHS   uint16 = 4   // Hesiod class (RFC 1035 §3.2.4)
	QClassANY uint16 = 255 // any class (*) (RFC 1035 §3.2.5)
)

// Standard DNS Header Bit Flags defined in RFC 1035 §4.1.1 and RFC 2181 §9.
const (
	FlagQR uint16 = 0x8000 // Query (0) or Response (1)
	FlagAA uint16 = 0x0400 // Authoritative Answer
	FlagTC uint16 = 0x0200 // TrunCation (RFC 1035 §4.1.1 & RFC 2181 §9)
	FlagRD uint16 = 0x0100 // Recursion Desired
	FlagRA uint16 = 0x0080 // Recursion Available
)

// Standard DNS Header Operation Codes (OPCODEs) defined in RFC 1035 §4.1.1.
const (
	OpcodeQuery  uint16 = 0 // a standard query (QUERY)
	OpcodeIQuery uint16 = 1 // an inverse query (IQUERY) (Obsolete per RFC 3425)
	OpcodeStatus uint16 = 2 // a server status request (STATUS)
)

// RCode defines standard DNS Response Codes (RCODEs) defined in RFC 1035 §4.1.1 and RFC 2308.
type RCode uint8

const (
	// RCodeNoError indicates no error condition occurred (RFC 1035 §4.1.1).
	RCodeNoError RCode = 0

	// RCodeFormatError indicates the name server was unable to interpret the query (RFC 1035 §4.1.1).
	RCodeFormatError RCode = 1

	// RCodeServerFailure indicates the name server failed due to internal error (RFC 1035 §4.1.1).
	RCodeServerFailure RCode = 2

	// RCodeNameError (NXDOMAIN) indicates the queried domain name does not exist (RFC 1035 §4.1.1, RFC 2308 §2.1).
	RCodeNameError RCode = 3

	// RCodeNotImplemented indicates the name server does not support the requested kind of query (RFC 1035 §4.1.1).
	RCodeNotImplemented RCode = 4

	// RCodeRefused indicates the name server refuses to perform the specified operation for policy reasons (RFC 1035 §4.1.1).
	RCodeRefused RCode = 5
)

// String returns the canonical RFC text representation of the DNS RCODE.
func (r RCode) String() string {
	switch r {
	case RCodeNoError:
		return "NOERROR"
	case RCodeFormatError:
		return "FORMERR"
	case RCodeServerFailure:
		return "SERVFAIL"
	case RCodeNameError:
		return "NXDOMAIN"
	case RCodeNotImplemented:
		return "NOTIMP"
	case RCodeRefused:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE_%d", uint8(r))
	}
}

// EDNS0 Option Codes defined in RFC 6891, RFC 7871, and RFC 7830.
const (
	EDNS0OptionECS     uint16 = 8  // RFC 7871
	EDNS0OptionPadding uint16 = 12 // RFC 7830
)

// EDNSOptions configures Extension Mechanisms for DNS (EDNS0) features,
// including EDNS Client Subnet (ECS, RFC 7871) and Message Padding (RFC 7830).
type EDNSOptions struct {
	// ClientIP specifies the client IP address to include in the ECS option
	// for localized CDN node resolution. An invalid address disables ECS.
	ClientIP netip.Addr

	// PadToBlock specifies the block size in bytes (e.g., 128) to pad the
	// message to, preventing side-channel size analysis over encrypted transports.
	// A value <= 0 disables padding.
	PadToBlock int
}

// DNSRecord represents an IP address record along with its authoritative TTL.
type DNSRecord struct {
	Addr netip.Addr
	TTL  uint32
}

// NormalizeTTL normalizes a raw 32-bit DNS wire TTL adhering strictly to RFC 2181 §8 and RFC 8767 §4.
//
// Behavior:
// - If the sign bit (MSB / bit 31) is set, RFC 2181 §8 interprets it as 0 unless clamped per RFC 8767.
// - Clamps the resulting duration to the standard 7-day cap (604,800s / MaxTTLCap per RFC 8767 §4).
func NormalizeTTL(rawTTL uint32) time.Duration {
	if (rawTTL & 0x80000000) != 0 {
		rawTTL &= MaxRFC2181TTL
	}

	maxSeconds := uint32(MaxTTLCap / time.Second)
	if rawTTL > maxSeconds {
		rawTTL = maxSeconds
	}

	return time.Duration(rawTTL) * time.Second
}

// SelectRRSetTTL selects the effective TTL for a Resource Record Set (RRSet) per RFC 2181 §5.2.
//
// RFC 2181 §5.2 mandates that if a client receives an RRSet with differing TTLs,
// it MUST treat the RRs for all purposes as if all TTLs in the RRSet were set to the lowest TTL in the RRSet.
func SelectRRSetTTL(ttls ...uint32) time.Duration {
	if len(ttls) == 0 {
		return 0
	}

	minTTL := ttls[0]
	for _, ttl := range ttls[1:] {
		if ttl < minTTL {
			minTTL = ttl
		}
	}

	return NormalizeTTL(minTTL)
}

// PackDNSQuery Encodes a DNS question section into RFC 1035 wire format,
// automatically applying EDNS0 128-byte block padding for encrypted transports.
func PackDNSQuery(id uint16, domain string, qtype uint16) ([]byte, error) {
	return PackDNSQueryExtended(id, domain, qtype, EDNSOptions{PadToBlock: 128})
}

// PackDNSQueryExtended encodes a DNS question into RFC 1035 wire format,
// applying EDNS0 Extension Mechanisms (Padding and Client Subnet) if requested.
//
// Preconditions:
//   - domain must be a valid, non-empty hostname.
//
// Postconditions:
//   - Returns a binary payload with OPT RR in the Additional section if EDNS is enabled.
func PackDNSQueryExtended(id uint16, domain string, qtype uint16, edns EDNSOptions) ([]byte, error) {
	if domain == "" {
		return nil, ErrInvalidDomain
	}

	buf := make([]byte, 0, 256)

	var header [12]byte
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1)

	hasEDNS := edns.PadToBlock > 0 || edns.ClientIP.IsValid()
	if hasEDNS {
		binary.BigEndian.PutUint16(header[10:12], 1)
	}

	buf = append(buf, header[:]...)

	var err error

	buf, err = appendQName(buf, domain)
	if err != nil {
		return nil, err
	}

	var tail [4]byte
	binary.BigEndian.PutUint16(tail[0:2], qtype)
	binary.BigEndian.PutUint16(tail[2:4], ClassIN)
	buf = append(buf, tail[:]...)

	if hasEDNS {
		buf = appendEDNS0OPT(buf, edns)
	}

	return buf, nil
}

// appendQName encodes a standard domain name string into DNS question label format.
func appendQName(buf []byte, domain string) ([]byte, error) {
	rest := domain
	for len(rest) > 0 {
		label := rest
		if idx := strings.IndexByte(rest, '.'); idx >= 0 {
			label = rest[:idx]
			rest = rest[idx+1:]
		} else {
			rest = ""
		}

		if len(label) == 0 {
			continue
		}

		if len(label) > MaxLabelLength {
			return nil, ErrInvalidDomain
		}

		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}

	return append(buf, 0x00), nil
}

// appendEDNS0OPT serializes an OPT pseudo-RR containing EDNS0 options onto buf.
func appendEDNS0OPT(buf []byte, edns EDNSOptions) []byte {
	buf = append(buf, 0x00)
	buf = appendUint16(buf, TypeOPT)
	buf = appendUint16(buf, 4096)
	buf = append(buf, 0x00, 0x00, 0x00, 0x00)

	rdLenOffset := len(buf)
	buf = appendUint16(buf, 0)
	rdataStart := len(buf)

	if edns.ClientIP.IsValid() {
		buf = appendECSOption(buf, edns.ClientIP)
	}

	if edns.PadToBlock > 0 {
		buf = appendPaddingOption(buf, edns.PadToBlock)
	}

	rdataLen := uint16(len(buf) - rdataStart)
	binary.BigEndian.PutUint16(buf[rdLenOffset:rdLenOffset+2], rdataLen)

	return buf
}

// appendECSOption encodes an EDNS Client Subnet (ECS, RFC 7871) option.
func appendECSOption(buf []byte, clientIP netip.Addr) []byte {
	var (
		family     uint16
		sourceMask byte
		ipBytes    []byte
	)

	if clientIP.Is6() {
		family = 2
		sourceMask = 56
		raw := clientIP.As16()
		ipBytes = raw[:7]
	} else {
		family = 1
		sourceMask = 24
		raw := clientIP.As4()
		ipBytes = raw[:3]
	}

	optLen := uint16(2 + 1 + 1 + len(ipBytes))
	buf = appendUint16(buf, EDNS0OptionECS)
	buf = appendUint16(buf, optLen)
	buf = appendUint16(buf, family)
	buf = append(buf, sourceMask, 0x00)

	return append(buf, ipBytes...)
}

// appendPaddingOption adds EDNS0 padding bytes (RFC 7830) to pad query to block length.
func appendPaddingOption(buf []byte, padToBlock int) []byte {
	currentLen := len(buf) + 4
	remainder := currentLen % padToBlock

	paddingNeeded := 0
	if remainder > 0 {
		paddingNeeded = padToBlock - remainder
	}

	buf = appendUint16(buf, EDNS0OptionPadding)

	buf = appendUint16(buf, uint16(paddingNeeded))
	if paddingNeeded > 0 {
		var zeroBuf [128]byte
		for paddingNeeded > 0 {
			chunk := min(paddingNeeded, len(zeroBuf))
			buf = append(buf, zeroBuf[:chunk]...)
			paddingNeeded -= chunk
		}
	}

	return buf
}

// appendUint16 writes a 16-bit unsigned integer in big-endian byte order.
func appendUint16(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}

// ParseDNSResponse extracts IP addresses from a binary RFC 1035 DNS response.
func ParseDNSResponse(msg []byte, expectedID uint16) ([]netip.Addr, error) {
	records, err := ParseDNSResponseRecords(msg, expectedID)
	if err != nil {
		return nil, err
	}

	addrs := make([]netip.Addr, len(records))
	for i, r := range records {
		addrs[i] = r.Addr
	}

	return addrs, nil
}

// ParseDNSResponseRecords parses an RFC 1035 binary DNS response packet
// and extracts IP address records along with their authoritative TTLs.
func ParseDNSResponseRecords(msg []byte, expectedID uint16) ([]DNSRecord, error) {
	if len(msg) < 12 {
		return nil, ErrTruncatedDNSMessage
	}

	id := binary.BigEndian.Uint16(msg[0:2])
	if id != 0 && expectedID != 0 && id != expectedID {
		return nil, ErrDNSResponseCode
	}

	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&0x000f != 0 {
		return nil, fmt.Errorf("%w: rcode=%d", ErrDNSResponseCode, flags&0x000f)
	}

	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))

	offset := 12

	var err error

	for range qdCount {
		offset, err = SkipDomainName(msg, offset)
		if err != nil {
			return nil, err
		}

		if offset+4 > len(msg) {
			return nil, ErrTruncatedDNSMessage
		}

		offset += 4
	}

	records := make([]DNSRecord, 0, anCount)
	for range anCount {
		if offset >= len(msg) {
			break
		}

		var (
			rec      DNSRecord
			consumed int
		)

		rec, consumed, err = parseAnswerRecord(msg, offset)
		if err != nil {
			return nil, err
		}

		offset += consumed

		if rec.Addr.IsValid() {
			records = append(records, rec)
		}
	}

	return records, nil
}

// parseAnswerRecord unpacks an RFC 1035 Resource Record from msg into a DNSRecord.
func parseAnswerRecord(msg []byte, offset int) (DNSRecord, int, error) {
	nextOffset, err := SkipDomainName(msg, offset)
	if err != nil {
		return DNSRecord{}, 0, err
	}

	if nextOffset+10 > len(msg) {
		return DNSRecord{}, 0, ErrTruncatedDNSMessage
	}

	rrType := binary.BigEndian.Uint16(msg[nextOffset : nextOffset+2])
	ttl := binary.BigEndian.Uint32(msg[nextOffset+4 : nextOffset+8])
	rdLength := int(binary.BigEndian.Uint16(msg[nextOffset+8 : nextOffset+10]))
	rdataOffset := nextOffset + 10

	if rdataOffset+rdLength > len(msg) {
		return DNSRecord{}, 0, ErrTruncatedDNSMessage
	}

	totalConsumed := (rdataOffset + rdLength) - offset
	rdata := msg[rdataOffset : rdataOffset+rdLength]

	if rrType == TypeA && rdLength == 4 {
		var ip4 [4]byte
		copy(ip4[:], rdata)
		return DNSRecord{Addr: netip.AddrFrom4(ip4), TTL: ttl}, totalConsumed, nil
	}

	if rrType == TypeAAAA && rdLength == 16 {
		var ip6 [16]byte
		copy(ip6[:], rdata)
		return DNSRecord{Addr: netip.AddrFrom16(ip6), TTL: ttl}, totalConsumed, nil
	}

	return DNSRecord{}, totalConsumed, nil
}

// SkipDomainName skips over a domain name in a DNS message, following DNS compression pointers if necessary.
func SkipDomainName(msg []byte, offset int) (int, error) {
	visited := 0

	for {
		if offset >= len(msg) || visited > 128 {
			return 0, ErrTruncatedDNSMessage
		}

		length := int(msg[offset])
		if (length & 0xc0) == 0xc0 {
			if offset+2 > len(msg) {
				return 0, ErrTruncatedDNSMessage
			}

			return offset + 2, nil
		}

		if length == 0 {
			return offset + 1, nil
		}

		offset += 1 + length
		visited++
	}
}

// ExtractECHFromHTTPSResponse extracts raw ECHConfigList bytes from an RFC 9460 HTTPS (Type 65) DNS response packet.
func ExtractECHFromHTTPSResponse(msg []byte, expectedID uint16) ([]byte, error) {
	if len(msg) < 12 {
		return nil, ErrTruncatedDNSMessage
	}

	id := binary.BigEndian.Uint16(msg[0:2])
	if id != 0 && expectedID != 0 && id != expectedID {
		return nil, ErrDNSResponseCode
	}

	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))

	offset := 12

	var err error

	for range qdCount {
		offset, err = SkipDomainName(msg, offset)
		if err != nil {
			return nil, err
		}

		if offset+4 > len(msg) {
			return nil, ErrTruncatedDNSMessage
		}

		offset += 4
	}

	for range anCount {
		if offset >= len(msg) {
			break
		}

		var (
			ech      []byte
			consumed int
		)

		ech, consumed, err = parseHTTPSRecord(msg, offset)
		if err == nil && len(ech) > 0 {
			return ech, nil
		}

		if consumed == 0 {
			break
		}

		offset += consumed
	}

	return nil, ErrECHConfigNotFound
}

// parseHTTPSRecord parses an RFC 9460 HTTPS RR extracting ECH configuration bytes.
func parseHTTPSRecord(msg []byte, offset int) ([]byte, int, error) {
	nextOffset, err := SkipDomainName(msg, offset)
	if err != nil {
		return nil, 0, err
	}

	if nextOffset+10 > len(msg) {
		return nil, 0, ErrTruncatedDNSMessage
	}

	rrType := binary.BigEndian.Uint16(msg[nextOffset : nextOffset+2])
	rdLength := int(binary.BigEndian.Uint16(msg[nextOffset+8 : nextOffset+10]))
	rdataOffset := nextOffset + 10

	if rdataOffset+rdLength > len(msg) {
		return nil, 0, ErrTruncatedDNSMessage
	}

	totalConsumed := (rdataOffset + rdLength) - offset
	if rrType != TypeHTTPS || rdLength < 4 {
		return nil, totalConsumed, nil
	}

	rdata := msg[rdataOffset : rdataOffset+rdLength]
	svcOffset := 2 // Skip SvcPriority

	svcOffset, err = SkipDomainName(rdata, svcOffset)
	if err != nil {
		return nil, totalConsumed, nil //nolint:nilerr
	}

	for svcOffset+4 <= len(rdata) {
		paramKey := binary.BigEndian.Uint16(rdata[svcOffset : svcOffset+2])
		paramLen := int(binary.BigEndian.Uint16(rdata[svcOffset+2 : svcOffset+4]))
		svcOffset += 4

		if svcOffset+paramLen > len(rdata) {
			break
		}

		// SvcParamKey 5 = "ech"
		if paramKey == 5 {
			echBytes := make([]byte, paramLen)
			copy(echBytes, rdata[svcOffset:svcOffset+paramLen])
			return echBytes, totalConsumed, nil
		}

		svcOffset += paramLen
	}

	return nil, totalConsumed, nil
}
