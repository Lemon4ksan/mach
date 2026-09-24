// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

// EntrySizeOverhead is the constant 32-byte overhead added to the length
// of an entry's name and value to compute its size, as defined in RFC 9204 §3.2.1.
const EntrySizeOverhead uint64 = 32

// Entry represents an entry in the static or dynamic header table.
type Entry struct {
	Name  string
	Value string
}

// NewEntry constructs a new Entry.
func NewEntry(name, value string) *Entry {
	return &Entry{
		Name:  name,
		Value: value,
	}
}

// Size returns the size of the entry including 32 bytes of overhead (RFC 9204 §3.2.1).
func (e *Entry) Size() uint64 {
	return uint64(len(e.Name)+len(e.Value)) + EntrySizeOverhead
}

// EntrySize computes the size of an entry given its name and value strings.
func EntrySize(name, value string) uint64 {
	return uint64(len(name)+len(value)) + EntrySizeOverhead
}
