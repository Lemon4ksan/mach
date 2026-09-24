// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package svcb implements Service Binding (SVCB) and HTTPS Resource Records strictly conforming to RFC 9460.
//
// It provides wire format decoding and encoding for SVCB (RR type 64) and HTTPS (RR type 65) resource records,
// parameter parsing (alpn, port, ipv4hint, ipv6hint, ech, mandatory), query name construction,
// and endpoint priority evaluation.
package svcb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// Standard DNS Resource Record Types defined in RFC 9460 §14.1 & §14.2.
const (
	// TypeSVCB is the general-purpose Service Binding DNS RR type (64) defined in RFC 9460 §14.1.
	TypeSVCB uint16 = 64
	// TypeHTTPS is the SVCB-compatible DNS RR type specialized for HTTP origins (65) defined in RFC 9460 §14.2.
	TypeHTTPS uint16 = 65
)

// SvcParamKey represents a registered Service Parameter Key defined in RFC 9460 §14.3.2.
type SvcParamKey uint16

// Registered SvcParamKey constants defined in RFC 9460 §14.3.2 (Table 1).
const (
	// ParamMandatory specifies the list of mandatory keys required to process this RR (RFC 9460 §8).
	ParamMandatory SvcParamKey = 0
	// ParamALPN specifies additional supported ALPN protocol identifiers (RFC 9460 §7.1).
	ParamALPN SvcParamKey = 1
	// ParamNoDefaultALPN indicates that the default ALPN protocol (e.g. "http/1.1") is not supported (RFC 9460 §7.1).
	ParamNoDefaultALPN SvcParamKey = 2
	// ParamPort specifies an alternative transport port for the endpoint (RFC 9460 §7.2).
	ParamPort SvcParamKey = 3
	// ParamIPv4Hint conveys IPv4 address hints to reach the service endpoint (RFC 9460 §7.3).
	ParamIPv4Hint SvcParamKey = 4
	// ParamECH conveys Encrypted ClientHello (ECH) configuration bytes (RFC 9460 §14.3.2 & ECH draft).
	ParamECH SvcParamKey = 5
	// ParamIPv6Hint conveys IPv6 address hints to reach the service endpoint (RFC 9460 §7.3).
	ParamIPv6Hint SvcParamKey = 6
	// ParamInvalidKey is the reserved invalid key code point (65535) defined in RFC 9460 §14.3.2.
	ParamInvalidKey SvcParamKey = 65535
)

// String returns the registered presentation name of the SvcParamKey (RFC 9460 §14.3.2).
func (k SvcParamKey) String() string {
	switch k {
	case ParamMandatory:
		return "mandatory"
	case ParamALPN:
		return "alpn"
	case ParamNoDefaultALPN:
		return "no-default-alpn"
	case ParamPort:
		return "port"
	case ParamIPv4Hint:
		return "ipv4hint"
	case ParamECH:
		return "ech"
	case ParamIPv6Hint:
		return "ipv6hint"
	default:
		return fmt.Sprintf("key%d", uint16(k))
	}
}

// ParseParamKey parses a presentation SvcParamKey string (e.g. "alpn", "port", "key1234") into [SvcParamKey].
func ParseParamKey(name string) (SvcParamKey, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mandatory":
		return ParamMandatory, nil
	case "alpn":
		return ParamALPN, nil
	case "no-default-alpn":
		return ParamNoDefaultALPN, nil
	case "port":
		return ParamPort, nil
	case "ipv4hint":
		return ParamIPv4Hint, nil
	case "ech":
		return ParamECH, nil
	case "ipv6hint":
		return ParamIPv6Hint, nil
	default:
		lower := strings.ToLower(name)
		if numStr, ok := strings.CutPrefix(lower, "key"); ok {
			if num, err := strconv.ParseUint(numStr, 10, 16); err == nil {
				return SvcParamKey(num), nil
			}
		}

		return 0, fmt.Errorf("svcb: unknown SvcParamKey %q: %w", name, ErrMalformedParam)
	}
}

// Standard parsing and validation errors for SVCB/HTTPS resource records.
var (
	ErrTruncatedRDATA    = errors.New("svcb: truncated RDATA wire format payload")
	ErrMalformedDomain   = errors.New("svcb: malformed TargetName domain wire format")
	ErrUnsortedParamKeys = errors.New(
		"svcb: SvcParamKeys are not in strictly increasing numeric order (RFC 9460 §2.2)",
	)
	ErrDuplicateParamKey  = errors.New("svcb: duplicate SvcParamKey in RR (RFC 9460 §2.2)")
	ErrMalformedParam     = errors.New("svcb: malformed SvcParam value length or payload")
	ErrIncompatibleRecord = errors.New("svcb: client missing required mandatory SvcParamKeys (RFC 9460 §8)")
)

// Record represents a parsed SVCB (Type 64) or HTTPS (Type 65) Resource Record (RFC 9460 §2).
type Record struct {
	// Priority specifies the SvcPriority. A value of 0 indicates AliasMode; >0 indicates ServiceMode (RFC 9460 §2.4.1).
	Priority uint16
	// TargetName specifies the alias target or service endpoint domain (RFC 9460 §2).
	TargetName string
	// Params maps registered [SvcParamKey] identifiers to their raw wire-format values (RFC 9460 §2.2).
	Params map[SvcParamKey][]byte
}

// IsAlias reports whether the record is operating in AliasMode (SvcPriority == 0, RFC 9460 §2.4.2).
func (r *Record) IsAlias() bool {
	return r != nil && r.Priority == 0
}

// IsService reports whether the record is operating in ServiceMode (SvcPriority > 0, RFC 9460 §2.4.3).
func (r *Record) IsService() bool {
	return r != nil && r.Priority > 0
}

// EffectiveTarget returns the effective target hostname.
// If TargetName is "." in ServiceMode, the owner hostname must be used (RFC 9460 §2.5.2).
func (r *Record) EffectiveTarget(owner string) string {
	if r == nil {
		return owner
	}

	if r.TargetName == "." || r.TargetName == "" {
		return strings.TrimSuffix(owner, ".")
	}

	return strings.TrimSuffix(r.TargetName, ".")
}

// ALPN decodes the comma-separated or length-prefixed ALPN protocol identifiers from ParamALPN (RFC 9460 §7.1).
func (r *Record) ALPN() []string {
	if r == nil || r.Params == nil {
		return nil
	}

	val, ok := r.Params[ParamALPN]
	if !ok || len(val) == 0 {
		return nil
	}

	var alpns []string

	idx := 0
	for idx < len(val) {
		l := int(val[idx])

		idx++
		if idx+l > len(val) {
			break
		}

		alpns = append(alpns, string(val[idx:idx+l]))
		idx += l
	}

	return alpns
}

// HasNoDefaultALPN reports whether the "no-default-alpn" parameter is present (RFC 9460 §7.1).
func (r *Record) HasNoDefaultALPN() bool {
	if r == nil || r.Params == nil {
		return false
	}

	_, ok := r.Params[ParamNoDefaultALPN]

	return ok
}

// Port extracts the alternative endpoint port from ParamPort (RFC 9460 §7.2).
func (r *Record) Port() (uint16, bool) {
	if r == nil || r.Params == nil {
		return 0, false
	}

	val, ok := r.Params[ParamPort]
	if !ok || len(val) != 2 {
		return 0, false
	}

	return binary.BigEndian.Uint16(val), true
}

// IPv4Hints extracts IPv4 address hints from ParamIPv4Hint (RFC 9460 §7.3).
func (r *Record) IPv4Hints() []netip.Addr {
	if r == nil || r.Params == nil {
		return nil
	}

	val, ok := r.Params[ParamIPv4Hint]
	if !ok || len(val) == 0 || len(val)%4 != 0 {
		return nil
	}

	addrs := make([]netip.Addr, 0, len(val)/4)
	for i := 0; i < len(val); i += 4 {
		var b [4]byte
		copy(b[:], val[i:i+4])
		addrs = append(addrs, netip.AddrFrom4(b))
	}

	return addrs
}

// IPv6Hints extracts IPv6 address hints from ParamIPv6Hint (RFC 9460 §7.3).
func (r *Record) IPv6Hints() []netip.Addr {
	if r == nil || r.Params == nil {
		return nil
	}

	val, ok := r.Params[ParamIPv6Hint]
	if !ok || len(val) == 0 || len(val)%16 != 0 {
		return nil
	}

	addrs := make([]netip.Addr, 0, len(val)/16)
	for i := 0; i < len(val); i += 16 {
		var b [16]byte
		copy(b[:], val[i:i+16])
		addrs = append(addrs, netip.AddrFrom16(b))
	}

	return addrs
}

// ECHConfig extracts raw Encrypted ClientHello configuration bytes from ParamECH (RFC 9460 §14.3.2).
func (r *Record) ECHConfig() []byte {
	if r == nil || r.Params == nil {
		return nil
	}

	return r.Params[ParamECH]
}

// MandatoryKeys extracts the list of mandatory SvcParamKeys defined in ParamMandatory (RFC 9460 §8).
func (r *Record) MandatoryKeys() []SvcParamKey {
	if r == nil || r.Params == nil {
		return nil
	}

	val, ok := r.Params[ParamMandatory]
	if !ok || len(val) == 0 || len(val)%2 != 0 {
		return nil
	}

	keys := make([]SvcParamKey, 0, len(val)/2)
	for i := 0; i < len(val); i += 2 {
		k := SvcParamKey(binary.BigEndian.Uint16(val[i : i+2]))
		keys = append(keys, k)
	}

	return keys
}

// IsCompatible verifies that all mandatory keys in the record are supported by the client (RFC 9460 §8).
func (r *Record) IsCompatible(supportedKeys ...SvcParamKey) bool {
	if r == nil || r.IsAlias() {
		return true
	}

	mand := r.MandatoryKeys()
	for _, k := range mand {
		if !slices.Contains(supportedKeys, k) {
			return false
		}
	}

	return true
}

// ParseRDATA parses the RDATA wire format payload into a [*Record] (RFC 9460 §2.2).
func ParseRDATA(rdata []byte) (*Record, error) {
	if len(rdata) < 3 {
		return nil, ErrTruncatedRDATA
	}

	priority := binary.BigEndian.Uint16(rdata[0:2])
	offset := 2

	targetName, newOffset, err := parseDomainName(rdata, offset)
	if err != nil {
		return nil, err
	}

	offset = newOffset
	params := make(map[SvcParamKey][]byte)

	var lastKey uint16

	firstKey := true

	for offset < len(rdata) {
		if offset+4 > len(rdata) {
			return nil, ErrTruncatedRDATA
		}

		key := binary.BigEndian.Uint16(rdata[offset : offset+2])
		valLen := int(binary.BigEndian.Uint16(rdata[offset+2 : offset+4]))
		offset += 4

		if offset+valLen > len(rdata) {
			return nil, ErrTruncatedRDATA
		}

		// RFC 9460 §2.2: SvcParamKeys SHALL appear in strictly increasing numeric order
		if !firstKey && key <= lastKey {
			if key == lastKey {
				return nil, ErrDuplicateParamKey
			}

			return nil, ErrUnsortedParamKeys
		}

		firstKey = false
		lastKey = key

		val := slices.Clone(rdata[offset : offset+valLen])
		params[SvcParamKey(key)] = val
		offset += valLen
	}

	return &Record{
		Priority:   priority,
		TargetName: targetName,
		Params:     params,
	}, nil
}

// MarshalRDATA encodes the record into DNS RDATA wire format bytes (RFC 9460 §2.2).
func (r *Record) MarshalRDATA() ([]byte, error) {
	if r == nil {
		return nil, errors.New("svcb: nil record")
	}

	var (
		buf     bytes.Buffer
		prioBuf [2]byte
	)
	binary.BigEndian.PutUint16(prioBuf[:], r.Priority)
	buf.Write(prioBuf[:])

	domainBytes, err := encodeDomainName(r.TargetName)
	if err != nil {
		return nil, err
	}

	buf.Write(domainBytes)

	// Sort keys in strictly increasing numerical order per RFC 9460 §2.2
	keys := make([]SvcParamKey, 0, len(r.Params))
	for k := range r.Params {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	for _, k := range keys {
		val := r.Params[k]

		var hdr [4]byte
		binary.BigEndian.PutUint16(hdr[0:2], uint16(k))
		binary.BigEndian.PutUint16(hdr[2:4], uint16(len(val)))
		buf.Write(hdr[:])
		buf.Write(val)
	}

	return buf.Bytes(), nil
}

// BuildHTTPSQueryName builds the canonical DNS query name for an HTTPS origin (RFC 9460 §9.1).
// For default port 443, no prefix is used ("example.com.").
// For custom ports (e.g. 8443), port prefix naming is used ("_8443._https.example.com.").
func BuildHTTPSQueryName(origin string, port uint16) string {
	cleanOrigin := strings.Trim(origin, ".")
	if port == 443 || port == 0 {
		return cleanOrigin + "."
	}

	return fmt.Sprintf("_%d._https.%s.", port, cleanOrigin)
}

// BuildSVCBQueryName builds the canonical DNS query name using Attrleaf Port Prefix Naming (RFC 9460 §2.3).
func BuildSVCBQueryName(scheme, service string, port uint16) string {
	cleanService := strings.Trim(service, ".")
	cleanScheme := strings.TrimPrefix(scheme, "_")

	if port == 0 {
		return fmt.Sprintf("_%s.%s.", cleanScheme, cleanService)
	}

	return fmt.Sprintf("_%d._%s.%s.", port, cleanScheme, cleanService)
}

// EncodeALPN encodes a list of ALPN protocol names into the wire-format SvcParamValue (RFC 9460 §7.1.1).
func EncodeALPN(alpns []string) []byte {
	var buf bytes.Buffer
	for _, a := range alpns {
		if len(a) > 255 || len(a) == 0 {
			continue
		}

		buf.WriteByte(byte(len(a)))
		buf.WriteString(a)
	}

	return buf.Bytes()
}

// EncodePort encodes a 16-bit port number into the wire-format SvcParamValue (RFC 9460 §7.2).
func EncodePort(port uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], port)

	return b[:]
}

// EncodeIPv4Hints encodes a slice of IPv4 addresses into the wire-format SvcParamValue (RFC 9460 §7.3).
func EncodeIPv4Hints(ips []net.IP) []byte {
	var buf bytes.Buffer
	for _, ip := range ips {
		if v4 := ip.To4(); len(v4) == 4 {
			buf.Write(v4)
		}
	}

	return buf.Bytes()
}

// EncodeIPv6Hints encodes a slice of IPv6 addresses into the wire-format SvcParamValue (RFC 9460 §7.3).
func EncodeIPv6Hints(ips []net.IP) []byte {
	var buf bytes.Buffer
	for _, ip := range ips {
		if v6 := ip.To16(); len(v6) == 16 && ip.To4() == nil {
			buf.Write(v6)
		}
	}

	return buf.Bytes()
}

func parseDomainName(data []byte, offset int) (string, int, error) {
	if offset >= len(data) {
		return "", offset, ErrTruncatedRDATA
	}

	// Zero length root label "." (RFC 9460 §2.5)
	if data[offset] == 0 {
		return ".", offset + 1, nil
	}

	var labels []string

	curr := offset

	for {
		if curr >= len(data) {
			return "", curr, ErrTruncatedRDATA
		}

		length := int(data[curr])
		curr++

		if length == 0 {
			break
		}

		// DNS wire compression pointers are not allowed in uncompressed TargetName (RFC 9460 §2.2)
		if (length & 0xC0) != 0 {
			return "", curr, ErrMalformedDomain
		}

		if curr+length > len(data) {
			return "", curr, ErrTruncatedRDATA
		}

		labels = append(labels, string(data[curr:curr+length]))
		curr += length
	}

	return strings.Join(labels, "."), curr, nil
}

func encodeDomainName(domain string) ([]byte, error) {
	domain = strings.TrimSpace(domain)
	if domain == "." || domain == "" {
		return []byte{0}, nil
	}

	var buf bytes.Buffer
	for label := range strings.SplitSeq(strings.TrimSuffix(domain, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("svcb: invalid domain label %q: %w", label, ErrMalformedDomain)
		}

		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}

	buf.WriteByte(0)

	return buf.Bytes(), nil
}

// ParseResponseRecords parses all Answer resource records of expectedType (e.g. TypeHTTPS or TypeSVCB)
// from a complete DNS response wire payload (RFC 1035 & RFC 9460).
func ParseResponseRecords(dnsMsg []byte, expectedType uint16) ([]*Record, error) {
	if len(dnsMsg) < 12 {
		return nil, ErrTruncatedRDATA
	}

	qdCount := int(binary.BigEndian.Uint16(dnsMsg[4:6]))
	anCount := int(binary.BigEndian.Uint16(dnsMsg[6:8]))

	offset := 12

	// Skip Question section
	for range qdCount {
		newOffset, err := skipName(dnsMsg, offset)
		if err != nil {
			return nil, err
		}

		if newOffset+4 > len(dnsMsg) {
			return nil, ErrTruncatedRDATA
		}

		offset = newOffset + 4
	}

	var records []*Record

	// Parse Answer section
	for range anCount {
		newOffset, err := skipName(dnsMsg, offset)
		if err != nil {
			return nil, err
		}

		if newOffset+10 > len(dnsMsg) {
			return nil, ErrTruncatedRDATA
		}

		rrType := binary.BigEndian.Uint16(dnsMsg[newOffset : newOffset+2])
		rdLen := int(binary.BigEndian.Uint16(dnsMsg[newOffset+8 : newOffset+10]))
		rdataOffset := newOffset + 10

		if rdataOffset+rdLen > len(dnsMsg) {
			return nil, ErrTruncatedRDATA
		}

		if rrType == expectedType {
			rec, err := ParseRDATA(dnsMsg[rdataOffset : rdataOffset+rdLen])
			if err == nil && rec != nil {
				records = append(records, rec)
			}
		}

		offset = rdataOffset + rdLen
	}

	return records, nil
}

func skipName(data []byte, offset int) (int, error) {
	curr := offset
	for {
		if curr >= len(data) {
			return 0, ErrTruncatedRDATA
		}

		length := int(data[curr])
		if length == 0 {
			return curr + 1, nil
		}

		// Compression pointer (2 bytes per RFC 1035 §4.1.4)
		if (length & 0xC0) == 0xC0 {
			if curr+2 > len(data) {
				return 0, ErrTruncatedRDATA
			}

			return curr + 2, nil
		}

		curr += 1 + length
	}
}
