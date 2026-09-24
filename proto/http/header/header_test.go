// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package header_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/header"
)

func TestHeaderConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"ContentType", header.ContentType, "Content-Type"},
		{"Authorization", header.Authorization, "Authorization"},
		{"Accept", header.Accept, "Accept"},
		{"XForwardedFor", header.XForwardedFor, "X-Forwarded-For"},
		{"SecFetchDest", header.SecFetchDest, "Sec-Fetch-Dest"},
		{"MIMEApplicationJSON", header.MIMEApplicationJSON, "application/json"},
		{"MethodGet", header.MethodGet, "GET"},
		{"ValueKeepAlive", header.ValueKeepAlive, "keep-alive"},
		{"DPoP", header.DPoP, "DPoP"},
		{"GRPCStatus", header.GRPCStatus, "grpc-status"},
		{"Traceparent", header.Traceparent, "traceparent"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("%s: got %q, want %q", tt.name, tt.constant, tt.expected)
		}
	}
}
