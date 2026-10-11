// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priority

import (
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// HeaderPriority is the standard RFC 9218 §4 header name for extensible HTTP priorities.
const HeaderPriority = "priority"

// Standard RFC 9218 §4.1 urgency levels (0 to 7).
const (
	// UrgencyUserBlocking (0) represents critical blocking resources (e.g. HTML documents, synchronous scripts).
	UrgencyUserBlocking = 0

	// UrgencyHigh (1) represents high-priority interactive resources (e.g. API fetch calls, XHR).
	UrgencyHigh = 1

	// UrgencyMedium (2) represents normal sub-resources.
	UrgencyMedium = 2

	// UrgencyLow (3) represents lower priority visual assets (e.g. images, icons).
	UrgencyLow = 3

	// UrgencyVeryLow (4) represents non-blocking assets.
	UrgencyVeryLow = 4

	// UrgencyBackground (5) represents background operations (e.g. prefetch, telemetry, analytics).
	UrgencyBackground = 5

	// UrgencyIdle (6) represents speculative or maintenance tasks.
	UrgencyIdle = 6

	// UrgencyMinimum (7) represents the lowest possible priority.
	UrgencyMinimum = 7
)

// Standard predefined [Priority] configurations.
var (
	// UserBlocking represents urgency 0 non-incremental priority (HTML / critical).
	UserBlocking = Priority{Urgency: UrgencyUserBlocking, Incremental: false}

	// Interactive represents urgency 1 incremental priority (API / fetch).
	Interactive = Priority{Urgency: UrgencyHigh, Incremental: true}

	// Low represents urgency 3 incremental priority (images / media).
	Low = Priority{Urgency: UrgencyLow, Incremental: true}

	// Background represents urgency 5 non-incremental priority (telemetry / prefetch).
	Background = Priority{Urgency: UrgencyBackground, Incremental: false}
)

// Priority encapsulates RFC 9218 §4 priority parameters: urgency (0-7, RFC 9218 §4.1) and incremental streaming flag (RFC 9218 §4.2).
// The zero value represents a default urgency 0 non-incremental priority.
type Priority struct {
	// Urgency indicates resource priority level from 0 (highest) to 7 (lowest) (RFC 9218 §4.1).
	Urgency int

	// Incremental indicates whether the response can be processed incrementally in parallel (RFC 9218 §4.2).
	Incremental bool
}

// New constructs a validated [Priority]. Caps urgency to [0, 7].
func New(urgency int, incremental bool) Priority {
	return Priority{
		Urgency:     min(max(urgency, 0), 7),
		Incremental: incremental,
	}
}

// AppendFormat serializes the [Priority] into an RFC 9218 §4 string and appends it to dst without heap allocations.
func (p Priority) AppendFormat(dst []byte) []byte {
	urg := min(max(p.Urgency, 0), 7)

	dst = append(dst, 'u', '=', byte('0'+urg))
	if p.Incremental {
		dst = append(dst, ',', ' ', 'i')
	}

	return dst
}

// Format serializes the [Priority] into an RFC 9218 §4 Structured Field dictionary string (e.g. "u=1, i").
func (p Priority) Format() string {
	var buf [8]byte
	return bytesconv.B2S(p.AppendFormat(buf[:0]))
}

func (p Priority) String() string {
	return p.Format()
}

// Parse deserializes an RFC 9218 §4 priority header value ([string] or [[]byte]) without heap allocations.
func Parse[S ~string | ~[]byte](s S) (Priority, error) {
	p := Priority{Urgency: 3, Incremental: false}
	raw := s

	start := 0
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
		start++
	}
	end := len(raw)
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\t') {
		end--
	}
	raw = raw[start:end]
	if len(raw) == 0 {
		return p, nil
	}

	for len(raw) > 0 {
		commaIdx := -1
		for i := 0; i < len(raw); i++ {
			if raw[i] == ',' {
				commaIdx = i
				break
			}
		}

		var part S
		if commaIdx >= 0 {
			part = raw[:commaIdx]
			raw = raw[commaIdx+1:]
		} else {
			part = raw
			raw = raw[:0]
		}

		pStart := 0
		for pStart < len(part) && (part[pStart] == ' ' || part[pStart] == '\t') {
			pStart++
		}
		pEnd := len(part)
		for pEnd > pStart && (part[pEnd-1] == ' ' || part[pEnd-1] == '\t') {
			pEnd--
		}
		item := part[pStart:pEnd]

		if equalStr(item, "i") || equalStr(item, "i=?1") {
			p.Incremental = true
			continue
		}
		if equalStr(item, "i=?0") {
			p.Incremental = false
			continue
		}
		if len(item) >= 3 && item[0] == 'u' && item[1] == '=' {
			valStr := item[2:]
			vStart := 0
			for vStart < len(valStr) && (valStr[vStart] == ' ' || valStr[vStart] == '\t') {
				vStart++
			}
			vEnd := len(valStr)
			for vEnd > vStart && (valStr[vEnd-1] == ' ' || valStr[vEnd-1] == '\t') {
				vEnd--
			}
			valTrim := valStr[vStart:vEnd]
			if len(valTrim) == 1 && valTrim[0] >= '0' && valTrim[0] <= '7' {
				p.Urgency = int(valTrim[0] - '0')
			}
		}
	}

	return p, nil
}


func equalStr[S ~string | ~[]byte](s S, target string) bool {
	if len(s) != len(target) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != target[i] {
			return false
		}
	}
	return true
}
