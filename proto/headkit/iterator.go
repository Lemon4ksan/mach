// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"iter"
	"net/http"
)

// Flatten yields an iterator traversing every individual key-value pair in h, expanding multi-value slices.
func Flatten(h http.Header) iter.Seq2[string, string] {
	return func(yield func(key, value string) bool) {
		for k, vv := range h {
			for _, v := range vv {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

// SplitCommaValues yields an iterator traversing all comma-separated tokens across all header entries matching key.
// Trims whitespace and skips empty entries with zero heap slice allocations.
func SplitCommaValues(h http.Header, key string) iter.Seq[string] {
	return func(yield func(token string) bool) {
		if h == nil {
			return
		}

		for _, entry := range h.Values(key) {
			for k := range Directives(entry) {
				if k != "" {
					if !yield(k) {
						return
					}
				}
			}
		}
	}
}
