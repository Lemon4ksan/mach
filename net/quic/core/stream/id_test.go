// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"testing"
)

func TestStreamID_Classification(t *testing.T) {
	tests := []struct {
		id           StreamID
		expectedType StreamType
		isClient     bool
		isServer     bool
		isBidi       bool
		isUni        bool
		streamNum    uint64
		expectedStr  string
	}{
		{
			id:           0,
			expectedType: StreamTypeClientBidi,
			isClient:     true,
			isServer:     false,
			isBidi:       true,
			isUni:        false,
			streamNum:    1,
			expectedStr:  "stream-0(client-bidi,#1)",
		},
		{
			id:           4,
			expectedType: StreamTypeClientBidi,
			isClient:     true,
			isServer:     false,
			isBidi:       true,
			isUni:        false,
			streamNum:    2,
			expectedStr:  "stream-4(client-bidi,#2)",
		},
		{
			id:           1,
			expectedType: StreamTypeServerBidi,
			isClient:     false,
			isServer:     true,
			isBidi:       true,
			isUni:        false,
			streamNum:    1,
			expectedStr:  "stream-1(server-bidi,#1)",
		},
		{
			id:           5,
			expectedType: StreamTypeServerBidi,
			isClient:     false,
			isServer:     true,
			isBidi:       true,
			isUni:        false,
			streamNum:    2,
			expectedStr:  "stream-5(server-bidi,#2)",
		},
		{
			id:           2,
			expectedType: StreamTypeClientUni,
			isClient:     true,
			isServer:     false,
			isBidi:       false,
			isUni:        true,
			streamNum:    1,
			expectedStr:  "stream-2(client-uni,#1)",
		},
		{
			id:           6,
			expectedType: StreamTypeClientUni,
			isClient:     true,
			isServer:     false,
			isBidi:       false,
			isUni:        true,
			streamNum:    2,
			expectedStr:  "stream-6(client-uni,#2)",
		},
		{
			id:           3,
			expectedType: StreamTypeServerUni,
			isClient:     false,
			isServer:     true,
			isBidi:       false,
			isUni:        true,
			streamNum:    1,
			expectedStr:  "stream-3(server-uni,#1)",
		},
		{
			id:           7,
			expectedType: StreamTypeServerUni,
			isClient:     false,
			isServer:     true,
			isBidi:       false,
			isUni:        true,
			streamNum:    2,
			expectedStr:  "stream-7(server-uni,#2)",
		},
	}

	for _, tc := range tests {
		if tc.id.Type() != tc.expectedType {
			t.Errorf("StreamID(%d).Type() = %v, want %v", tc.id, tc.id.Type(), tc.expectedType)
		}
		if tc.id.IsClientInitiated() != tc.isClient {
			t.Errorf("StreamID(%d).IsClientInitiated() = %v, want %v", tc.id, tc.id.IsClientInitiated(), tc.isClient)
		}
		if tc.id.IsServerInitiated() != tc.isServer {
			t.Errorf("StreamID(%d).IsServerInitiated() = %v, want %v", tc.id, tc.id.IsServerInitiated(), tc.isServer)
		}
		if tc.id.IsBidi() != tc.isBidi {
			t.Errorf("StreamID(%d).IsBidi() = %v, want %v", tc.id, tc.id.IsBidi(), tc.isBidi)
		}
		if tc.id.IsUni() != tc.isUni {
			t.Errorf("StreamID(%d).IsUni() = %v, want %v", tc.id, tc.id.IsUni(), tc.isUni)
		}
		if tc.id.StreamNum() != tc.streamNum {
			t.Errorf("StreamID(%d).StreamNum() = %v, want %v", tc.id, tc.id.StreamNum(), tc.streamNum)
		}
		if tc.id.String() != tc.expectedStr {
			t.Errorf("StreamID(%d).String() = %q, want %q", tc.id, tc.id.String(), tc.expectedStr)
		}

		// Verify round-trip reconstruction
		reconstructed := NewStreamID(tc.expectedType, tc.streamNum)
		if reconstructed != tc.id {
			t.Errorf("NewStreamID(%v, %d) = %d, want %d", tc.expectedType, tc.streamNum, reconstructed, tc.id)
		}
	}
}

func TestStreamType_String(t *testing.T) {
	if StreamType(99).String() != "stream-type(99)" {
		t.Errorf("unexpected string representation: %s", StreamType(99).String())
	}
	if Perspective(0).String() != "client" || Perspective(1).String() != "server" {
		t.Errorf("unexpected perspective string")
	}
}
