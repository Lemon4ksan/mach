// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"math"

	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/mach/proto/headkit"
)

const (
	// indexByte represents the 1-bit MSB pattern (1xxxxxxx) for indexed header field representations (RFC 7541 §6.1).
	indexByte = 128

	// literalByte represents the 2-bit pattern (01xxxxxx) for literal header field with incremental indexing (RFC 7541 §6.2.1).
	literalByte = 64

	// noIndexByte represents the 4-bit pattern (0000xxxx / 0001xxxx) for literal header fields without indexing or never indexed (RFC 7541 §6.2.2 & §6.2.3).
	noIndexByte = 240

	// maxIndex marks the boundary index for dynamic table entries (RFC 7541 §2.3.3 & Appendix A: 61 static entries, dynamic starts at 62).
	maxIndex = 62
)

var (
	// ErrIntegerTruncated indicates that a variable-length integer buffer ended before completion (RFC 7541 §5.1).
	ErrIntegerTruncated = errors.New("hpack: truncated integer (RFC 7541 §5.1)")

	// ErrIntegerOverflow indicates that a variable-length integer representation exceeds implementation limits (RFC 7541 §5.1).
	ErrIntegerOverflow = errors.New("hpack: integer representation exceeds implementation limits (RFC 7541 §5.1)")

	// ErrIndexZero indicates that a decoded index value was 0, which is forbidden (RFC 7541 §6.1).
	ErrIndexZero = errors.New("hpack: index value of 0 is forbidden (RFC 7541 §6.1)")

	// ErrInvalidHuffman indicates malformed Huffman coding, explicit EOS symbol, or invalid padding (RFC 7541 §5.2).
	ErrInvalidHuffman = errors.New("hpack: invalid Huffman code or padding (RFC 7541 §5.2)")

	// ErrTableSizeUpdateNotAllowed indicates that a dynamic table size update occurred elsewhere
	// than at the beginning of a header block (RFC 7541 §4.2).
	ErrTableSizeUpdateNotAllowed = errors.New(
		"hpack: dynamic table size update MUST occur at the beginning of the header block (RFC 7541 §4.2)",
	)

	// ErrTableSizeLimitExceeded indicates that a dynamic table size update exceeds the protocol limit (RFC 7541 §4.2 & §6.3).
	ErrTableSizeLimitExceeded = errors.New(
		"hpack: dynamic table size update exceeds maximum capacity (RFC 7541 §4.2 & §6.3)",
	)

	// ErrIndexOutOfBounds indicates that an index exceeds the combined static and dynamic table address space (RFC 7541 §2.3.3).
	ErrIndexOutOfBounds = errors.New("hpack: index out of bounds (RFC 7541 §2.3.3)")
)

var headerStorage = pool.NewPerCorePool(func() *HeaderField {
	return &HeaderField{}
})

// HeaderField represents an HTTP header key-value pair inside HPACK static and dynamic indexing tables (RFC 7541 §1.3 & §2.3).
// The zero value is ready to use, or instances may be acquired from [AcquireHeaderField].
type HeaderField struct {
	key      []byte
	value    []byte
	keyBuf   []byte
	valBuf   []byte
	sensible bool
	static   bool
}

// AcquireHeaderField retrieves an empty [HeaderField] from the pool.
func AcquireHeaderField() *HeaderField {
	return headerStorage.Get()
}

// ReleaseHeaderField resets hf and returns it to the pool.
func ReleaseHeaderField(hf *HeaderField) {
	if hf != nil && !hf.static {
		hf.Reset()
		headerStorage.Put(hf)
	}
}

// String returns the formatted representation of the header field as "key: value".
func (hf *HeaderField) String() string { return string(hf.AppendBytes(nil)) }

// Empty reports whether both key and value are empty.
func (hf *HeaderField) Empty() bool { return len(hf.key) == 0 && len(hf.value) == 0 }

// Reset clears the key, value, and sensitivity flags of the header field.
func (hf *HeaderField) Reset() {
	if hf.static {
		return
	}

	hf.key = nil
	hf.value = nil
	hf.keyBuf = hf.keyBuf[:0]
	hf.valBuf = hf.valBuf[:0]
	hf.sensible = false
}

// Size returns the entry size in octets as the sum of key length, value length, and 32 octets of overhead (RFC 7541 §4.1).
func (hf *HeaderField) Size() uint32 { return uint32(len(hf.key) + len(hf.value) + 32) } //nolint:gosec

// Key returns the header field name as a string.
func (hf *HeaderField) Key() string { return string(hf.key) }

// Value returns the header field value as a string.
func (hf *HeaderField) Value() string { return string(hf.value) }

// KeyBytes returns the uncopied byte slice of the header field name.
func (hf *HeaderField) KeyBytes() []byte { return hf.key }

// ValueBytes returns the uncopied byte slice of the header field value.
func (hf *HeaderField) ValueBytes() []byte { return hf.value }

// IsPseudo reports whether the header field is an HTTP/2 pseudo-header beginning with a colon ':'.
func (hf *HeaderField) IsPseudo() bool { return len(hf.key) > 0 && hf.key[0] == ':' }

// IsSensible reports whether the header field is marked sensitive and must never be indexed (RFC 7541 §6.2.3).
func (hf *HeaderField) IsSensible() bool { return hf.sensible }

// Set stores the key and value strings into the header field.
func (hf *HeaderField) Set(k, v string) {
	hf.SetKey(k)
	hf.SetValue(v)
}

// SetBytes stores the key and value byte slices into the header field.
func (hf *HeaderField) SetBytes(k, v []byte) {
	hf.SetKeyBytes(k)
	hf.SetValueBytes(v)
}

// SetKey sets the header field name from a string.
func (hf *HeaderField) SetKey(key string) {
	if ik := headkit.InternKey(key); ik != nil {
		hf.key = ik
		return
	}

	hf.keyBuf = append(hf.keyBuf[:0], key...)
	hf.key = hf.keyBuf
}

// SetValue sets the header field value from a string.
func (hf *HeaderField) SetValue(value string) {
	if iv := headkit.InternValue(value); iv != nil {
		hf.value = iv
		return
	}

	hf.valBuf = append(hf.valBuf[:0], value...)
	hf.value = hf.valBuf
}

// SetKeyBytes sets the header field name from a byte slice.
func (hf *HeaderField) SetKeyBytes(key []byte) {
	if ik := headkit.InternKey(key); ik != nil {
		hf.key = ik
		return
	}

	hf.keyBuf = append(hf.keyBuf[:0], key...)
	hf.key = hf.keyBuf
}

// SetValueBytes sets the header field value from a byte slice.
func (hf *HeaderField) SetValueBytes(value []byte) {
	if iv := headkit.InternValue(value); iv != nil {
		hf.value = iv
		return
	}

	hf.valBuf = append(hf.valBuf[:0], value...)
	hf.value = hf.valBuf
}

// CopyTo copies the key, value, and sensitivity flag to other.
func (hf *HeaderField) CopyTo(other *HeaderField) {
	if ik := headkit.InternKey(hf.key); ik != nil {
		other.key = ik
	} else {
		other.keyBuf = append(other.keyBuf[:0], hf.key...)
		other.key = other.keyBuf
	}

	if iv := headkit.InternValue(hf.value); iv != nil {
		other.value = iv
	} else {
		other.valBuf = append(other.valBuf[:0], hf.value...)
		other.value = other.valBuf
	}

	other.sensible = hf.sensible
}

// AppendBytes appends the formatted "key: value" representation to dst and returns the slice.
func (hf *HeaderField) AppendBytes(dst []byte) []byte {
	dst = append(dst, hf.key...)
	dst = append(dst, ':', ' ')
	dst = append(dst, hf.value...)

	return dst
}

var hpackStorage = pool.NewPerCorePool(func() *HPACK {
	return &HPACK{
		maxTableSize:           4096,
		MaxCapacity:            4096,
		tableSizeUpdateAllowed: true,
		dynamic:                make([]*HeaderField, 0, 16),
	}
})

// HPACK manages static and dynamic header compression tables and codec operations (RFC 7541).
// The zero value is ready to use with default settings, or instances may be acquired from [AcquireHPACK].
type HPACK struct {
	DisableCompression     bool
	DisableDynamicTable    bool
	tableSizeUpdateAllowed bool
	pendingTableSizeUpdate bool
	dynamic                []*HeaderField
	maxTableSize           uint32
	dynamicSize            uint32
	MaxCapacity            uint32
}

// AcquireHPACK retrieves an [HPACK] instance from the pool.
func AcquireHPACK() *HPACK {
	hp := hpackStorage.Get()
	hp.Reset()

	return hp
}

// ReleaseHPACK returns hp to the pool after clearing dynamic table entries.
func ReleaseHPACK(hp *HPACK) {
	if hp != nil {
		hpackStorage.Put(hp)
	}
}

// Reset clears dynamic table entries and restores table size defaults.
func (hp *HPACK) Reset() {
	hp.releaseDynamic()
	hp.maxTableSize = 4096
	hp.MaxCapacity = 4096
	hp.dynamicSize = 0
	hp.tableSizeUpdateAllowed = true
	hp.pendingTableSizeUpdate = false
	hp.DisableCompression = false
	hp.DisableDynamicTable = false
}

// BeginHeaderBlock signals the start of a new header block, permitting dynamic table size updates at the beginning (RFC 7541 §4.2).
func (hp *HPACK) BeginHeaderBlock() {
	hp.tableSizeUpdateAllowed = true
}

func (hp *HPACK) releaseDynamic() {
	for _, hf := range hp.dynamic {
		ReleaseHeaderField(hf)
	}

	hp.dynamic = hp.dynamic[:0]
	hp.dynamicSize = 0
}

// SetMaxCapacity sets the upper bound on dynamic table size in octets (RFC 7541 §4.2).
// Dynamic table size updates cannot exceed this capacity.
func (hp *HPACK) SetMaxCapacity(capacity uint32) {
	hp.MaxCapacity = capacity
	if hp.maxTableSize > capacity {
		hp.maxTableSize = capacity
		hp.shrink()
	}
}

// SetMaxTableSize updates the dynamic table size limit and evicts older entries if necessary (RFC 7541 §4.2 & §6.3).
func (hp *HPACK) SetMaxTableSize(size uint32) {
	if hp.maxTableSize != size {
		hp.pendingTableSizeUpdate = true
	}
	hp.maxTableSize = size
	if size > hp.MaxCapacity {
		hp.MaxCapacity = size
	}

	if hp.dynamicSize > size {
		hp.shrink()
	}
}

// DynamicSize returns the current memory size in octets consumed by entries in the dynamic table (RFC 7541 §4.1).
func (hp *HPACK) DynamicSize() uint32 {
	return hp.dynamicSize
}

// DynamicEntries returns the number of entries currently stored in the dynamic table (RFC 7541 §2.3.2).
func (hp *HPACK) DynamicEntries() int {
	return len(hp.dynamic)
}

// addDynamic inserts a new header field into the dynamic table with FIFO ordering and eviction (RFC 7541 §2.3.2 & §4.4).
func (hp *HPACK) addDynamic(hf *HeaderField) {
	if hf.Size() > hp.maxTableSize {
		// RFC 7541 §4.4: An attempt to add an entry larger than the maximum size
		// causes the table to be emptied of all existing entries and results in an empty table.
		hp.releaseDynamic()
		return
	}

	hf2 := AcquireHeaderField()
	hf.CopyTo(hf2)
	hp.dynamic = append(hp.dynamic, hf2)
	hp.dynamicSize += hf2.Size()
	hp.shrink()
}

// shrink evicts oldest entries from the dynamic table until size is within maxTableSize limit (RFC 7541 §4.3 & §4.4).
func (hp *HPACK) shrink() {
	var n int

	for n = 0; n < len(hp.dynamic) && hp.dynamicSize > hp.maxTableSize; n++ {
		entrySize := hp.dynamic[n].Size()
		if hp.dynamicSize > entrySize {
			hp.dynamicSize -= entrySize
		} else {
			hp.dynamicSize = 0
		}
	}

	if n != 0 {
		for i := range n {
			ReleaseHeaderField(hp.dynamic[i])
		}

		hp.dynamic = append(hp.dynamic[:0], hp.dynamic[n:]...)
	}
}

// peek resolves an 1-based index against the unified index address space (RFC 7541 §2.3.3: static 1..61, dynamic 62+).
func (hp *HPACK) peek(n uint64) *HeaderField {
	if n < maxIndex {
		if n == 0 {
			return nil
		}

		idx := int(n - 1)
		if idx >= len(staticTable) {
			return nil
		}

		return staticTable[idx]
	}

	dynOffset := n - maxIndex
	if dynOffset >= uint64(len(hp.dynamic)) {
		return nil
	}

	idx := len(hp.dynamic) - int(dynOffset) - 1

	return hp.dynamic[idx]
}

//go:inline
//go:nosplit
func fastStaticLookup(key, val []byte) (n uint64, fullMatch, found bool) {
	switch string(key) {
	case ":authority":
		return 1, false, true
	case ":method":
		switch string(val) {
		case "GET":
			return 2, true, true
		case "POST":
			return 3, true, true
		default:
			return 2, false, true
		}
	case ":path":
		switch string(val) {
		case "/":
			return 4, true, true
		case "/index.html":
			return 5, true, true
		default:
			return 4, false, true
		}
	case ":scheme":
		switch string(val) {
		case "http":
			return 6, true, true
		case "https":
			return 7, true, true
		default:
			return 7, false, true
		}
	case ":status":
		if string(val) == "200" {
			return 8, true, true
		}
		return 8, false, true
	case "accept-encoding":
		if string(val) == "gzip, deflate" {
			return 16, true, true
		}
		return 16, false, true
	case "accept":
		return 19, false, true
	case "content-type":
		return 31, false, true
	case "cookie":
		return 32, false, true
	case "content-length":
		return 28, false, true
	case "content-encoding":
		return 27, false, true
	case "host":
		return 38, false, true
	case "user-agent":
		return 58, false, true
	default:
		return 0, false, false
	}
}

func (hp *HPACK) search(hf *HeaderField) (n uint64, fullMatch bool) {
	for i, hf2 := range hp.dynamic {
		if bytes.Equal(hf.key, hf2.key) && bytes.Equal(hf.value, hf2.value) {
			return uint64(maxIndex + len(hp.dynamic) - i - 1), true
		}
	}

	if idx, full, found := fastStaticLookup(hf.key, hf.value); found {
		if full {
			return idx, true
		}

		n = idx
	}

	for i, hf2 := range staticTable {
		if !bytes.Equal(hf.key, hf2.key) {
			continue
		}

		if bytes.Equal(hf.value, hf2.value) {
			return uint64(i + 1), true
		}

		if n == 0 {
			n = uint64(i + 1)
		}
	}

	return n, false
}

// Decode returns an iterator yielding decoded [HeaderField] entries and any decoding errors encountered in b (RFC 7541 §3.2).
func (hp *HPACK) Decode(b []byte) iter.Seq2[*HeaderField, error] {
	return func(yield func(*HeaderField, error) bool) {
		// RFC 7541 §4.2: Dynamic table size updates MUST occur at the beginning of the header block.
		hp.BeginHeaderBlock()

		for len(b) > 0 {
			hf := AcquireHeaderField()

			rem, err := hp.Next(hf, b)
			if err != nil {
				ReleaseHeaderField(hf)
				yield(nil, err)
				return
			}

			if len(rem) >= len(b) {
				ReleaseHeaderField(hf)
				yield(nil, errors.New("hpack: decoder failed to make progress"))
				return
			}

			if hf.Empty() {
				ReleaseHeaderField(hf)
			} else if !yield(hf, nil) {
				return
			}

			b = rem
		}
	}
}

// Next decodes the next header field from b into hf (RFC 7541 §3.2 & §6), returning the unconsumed slice.
func (hp *HPACK) Next(hf *HeaderField, b []byte) ([]byte, error) {
	for len(b) > 0 {
		c := b[0]
		switch {
		case c&indexByte == indexByte:
			// RFC 7541 §6.1: Indexed Header Field Representation ('1' 1-bit prefix)
			hp.tableSizeUpdateAllowed = false
			return hp.decodeIndexed(hf, b)
		case c&literalByte == literalByte:
			// RFC 7541 §6.2.1: Literal Header Field with Incremental Indexing ('01' 2-bit prefix)
			hp.tableSizeUpdateAllowed = false
			return hp.decodeLiteralIndexed(hf, b)
		case c&noIndexByte == 16:
			// RFC 7541 §6.2.3: Literal Header Field Never Indexed ('0001' 4-bit prefix)
			hp.tableSizeUpdateAllowed = false
			hf.sensible = true
			return hp.decodeLiteralNoIndex(hf, b)
		case c&noIndexByte == 0:
			// RFC 7541 §6.2.2: Literal Header Field without Indexing ('0000' 4-bit prefix)
			hp.tableSizeUpdateAllowed = false
			return hp.decodeLiteralNoIndex(hf, b)
		case c&0xe0 == 0x20:
			// RFC 7541 §4.2 & §6.3: Dynamic Table Size Update ('001' 3-bit prefix).
			// This dynamic table size update MUST occur at the beginning of the first header block
			// following the change to the dynamic table size. If it occurs elsewhere, it MUST be treated
			// as a decoding error.
			if !hp.tableSizeUpdateAllowed {
				return b, ErrTableSizeUpdateNotAllowed
			}

			var (
				n   uint64
				err error
			)

			b, n, err = readInt(5, b)
			if err != nil {
				return b, err
			}

			if n > uint64(hp.MaxCapacity) {
				return b, ErrTableSizeLimitExceeded
			}

			hp.maxTableSize = uint32(n)
			hp.shrink()

		default:
			return b, errors.New("hpack: unknown header representation (RFC 7541 §6)")
		}
	}

	return b, nil
}

// decodeIndexed decodes an indexed header field representation (RFC 7541 §6.1).
func (hp *HPACK) decodeIndexed(hf *HeaderField, b []byte) ([]byte, error) {
	b, n, err := readInt(7, b)
	if err != nil {
		return b, err
	}

	if n == 0 {
		// RFC 7541 §6.1: Index value of 0 is not used and MUST be treated as a decoding error.
		return b, ErrIndexZero
	}

	hf2 := hp.peek(n)
	if hf2 == nil {
		return b, fmt.Errorf("%w: %d", ErrIndexOutOfBounds, n)
	}

	hf2.CopyTo(hf)

	return b, nil
}

// decodeLiteralIndexed decodes a literal header field with incremental indexing (RFC 7541 §6.2.1).
func (hp *HPACK) decodeLiteralIndexed(hf *HeaderField, b []byte) ([]byte, error) {
	c := b[0]

	var (
		n   uint64
		err error
	)

	var stackBuf [512]byte

	if c != 64 {
		b, n, err = readInt(6, b)
		if err != nil {
			return b, err
		}

		hf2 := hp.peek(n)
		if hf2 == nil {
			return b, fmt.Errorf("literal indexed field not found: %d", n)
		}

		hf.SetKeyBytes(hf2.KeyBytes())
	} else {
		b = b[1:]
		dst := stackBuf[:0]

		b, dst, err = readString(dst, b)
		if err != nil {
			return b, err
		}

		hf.SetKeyBytes(dst)
	}

	dst := stackBuf[:0]

	b, dst, err = readString(dst, b)
	if err != nil {
		return b, err
	}

	hf.SetValueBytes(dst)
	hp.addDynamic(hf)

	return b, nil
}

// decodeLiteralNoIndex decodes a literal header field without indexing or never indexed (RFC 7541 §6.2.2 & §6.2.3).
func (hp *HPACK) decodeLiteralNoIndex(hf *HeaderField, b []byte) ([]byte, error) {
	c := b[0]

	var (
		n   uint64
		err error
	)

	var stackBuf [512]byte

	if c&15 != 0 {
		b, n, err = readInt(4, b)
		if err != nil {
			return b, err
		}

		hf2 := hp.peek(n)
		if hf2 == nil {
			return b, fmt.Errorf("non-indexed field not found: %d", n)
		}

		hf.SetKeyBytes(hf2.key)
	} else {
		b = b[1:]
		dst := stackBuf[:0]

		b, dst, err = readString(dst, b)
		if err != nil {
			return b, err
		}

		hf.SetKeyBytes(dst)
	}

	dst := stackBuf[:0]

	b, dst, err = readString(dst, b)
	if err != nil {
		return b, err
	}

	hf.SetValueBytes(dst)

	return b, nil
}

// AppendHeader encodes hf into dst using HPACK binary representations (RFC 7541 §6).
func (hp *HPACK) AppendHeader(dst []byte, hf *HeaderField, store bool) []byte {
	if hp.pendingTableSizeUpdate {
		hp.pendingTableSizeUpdate = false
		var tmp [8]byte
		b := appendInt(tmp[:0], 5, uint64(hp.maxTableSize))
		b[0] |= 0x20
		dst = append(dst, b...)
	}

	c := !hp.DisableCompression
	index, fullMatch := hp.search(hf)

	if fullMatch {
		// RFC 7541 §6.1: Indexed header field representation
		dst = append(dst, indexByte)
		return appendInt(dst, 7, index)
	}

	var bits uint8

	switch {
	case hf.sensible:
		// RFC 7541 §6.2.3 & §7.1.3: Literal Header Field Never Indexed (protects confidential headers)
		c = false
		bits = 4

		dst = append(dst, 16)

	case !store || hp.DisableDynamicTable:
		// RFC 7541 §6.2.2: Literal Header Field without Indexing
		bits = 4

		dst = append(dst, 0)

	default:
		// RFC 7541 §6.2.1: Literal Header Field with Incremental Indexing (0x40)
		bits = 6

		dst = append(dst, literalByte)

		hp.addDynamic(hf)
	}

	if index > 0 {
		dst = appendInt(dst, bits, index)
	} else {
		dst = appendInt(dst, bits, 0)
		dst = appendString(dst, hf.key, c)
	}

	dst = appendString(dst, hf.value, c)

	return dst
}

var byteStorage = pool.NewPerCorePool(func() *[]byte {
	b := make([]byte, 0, 512)
	return &b
})

// readInt decodes an unsigned variable-length integer with an N-bit prefix (RFC 7541 §5.1).
func readInt(n int, b []byte) ([]byte, uint64, error) {
	if len(b) == 0 {
		return b, 0, ErrIntegerTruncated
	}

	b0 := byte(1<<n - 1)
	if b0&b[0] != b0 {
		return b[1:], uint64(b[0] & b0), nil
	}

	if len(b) == 1 {
		return b, 0, ErrIntegerTruncated
	}

	var (
		nn    uint64
		shift uint
	)

	i := 1

	for i < len(b) {
		c := b[i]
		i++

		if shift > 63 || (shift == 63 && (c&127) > 1) {
			return b[i:], 0, ErrIntegerOverflow
		}

		val := uint64(c & 127)

		added := val << shift
		if nn > math.MaxUint64-added {
			return b[i:], 0, ErrIntegerOverflow
		}

		nn += added

		if c&128 == 0 {
			maxPrefix := uint64(b0)
			if nn > math.MaxUint64-maxPrefix {
				return b[i:], 0, ErrIntegerOverflow
			}

			return b[i:], nn + maxPrefix, nil
		}

		shift += 7
		if shift > 63 {
			return b[i:], 0, ErrIntegerOverflow
		}
	}

	return b, 0, ErrIntegerTruncated
}

// AppendInt encodes an unsigned integer index using an N-bit prefix into dst (RFC 7541 §5.1).
func AppendInt(dst []byte, bits uint8, index uint64) []byte {
	return appendInt(dst, bits, index)
}

// ReadInt decodes an unsigned integer from b using an N-bit prefix (RFC 7541 §5.1), returning the remaining bytes and value.
func ReadInt(n int, b []byte) ([]byte, uint64) {
	rem, val, _ := readInt(n, b)
	return rem, val
}

// appendInt encodes an unsigned variable-length integer index using an N-bit prefix into dst (RFC 7541 §5.1).
func appendInt(dst []byte, bits uint8, index uint64) []byte {
	if len(dst) == 0 {
		dst = append(dst, 0)
	}

	b0 := uint64(1<<bits - 1)

	if index < b0 { //nolint:gosec
		dst[len(dst)-1] |= byte(index)
		return dst
	}

	dst[len(dst)-1] |= byte(b0) //nolint:gosec
	index -= b0

	for index >= 128 {
		dst = append(dst, byte(index&127)|128)
		index >>= 7
	}

	dst = append(dst, byte(index))

	return dst
}

// readString decodes an HPACK string literal representation with optional Huffman decoding (RFC 7541 §5.2).
func readString(dst, b []byte) ([]byte, []byte, error) {
	if len(b) == 0 {
		return b, dst, errors.New("hpack: malformed string (RFC 7541 §5.2)")
	}

	mustDecode := b[0]&128 == 128

	var (
		n   uint64
		err error
	)

	b, n, err = readInt(7, b)
	if err != nil {
		return b, dst, err
	}

	if uint64(len(b)) < n {
		return b, dst, errors.New("hpack: truncated string literal (RFC 7541 §5.2)")
	}

	if mustDecode {
		dst, err = AppendHuffmanDecode(dst, b[:n])
		if err != nil {
			return b, dst, err
		}
	} else {
		dst = append(dst, b[:n]...)
	}

	return b[n:], dst, nil
}

// AppendHuffman encodes s ([string] or [[]byte]) using static HPACK Huffman coding and appends the result to dst (RFC 7541 §5.2 & Appendix B).
func AppendHuffman[S ~string | ~[]byte](dst []byte, s S) []byte {
	return HuffmanEncode(dst, s)
}


// appendString encodes a string literal with optional static Huffman coding and 7-bit length prefix (RFC 7541 §5.2).
func appendString(dst, src []byte, encode bool) []byte {
	var (
		bufPtr   *[]byte
		payload  []byte
		stackBuf [512]byte
	)

	switch {
	case !encode:
		payload = src
	case len(src) <= 256:
		payload = HuffmanEncode(stackBuf[:0], src)
	default:
		bufPtr = byteStorage.Get()
		payload = HuffmanEncode((*bufPtr)[:0], src)
	}

	payloadLen := uint64(len(payload))
	hBitIdx := len(dst)
	dst = append(dst, 0)

	dst = appendInt(dst, 7, payloadLen)
	dst = append(dst, payload...)

	if encode {
		if bufPtr != nil {
			*bufPtr = payload
			byteStorage.Put(bufPtr)
		}

		dst[hBitIdx] |= 0x80
	}

	return dst
}

func init() {
	for _, hf := range staticTable {
		hf.static = true
	}
}

// staticTable defines the 61 predefined HTTP/2 header field entries specified in RFC 7541 Appendix A.
var staticTable = []*HeaderField{
	{key: []byte(":authority")},
	{key: []byte(":method"), value: []byte("GET")},
	{key: []byte(":method"), value: []byte("POST")},
	{key: []byte(":path"), value: []byte("/")},
	{key: []byte(":path"), value: []byte("/index.html")},
	{key: []byte(":scheme"), value: []byte("http")},
	{key: []byte(":scheme"), value: []byte("https")},
	{key: []byte(":status"), value: []byte("200")},
	{key: []byte(":status"), value: []byte("204")},
	{key: []byte(":status"), value: []byte("206")},
	{key: []byte(":status"), value: []byte("304")},
	{key: []byte(":status"), value: []byte("400")},
	{key: []byte(":status"), value: []byte("404")},
	{key: []byte(":status"), value: []byte("500")},
	{key: []byte("accept-charset")},
	{key: []byte("accept-encoding"), value: []byte("gzip, deflate")},
	{key: []byte("accept-language")},
	{key: []byte("accept-ranges")},
	{key: []byte("accept")},
	{key: []byte("access-control-allow-origin")},
	{key: []byte("age")},
	{key: []byte("allow")},
	{key: []byte("authorization")},
	{key: []byte("cache-control")},
	{key: []byte("content-disposition")},
	{key: []byte("content-encoding")},
	{key: []byte("content-language")},
	{key: []byte("content-length")},
	{key: []byte("content-location")},
	{key: []byte("content-range")},
	{key: []byte("content-type")},
	{key: []byte("cookie")},
	{key: []byte("date")},
	{key: []byte("etag")},
	{key: []byte("expect")},
	{key: []byte("expires")},
	{key: []byte("from")},
	{key: []byte("host")},
	{key: []byte("if-match")},
	{key: []byte("if-modified-since")},
	{key: []byte("if-none-match")},
	{key: []byte("if-range")},
	{key: []byte("if-unmodified-since")},
	{key: []byte("last-modified")},
	{key: []byte("link")},
	{key: []byte("location")},
	{key: []byte("max-forwards")},
	{key: []byte("proxy-authenticate")},
	{key: []byte("proxy-authorization")},
	{key: []byte("range")},
	{key: []byte("referer")},
	{key: []byte("refresh")},
	{key: []byte("retry-after")},
	{key: []byte("server")},
	{key: []byte("set-cookie")},
	{key: []byte("strict-transport-security")},
	{key: []byte("transfer-encoding")},
	{key: []byte("user-agent")},
	{key: []byte("vary")},
	{key: []byte("via")},
	{key: []byte("www-authenticate")},
}
