// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"encoding/binary"
	"errors"
	"iter"
	"math/bits"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	// ErrWhitespaceBeforeColon is returned when whitespace occurs between header name and colon (RFC 9112 §5.1).
	ErrWhitespaceBeforeColon = errors.New(
		"headkit: whitespace between header name and colon prohibited (RFC 9112 §5.1)",
	)

	// ErrDuplicateContentLength is returned when duplicate conflicting Content-Length headers are present (RFC 9112 §6.3).
	ErrDuplicateContentLength = errors.New("headkit: duplicate conflicting content-length headers (RFC 9112 §6.3)")

	// ErrMultipleHostHeaders is returned when multiple Host headers are present (RFC 9112 §3.2).
	ErrMultipleHostHeaders = errors.New("headkit: multiple host headers prohibited (RFC 9112 §3.2)")

	// ErrObsFold is returned when obsolete line folding is encountered (RFC 9112 §5.2).
	ErrObsFold = errors.New("headkit: obsolete line folding prohibited (RFC 9112 §5.2)")

	// ErrMalformedHeader is returned when a header line fails RFC 9112 §5 syntax rules.
	ErrMalformedHeader = errors.New("headkit: malformed header line (RFC 9112 §5)")

	// ErrBareCR is returned when a bare CR not immediately followed by LF is detected (RFC 9112 §2.2).
	ErrBareCR = errors.New("headkit: bare CR prohibited (RFC 9112 §2.2)")
)

const (
	IdxHost = iota
	IdxConnection
	IdxContentLength
	IdxContentType
	IdxUserAgent
	IdxAccept

	idxReservedCount = 16
	maxHeaders       = 64
)

// HeaderEntry remains for backwards compatibility and dynamic additions.
type HeaderEntry struct {
	Key   string
	Value string
}

// Headers is a high-performance, O(1) perfect-hashed HTTP header storage.
type Headers struct {
	buf    []byte
	packed [maxHeaders]uint64
	count  int

	dynamic []HeaderEntry // Fallback for dynamically added/modified headers
	err     error
}

func NewWithCapacity(cap int) Headers {
	return Headers{
		count:   idxReservedCount,
		dynamic: make([]HeaderEntry, 0, cap),
	}
}

func (h *Headers) Reset() {
	h.buf = h.buf[:0]
	h.err = nil

	h.count = idxReservedCount
	for i := range maxHeaders {
		h.packed[i] = 0
	}

	h.dynamic = h.dynamic[:0]
}

// Err returns any parsing error encountered during SWAR block processing (RFC 9112 §5.1).
func (h *Headers) Err() error {
	return h.err
}

const (
	lsb = 0x0101010101010101
	msb = 0x8080808080808080
)

func hasZeroByte(v uint64) uint64 {
	return (v - lsb) & ^v & msb
}

func hasByte(v uint64, b byte) uint64 {
	return hasZeroByte(v ^ (uint64(b) * lsb))
}

// ParseHeaderBlockSWAR parses the entire header block in ONE pass using 64-bit SWAR.
func (h *Headers) ParseHeaderBlockSWAR(block []byte) {
	h.buf = block
	h.count = idxReservedCount

	keyStart := 0
	keyEnd := -1
	valStart := -1

	// Check if message begins with obsolete line folding (RFC 9112 §5.2)
	if len(block) > 0 && (block[0] == ' ' || block[0] == '\t') {
		if h.err == nil {
			h.err = ErrObsFold
		}
	}

	i := 0
	for i+8 <= len(block) {
		v := binary.LittleEndian.Uint64(block[i:])

		colonMask := hasByte(v, ':')
		lfMask := hasByte(v, '\n')

		mask := colonMask | lfMask
		if mask == 0 {
			i += 8
			continue
		}

		// Process found delimiters in the current 8-byte chunk
		chunkOffset := 0
		for mask != 0 {
			tz := bits.TrailingZeros64(mask)
			idx := i + (tz / 8)
			c := block[idx]

			if c == ':' && keyEnd == -1 {
				// RFC 9112 §5.1: Field name cannot be empty
				if idx == keyStart {
					if h.err == nil {
						h.err = ErrMalformedHeader
					}
				}

				// RFC 9112 §5.1: No whitespace is allowed between the header field name and colon.
				// A server MUST reject, with a response status code of 400 (Bad Request),
				// any received request message that contains whitespace between a header field name and colon.
				if idx > keyStart && (block[idx-1] == ' ' || block[idx-1] == '\t') {
					if h.err == nil {
						h.err = ErrWhitespaceBeforeColon
					}
				}

				keyEnd = idx
				valStart = idx + 1
				// Trim leading spaces for value
				for valStart < len(block) && (block[valStart] == ' ' || block[valStart] == '\t') {
					valStart++
				}
			} else if c == '\n' {
				valEnd := idx
				if valEnd > 0 && block[valEnd-1] == '\r' {
					valEnd--
				}

				if keyEnd != -1 {
					h.addPacked(keyStart, keyEnd, valStart, valEnd)
				} else if keyStart < valEnd {
					// RFC 9112 §5: Non-empty line without a colon
					if h.err == nil {
						h.err = ErrMalformedHeader
					}
				}

				// RFC 9112 §2.2: Bare CR check and null byte check
				for j := keyStart; j < valEnd; j++ {
					b := block[j]
					switch b {
					case 0:
						if h.err == nil {
							h.err = ErrMalformedHeader
						}
					case '\r':
						if h.err == nil {
							h.err = ErrBareCR
						}
					}
				}

				keyStart = idx + 1
				keyEnd = -1
				valStart = -1

				// RFC 9112 §5.2: Obsolete line folding: line cannot start with SP or HTAB
				if keyStart < len(block) && (block[keyStart] == ' ' || block[keyStart] == '\t') {
					if h.err == nil {
						h.err = ErrObsFold
					}
				}
			}

			// Clear the bit and continue
			clearMask := uint64(0xFF) << tz
			mask &^= clearMask
			colonMask &^= clearMask
			lfMask &^= clearMask
			chunkOffset = (tz / 8) + 1
		}

		// Move to the last found delimiter + 1, or next 8 byte chunk
		if chunkOffset > 0 {
			i += chunkOffset
		} else {
			i += 8
		}
	}

	// Tail processing
	for ; i < len(block); i++ {
		c := block[i]
		if c == ':' && keyEnd == -1 {
			if i == keyStart {
				if h.err == nil {
					h.err = ErrMalformedHeader
				}
			}

			// RFC 9112 §5.1: No whitespace is allowed between the header field name and colon.
			if i > keyStart && (block[i-1] == ' ' || block[i-1] == '\t') {
				if h.err == nil {
					h.err = ErrWhitespaceBeforeColon
				}
			}

			keyEnd = i

			valStart = i + 1
			for valStart < len(block) && (block[valStart] == ' ' || block[valStart] == '\t') {
				valStart++
			}
		} else if c == '\n' {
			valEnd := i
			if valEnd > 0 && block[valEnd-1] == '\r' {
				valEnd--
			}

			if keyEnd != -1 {
				h.addPacked(keyStart, keyEnd, valStart, valEnd)
			} else if keyStart < valEnd {
				if h.err == nil {
					h.err = ErrMalformedHeader
				}
			}

			for j := keyStart; j < valEnd; j++ {
				b := block[j]
				switch b {
				case 0:
					if h.err == nil {
						h.err = ErrMalformedHeader
					}
				case '\r':
					if h.err == nil {
						h.err = ErrBareCR
					}
				}
			}

			keyStart = i + 1
			keyEnd = -1
			valStart = -1

			if keyStart < len(block) && (block[keyStart] == ' ' || block[keyStart] == '\t') {
				if h.err == nil {
					h.err = ErrObsFold
				}
			}
		}
	}

	// Flush the last header if the block doesn't end with \n
	if keyEnd != -1 {
		valEnd := len(block)
		if valEnd > 0 && block[valEnd-1] == '\r' {
			valEnd--
		}

		h.addPacked(keyStart, keyEnd, valStart, valEnd)

		for j := keyStart; j < valEnd; j++ {
			b := block[j]
			switch b {
			case 0:
				if h.err == nil {
					h.err = ErrMalformedHeader
				}
			case '\r':
				if h.err == nil {
					h.err = ErrBareCR
				}
			}
		}
	} else if keyStart < len(block) {
		valEnd := len(block)
		if valEnd > 0 && block[valEnd-1] == '\r' {
			valEnd--
		}

		if keyStart < valEnd {
			if h.err == nil {
				h.err = ErrMalformedHeader
			}
		}
	}
}

func (h *Headers) addPacked(keyStart, keyEnd, valStart, valEnd int) {
	// Trim spaces from key
	for keyStart < keyEnd && (h.buf[keyStart] == ' ' || h.buf[keyStart] == '\t') {
		keyStart++
	}

	for keyEnd > keyStart && (h.buf[keyEnd-1] == ' ' || h.buf[keyEnd-1] == '\t') {
		keyEnd--
	}

	// Trim spaces from value
	for valStart < valEnd && (h.buf[valStart] == ' ' || h.buf[valStart] == '\t') {
		valStart++
	}

	for valEnd > valStart && (h.buf[valEnd-1] == ' ' || h.buf[valEnd-1] == '\t') {
		valEnd--
	}

	if keyEnd <= keyStart || valEnd < valStart {
		if h.err == nil && keyEnd <= keyStart {
			h.err = ErrMalformedHeader
		}

		return
	}

	keyLen := keyEnd - keyStart
	valLen := valEnd - valStart

	packed := (uint64(keyStart) << 48) | (uint64(keyLen) << 32) | (uint64(valStart) << 16) | uint64(valLen)

	// Perfect Hashing logic
	key := h.buf[keyStart:keyEnd]
	switch len(key) {
	case 4:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Host") {
			if h.packed[IdxHost] != 0 {
				if h.err == nil {
					h.err = ErrMultipleHostHeaders
				}

				return
			}

			h.packed[IdxHost] = packed

			return
		}

	case 6:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Accept") {
			h.packed[IdxAccept] = packed
			return
		}
	case 10:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Connection") {
			h.packed[IdxConnection] = packed
			return
		}

		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "User-Agent") {
			h.packed[IdxUserAgent] = packed
			return
		}

	case 12:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Content-Type") {
			h.packed[IdxContentType] = packed
			return
		}
	case 14:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Content-Length") {
			if h.packed[IdxContentLength] != 0 {
				oldVal := h.getVal(h.packed[IdxContentLength])

				newVal := bytesconv.B2S(h.buf[valStart:valEnd])
				if oldVal != newVal {
					if h.err == nil {
						h.err = ErrDuplicateContentLength
					}

					return
				}

				return
			}

			h.packed[IdxContentLength] = packed

			return
		}
	}

	if h.count < maxHeaders {
		h.packed[h.count] = packed
		h.count++
	} else {
		h.dynamic = append(h.dynamic, HeaderEntry{
			Key:   string(key),
			Value: string(h.buf[valStart:valEnd]),
		})
	}
}

func (h *Headers) Get(key string) string {
	// 1. O(1) Perfect Hashing Fast Path
	switch len(key) {
	case 4:
		if bytesconv.EqualFoldASCII(key, "Host") {
			if v := h.getVal(h.packed[IdxHost]); v != "" {
				return v
			}
		}

	case 6:
		if bytesconv.EqualFoldASCII(key, "Accept") {
			if v := h.getVal(h.packed[IdxAccept]); v != "" {
				return v
			}
		}

	case 10:
		if bytesconv.EqualFoldASCII(key, "Connection") {
			if v := h.getVal(h.packed[IdxConnection]); v != "" {
				return v
			}
		}

		if bytesconv.EqualFoldASCII(key, "User-Agent") {
			if v := h.getVal(h.packed[IdxUserAgent]); v != "" {
				return v
			}
		}

	case 12:
		if bytesconv.EqualFoldASCII(key, "Content-Type") {
			if v := h.getVal(h.packed[IdxContentType]); v != "" {
				return v
			}
		}

	case 14:
		if bytesconv.EqualFoldASCII(key, "Content-Length") {
			if v := h.getVal(h.packed[IdxContentLength]); v != "" {
				return v
			}
		}
	}

	// 2. Packed array scan
	for i := idxReservedCount; i < h.count; i++ {
		p := h.packed[i]
		if p == 0 {
			continue
		}

		k := h.getKey(p)
		if bytesconv.EqualFoldASCII(k, key) {
			return h.getVal(p)
		}
	}

	// 3. Dynamic array scan
	for i := range h.dynamic {
		if bytesconv.EqualFoldASCII(h.dynamic[i].Key, key) {
			return h.dynamic[i].Value
		}
	}

	return ""
}

func (h *Headers) getKey(p uint64) string {
	if p == 0 {
		return ""
	}

	start := (p >> 48) & 0xFFFF
	length := (p >> 32) & 0xFFFF

	return bytesconv.B2S(h.buf[start : start+length])
}

func (h *Headers) getVal(p uint64) string {
	if p == 0 {
		return ""
	}

	start := (p >> 16) & 0xFFFF
	length := p & 0xFFFF

	return bytesconv.B2S(h.buf[start : start+length])
}

func (h *Headers) Set(key, val string) {
	h.Del(key)
	h.dynamic = append(h.dynamic, HeaderEntry{Key: key, Value: val})
}

func (h *Headers) Add(key, val string) {
	h.dynamic = append(h.dynamic, HeaderEntry{Key: key, Value: val})
}

func (h *Headers) Has(key string) bool {
	return h.Get(key) != ""
}

func (h *Headers) Del(key string) {
	// Clear from packed
	for i := range h.count {
		p := h.packed[i]
		if p == 0 {
			continue
		}

		if bytesconv.EqualFoldASCII(h.getKey(p), key) {
			h.packed[i] = 0
		}
	}

	// Clear from dynamic
	n := 0
	for _, entry := range h.dynamic {
		if !bytesconv.EqualFoldASCII(entry.Key, key) {
			h.dynamic[n] = entry
			n++
		}
	}

	h.dynamic = h.dynamic[:n]
}

// All returns an iterator over all headers.
func (h *Headers) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for i := range h.count {
			p := h.packed[i]
			if p != 0 {
				if !yield(h.getKey(p), h.getVal(p)) {
					return
				}
			}
		}

		for i := range h.dynamic {
			if !yield(h.dynamic[i].Key, h.dynamic[i].Value) {
				return
			}
		}
	}
}

var (
	hdrColonSpace = []byte(": ")
	hdrCRLF       = []byte("\r\n")
)

// WriteTo writes headers directly into bufio.Writer without closures (0 allocs)
func (h *Headers) WriteTo(bw *bytesconv.ByteBuffer) {
	for i := range h.count {
		p := h.packed[i]
		if p != 0 {
			_, _ = bw.WriteString(h.getKey(p))
			_, _ = bw.Write(hdrColonSpace)
			_, _ = bw.WriteString(h.getVal(p))
			_, _ = bw.Write(hdrCRLF)
		}
	}

	for i := range h.dynamic {
		_, _ = bw.WriteString(h.dynamic[i].Key)
		_, _ = bw.Write(hdrColonSpace)
		_, _ = bw.WriteString(h.dynamic[i].Value)
		_, _ = bw.Write(hdrCRLF)
	}
}

func (h *Headers) IsKeepAlive(proto string) bool {
	connHeader := h.getVal(h.packed[IdxConnection])
	if connHeader == "" {
		connHeader = h.Get(Connection)
	}

	if proto == "HTTP/1.0" {
		return bytesconv.EqualFoldASCII(connHeader, ValueKeepAlive)
	}

	// For HTTP/1.1, keep-alive by default
	isKeepAlive := !bytesconv.EqualFoldASCII(connHeader, ValueClose)

	return isKeepAlive
}

func (h *Headers) AddFromHTTP(src map[string][]string) {
	for k, vv := range src {
		for _, v := range vv {
			h.Add(k, v)
		}
	}
}

// Entries returns all headers as a slice. Allocates!
func (h *Headers) Entries() []HeaderEntry {
	entries := make([]HeaderEntry, 0, h.count+len(h.dynamic))
	for k, v := range h.All() {
		entries = append(entries, HeaderEntry{Key: k, Value: v})
	}

	return entries
}
