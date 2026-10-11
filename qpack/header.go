// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qpack implements QPACK: Field Compression for HTTP/3 (RFC 9204).
package qpack

import (
	"iter"
	"strings"
)

// HeaderField represents a single HTTP/3 header or trailer field.
type HeaderField struct {
	Name  string
	Value string
}

// IsPseudo reports whether the header field is an HTTP/3 pseudo-header (e.g. :method, :path).
func (hf HeaderField) IsPseudo() bool {
	return strings.HasPrefix(hf.Name, ":")
}

// HeaderFields represents an ordered slice of HTTP/3 header fields.
type HeaderFields []HeaderField

// All returns an iterator over header field (name, value) pairs in sequence.
func (h HeaderFields) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for i := range h {
			if !yield(h[i].Name, h[i].Value) {
				return
			}
		}
	}
}

// Values returns an iterator over HeaderField values in sequence.
func (h HeaderFields) Values() iter.Seq[HeaderField] {
	return func(yield func(HeaderField) bool) {
		for i := range h {
			if !yield(h[i]) {
				return
			}
		}
	}
}

// Get returns the first value associated with the given header name.
// Header name matching is case-insensitive for non-pseudo headers and exact for pseudo-headers.
func (h HeaderFields) Get(name string) (string, bool) {
	for i := range h {
		if strings.EqualFold(h[i].Name, name) {
			return h[i].Value, true
		}
	}

	return "", false
}
