package h1

import (
	"bytes"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	coreh1 "github.com/lemon4ksan/mach/core/h1"
)

func peekArgBytesHeaders(h *coreh1.Headers, key []byte) []byte {
	v := h.Get(bytesconv.B2S(key))
	if v == "" {
		return nil
	}
	return bytesconv.S2B(v)
}

func setArgBytesHeaders(h *coreh1.Headers, key, value []byte, noValue bool) {
	h.Set(string(key), string(value))
}

func appendArgBytesHeaders(h *coreh1.Headers, key, value []byte, noValue bool) {
	h.Add(string(key), string(value))
}

func copyHeaders(dst *coreh1.Headers, src *coreh1.Headers) {
	dst.Reset()
	for _, e := range src.Entries() {
		dst.Add(e.Key, e.Value)
	}
}

func peekAllArgBytesToDstHeaders(dst [][]byte, h *coreh1.Headers, key []byte) [][]byte {
	kStr := bytesconv.B2S(key)
	for _, e := range h.Entries() {
		if bytesconv.EqualFoldASCII(e.Key, kStr) {
			dst = append(dst, bytesconv.S2B(e.Value))
		}
	}
	return dst
}

func parseTrailerHeaders(src []byte, dest *coreh1.Headers, disableNormalizing bool) (int, error) {
	var err error
	n := 0
	for len(src) > 0 {
		idxSemi := bytes.IndexByte(src, '\n')
		if idxSemi < 0 {
			break
		}
		line := src[:idxSemi]
		src = src[idxSemi+1:]
		n += idxSemi + 1

		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) == 0 {
			break
		}
		dest.ParseHeaderLine(line)
	}
	return n, err
}
