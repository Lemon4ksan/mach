// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

// EntrySizeOverhead is the 32-byte overhead added to the lengths of entry name and value (RFC 9204 §3.2.1).
const EntrySizeOverhead uint64 = 32

// Entry represents a key-value header field in the static or dynamic header table (RFC 9204 §3.2).
// The zero value is ready to use.
type Entry struct {
	Name  string
	Value string
}

// NewEntry creates a new [Entry] with the given name and value.
func NewEntry(name, value string) *Entry {
	return &Entry{
		Name:  name,
		Value: value,
	}
}

// Size returns the entry size in octets including [EntrySizeOverhead] (RFC 9204 §3.2.1).
func (e *Entry) Size() uint64 {
	return uint64(len(e.Name)+len(e.Value)) + EntrySizeOverhead
}

// EntrySize calculates the size in octets of an entry with the given name and value (RFC 9204 §3.2.1).
func EntrySize(name, value string) uint64 {
	return uint64(len(name)+len(value)) + EntrySizeOverhead
}
