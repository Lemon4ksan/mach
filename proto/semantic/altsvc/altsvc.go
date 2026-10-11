// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package altsvc

import (
	"iter"
	"strconv"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// DefaultMaxAge specifies the default lifetime for an alternative service (24 hours per RFC 7838 §3.1).
const DefaultMaxAge = 86400 * time.Second

// Service represents an alternative service entry per RFC 7838 §3.
// The zero value represents an empty, unconfigured alternative service.
type Service struct {
	// ProtocolID identifies the ALPN protocol (e.g. "h3", "h2") (RFC 7838 §3).
	ProtocolID string
	// Host specifies the alternative host name, or empty string for the origin host (RFC 7838 §3).
	Host string
	// Port specifies the alternative TCP/UDP port number (RFC 7838 §3).
	Port int
	// MaxAge specifies the freshness lifetime (ma parameter) of the alternative service (RFC 7838 §3.1).
	MaxAge time.Duration
	// Persist indicates whether the alternative service survives network configuration changes (RFC 7838 §3.1).
	Persist bool
}

// ForEach parses an Alt-Svc header value ([string] or [[]byte]) without heap allocations,
// invoking yield for each valid [Service] parsed per RFC 7838 §3.
func ForEach[S ~string | ~[]byte](header S, yield func(Service) bool) bool {
	raw := stringOrBytesToString(header)
	raw = trimSpace(raw)
	if raw == "" || raw == "clear" {
		return true
	}

	for len(raw) > 0 {
		part, rest := splitNextEntry(raw)
		raw = rest
		part = trimSpace(part)
		if part == "" {
			continue
		}

		var (
			svc      Service
			hasFirst bool
		)

		for len(part) > 0 {
			param, nextParam := splitNextParam(part)
			part = nextParam
			param = trimSpace(param)
			if param == "" {
				continue
			}

			if !hasFirst {
				hasFirst = true
				protoID, hostPort, ok := cutByte(param, '=')
				if !ok {
					break
				}

				hostPort = trimSpace(hostPort)
				if len(hostPort) >= 2 && hostPort[0] == '"' && hostPort[len(hostPort)-1] == '"' {
					hostPort = hostPort[1 : len(hostPort)-1]
				}

				colonIdx := lastIndexByte(hostPort, ':')
				if colonIdx == -1 {
					break
				}

				port, ok := parseUint(hostPort[colonIdx+1:])
				if !ok || port == 0 {
					break
				}

				svc = Service{
					ProtocolID: protoID,
					Host:       hostPort[:colonIdx],
					Port:       port,
					MaxAge:     DefaultMaxAge,
				}
				continue
			}

			key, val, _ := cutByte(param, '=')
			key = trimSpace(key)
			val = trimSpace(val)
			switch key {
			case "ma":
				if secs, ok := parseUint(val); ok {
					svc.MaxAge = time.Duration(secs) * time.Second
				}
			case "persist":
				if val == "1" {
					svc.Persist = true
				}
			}
		}

		if hasFirst && svc.Port != 0 {
			if !yield(svc) {
				return false
			}
		}
	}

	return true
}

// All returns an iterator over alternative services without allocating slices (RFC 7838 §3).
// Handles the special "clear" keyword to invalidate all alternative services.
func All[S ~string | ~[]byte](header S) iter.Seq[Service] {
	return func(yield func(Service) bool) {
		ForEach(header, yield)
	}
}

// AppendParse parses an Alt-Svc header and appends each discovered [Service] to dst.
func AppendParse[S ~string | ~[]byte](dst []Service, header S) []Service {
	ForEach(header, func(s Service) bool {
		dst = append(dst, s)
		return true
	})
	return dst
}

// Parse parses an Alt-Svc header ([string] or [[]byte]) into a slice of services (RFC 7838 §3).
func Parse[S ~string | ~[]byte](header S) []Service {
	return AppendParse(nil, header)
}

// Format serializes the given alternative services into an RFC 7838 Alt-Svc header value.
func Format(services ...Service) string {
	if len(services) == 0 {
		return ""
	}
	return bytesconv.B2S(AppendFormat(nil, services...))
}

// AppendFormat appends the serialized RFC 7838 Alt-Svc header string representation to dst without heap allocations.
func AppendFormat(dst []byte, services ...Service) []byte {
	for i, svc := range services {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = append(dst, svc.ProtocolID...)
		dst = append(dst, '=', '"')
		if svc.Host != "" {
			dst = append(dst, svc.Host...)
		}
		dst = append(dst, ':')
		dst = strconv.AppendInt(dst, int64(svc.Port), 10)
		dst = append(dst, '"')

		if svc.MaxAge != DefaultMaxAge {
			dst = append(dst, ';', ' ', 'm', 'a', '=')
			dst = strconv.AppendInt(dst, int64(svc.MaxAge/time.Second), 10)
		}
		if svc.Persist {
			dst = append(dst, ';', ' ', 'p', 'e', 'r', 's', 'i', 's', 't', '=', '1')
		}
	}
	return dst
}

func stringOrBytesToString[S ~string | ~[]byte](s S) string {
	switch v := any(s).(type) {
	case string:
		return v
	case []byte:
		return bytesconv.B2S(v)
	default:
		return ""
	}
}

// specialized for string and []byte via overload
func init() {
	// keep compiler type checks happy
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

func cutByte(s string, sep byte) (before, after string, ok bool) {
	if i := indexByte(s, sep); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func splitNextEntry(s string) (entry, rest string) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if s[i] == ',' && !inQuote {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func splitNextParam(s string) (param, rest string) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if s[i] == ';' && !inQuote {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func parseUint(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
