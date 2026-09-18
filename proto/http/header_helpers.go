// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bytes"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
)

func peekArgBytesHeaders(h *coreheaders.Headers, key []byte) []byte {
	v := h.Get(bytesconv.B2S(key))
	if v == "" {
		return nil
	}

	return bytesconv.S2B(v)
}

func setArgBytesHeaders(h *coreheaders.Headers, key, value []byte, noValue bool) {
	h.Set(string(key), string(value))
}

func appendArgBytesHeaders(h *coreheaders.Headers, key, value []byte, noValue bool) {
	h.Add(string(key), string(value))
}

func copyHeaders(dst, src *coreheaders.Headers) {
	dst.Reset()

	for _, e := range src.Entries() {
		dst.Add(e.Key, e.Value)
	}
}

func peekAllArgBytesToDstHeaders(dst [][]byte, h *coreheaders.Headers, key []byte) [][]byte {
	kStr := bytesconv.B2S(key)
	for _, e := range h.Entries() {
		if bytesconv.EqualFoldASCII(e.Key, kStr) {
			dst = append(dst, bytesconv.S2B(e.Value))
		}
	}

	return dst
}

func parseTrailerHeaders(src []byte, dest *coreheaders.Headers, disableNormalizing bool) (int, error) {
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

		idxColon := bytes.IndexByte(line, ':')
		if idxColon > 0 {
			k := line[:idxColon]
			v := bytes.TrimSpace(line[idxColon+1:])
			dest.Add(string(k), string(v))
		}
	}

	return n, err
}
