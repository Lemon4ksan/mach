// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headers

import (
	"encoding/binary"
	"math/bits"
	"slices"

	"github.com/lemon4ksan/mach/core/bytesutil"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
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
}

func NewWithCapacity(cap int) Headers {
	return Headers{
		count:   idxReservedCount,
		dynamic: make([]HeaderEntry, 0, cap),
	}
}

func (h *Headers) SetRawBuf(b []byte) {
	h.buf = b
}

func (h *Headers) GetRawBuf() []byte {
	return h.buf
}

func (h *Headers) Reset() {
	h.buf = h.buf[:0]
	h.count = idxReservedCount
	for i := range maxHeaders {
		h.packed[i] = 0
	}
	h.dynamic = h.dynamic[:0]
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
				}
				keyStart = idx + 1
				keyEnd = -1
				valStart = -1
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
			}
			keyStart = i + 1
			keyEnd = -1
			valStart = -1
		}
	}

	// Flush the last header if the block doesn't end with \n
	if keyEnd != -1 {
		valEnd := len(block)
		if valEnd > 0 && block[valEnd-1] == '\r' {
			valEnd--
		}
		h.addPacked(keyStart, keyEnd, valStart, valEnd)
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
	case 12:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Content-Type") {
			h.packed[IdxContentType] = packed
			return
		}
	case 14:
		if bytesconv.EqualFoldASCII(bytesconv.B2S(key), "Content-Length") {
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
	for i := 0; i < h.count; i++ {
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

// VisitAll invokes the f callback for each header without allocating a slice
func (h *Headers) VisitAll(f func(key, value string)) {
	for i := 0; i < h.count; i++ {
		p := h.packed[i]
		if p != 0 {
			f(h.getKey(p), h.getVal(p))
		}
	}
	for i := 0; i < len(h.dynamic); i++ {
		f(h.dynamic[i].Key, h.dynamic[i].Value)
	}
}

var (
	hdrColonSpace = []byte(": ")
	hdrCRLF       = []byte("\r\n")
)

// WriteTo writes headers directly into bufio.Writer without closures (0 allocs)
func (h *Headers) WriteTo(bw *bytesutil.ByteBuffer) {
	for i := 0; i < h.count; i++ {
		p := h.packed[i]
		if p != 0 {
			_, _ = bw.WriteString(h.getKey(p))
			_, _ = bw.Write(hdrColonSpace)
			_, _ = bw.WriteString(h.getVal(p))
			_, _ = bw.Write(hdrCRLF)
		}
	}
	for i := 0; i < len(h.dynamic); i++ {
		_, _ = bw.WriteString(h.dynamic[i].Key)
		_, _ = bw.Write(hdrColonSpace)
		_, _ = bw.WriteString(h.dynamic[i].Value)
		_, _ = bw.Write(hdrCRLF)
	}
}

func (h *Headers) IsKeepAlive(proto string) bool {
	connHeader := h.getVal(h.packed[IdxConnection])
	if connHeader == "" {
		connHeader = h.Get(header.Connection)
	}

	if proto == "HTTP/1.0" {
		return bytesconv.EqualFoldASCII(connHeader, header.ValueKeepAlive)
	}
	// For HTTP/1.1, keep-alive by default
	isKeepAlive := !bytesconv.EqualFoldASCII(connHeader, header.ValueClose)
	return isKeepAlive
}

// Deprecated: used for legacy 1-by-1 string parsing
func (h *Headers) ParseHeaderLine(line []byte) bool {
	// legacy check for colon
	colonFound := slices.Contains(line, ':')
	if !colonFound {
		return false
	}

	// empty key check "   : val"
	firstColon := -1
	for i, b := range line {
		if b == ':' {
			firstColon = i
			break
		}
	}
	keyStr := line[:firstColon]
	isEmpty := true
	for _, b := range keyStr {
		if b != ' ' && b != '\t' {
			isEmpty = false
			break
		}
	}
	if isEmpty {
		return false
	}

	h.buf = append(h.buf, line...)
	if len(line) > 0 && line[len(line)-1] != '\n' {
		h.buf = append(h.buf, '\n') // ensure SWAR triggers
	}
	h.ParseHeaderBlockSWAR(h.buf)
	return true
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
	h.VisitAll(func(k, v string) {
		entries = append(entries, HeaderEntry{Key: k, Value: v})
	})
	return entries
}
