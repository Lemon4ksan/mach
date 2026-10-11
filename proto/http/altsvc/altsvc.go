// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package altsvc provides backward-compatible forwarders to RFC 7838 Alt-Svc utilities.
package altsvc

import (
	"iter"

	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

// DefaultMaxAge forwarded from proto/semantic/altsvc.
const DefaultMaxAge = altsvc.DefaultMaxAge

// Service forwarded from proto/semantic/altsvc.
type Service = altsvc.Service

// All returns an iterator over alternative services per RFC 7838 §3.
func All(header string) iter.Seq[Service] {
	return altsvc.All(header)
}

// AllBytes returns an iterator over alternative services from raw bytes.
func AllBytes(header []byte) iter.Seq[Service] {
	return altsvc.AllBytes(header)
}

// ForEach parses header without allocations, invoking yield for each Service.
func ForEach[S ~string | ~[]byte](header S, yield func(Service) bool) bool {
	return altsvc.ForEach(header, yield)
}

// AppendParse parses header and appends found services to dst.
func AppendParse[S ~string | ~[]byte](dst []Service, header S) []Service {
	return altsvc.AppendParse(dst, header)
}

// Parse parses an Alt-Svc header value into a slice of services (RFC 7838 §3).
func Parse(header string) []Service {
	return altsvc.Parse(header)
}

// ParseBytes parses an Alt-Svc header byte slice into a slice of services.
func ParseBytes(header []byte) []Service {
	return altsvc.ParseBytes(header)
}

// Format serializes the given alternative services into an RFC 7838 Alt-Svc header value.
func Format(services ...Service) string {
	return altsvc.Format(services...)
}

// AppendFormat appends the serialized RFC 7838 Alt-Svc header string representation to dst.
func AppendFormat(dst []byte, services ...Service) []byte {
	return altsvc.AppendFormat(dst, services...)
}
