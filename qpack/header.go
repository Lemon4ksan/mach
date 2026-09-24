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

// All returns a push iterator over header field (name, value) pairs in sequence.
// Supports early termination when the yield function returns false.
// It executes with zero heap allocations.
func (hfs HeaderFields) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for i := range hfs {
			if !yield(hfs[i].Name, hfs[i].Value) {
				return
			}
		}
	}
}

// Values returns a push iterator over HeaderField values in sequence.
// Supports early termination when the yield function returns false.
// It executes with zero heap allocations.
func (hfs HeaderFields) Values() iter.Seq[HeaderField] {
	return func(yield func(HeaderField) bool) {
		for i := range hfs {
			if !yield(hfs[i]) {
				return
			}
		}
	}
}

// Get returns the first value associated with the given header name.
// Header name matching is case-insensitive for non-pseudo headers and exact for pseudo-headers.
// Returns ("", false) if the header is not present.
func (hfs HeaderFields) Get(name string) (string, bool) {
	for i := range hfs {
		if strings.EqualFold(hfs[i].Name, name) {
			return hfs[i].Value, true
		}
	}

	return "", false
}
