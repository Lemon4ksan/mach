// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"crypto"
	"crypto/hkdf"
	"encoding/binary"
	"fmt"
)

// HKDFExtract performs HKDF-Extract (RFC 5869).
func HKDFExtract(hash crypto.Hash, secret, salt []byte) []byte {
	prk, err := hkdf.Extract(hash.New, secret, salt)
	if err != nil {
		panic(fmt.Errorf("quic: HKDF-Extract failed: %w", err))
	}
	return prk
}

// HKDFExpandLabel performs HKDF-Expand-Label as defined in RFC 8446 Section 7.1 and RFC 9001 Section 5.1.
//
//	struct {
//	    uint16 length = Length;
//	    opaque label<7..255> = "tls13 " + Label;
//	    opaque context<0..255> = Context;
//	};
func HKDFExpandLabel(hash crypto.Hash, secret, context []byte, label string, length int) []byte {
	fullLabel := "tls13 " + label
	info := make([]byte, 2+1+len(fullLabel)+1+len(context))
	binary.BigEndian.PutUint16(info[0:2], uint16(length))
	info[2] = uint8(len(fullLabel))
	copy(info[3:], fullLabel)
	info[3+len(fullLabel)] = uint8(len(context))
	copy(info[4+len(fullLabel):], context)

	expanded, err := hkdf.Expand(hash.New, secret, string(info), length)
	if err != nil {
		panic(fmt.Errorf("quic: HKDF-Expand-Label failed: %w", err))
	}
	return expanded
}
