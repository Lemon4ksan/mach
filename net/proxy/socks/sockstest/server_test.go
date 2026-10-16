// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sockstest

import (
	"bytes"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

func TestParseAuthRequest(t *testing.T) {
	for i, tt := range []struct {
		wire []byte
		req  *AuthRequest
	}{
		{
			[]byte{0x05, 0x00},
			&AuthRequest{
				socks.Version5,
				nil,
			},
		},
		{
			[]byte{0x05, 0x01, 0xff},
			&AuthRequest{
				socks.Version5,
				[]socks.AuthMethod{
					socks.AuthMethodNoAcceptableMethods,
				},
			},
		},
		{
			[]byte{0x05, 0x02, 0x00, 0xff},
			&AuthRequest{
				socks.Version5,
				[]socks.AuthMethod{
					socks.AuthMethodNotRequired,
					socks.AuthMethodNoAcceptableMethods,
				},
			},
		},

		// corrupted requests
		{nil, nil},
		{[]byte{0x00, 0x01}, nil},
		{[]byte{0x06, 0x00}, nil},
		{[]byte{0x05, 0x02, 0x00}, nil},
	} {
		req, err := ParseAuthRequest(tt.wire)
		if !reflect.DeepEqual(req, tt.req) {
			t.Errorf("#%d: got %v, %v; want %v", i, req, err, tt.req)
			continue
		}
	}
}

func TestParseCmdRequest(t *testing.T) {
	for i, tt := range []struct {
		wire []byte
		req  *CmdRequest
	}{
		{
			[]byte{0x05, 0x01, 0x00, 0x01, 192, 0, 2, 1, 0x17, 0x4b},
			&CmdRequest{
				socks.Version5,
				socks.CmdConnect,
				socks.Addr{
					IP:   net.IP{192, 0, 2, 1},
					Port: 5963,
				},
			},
		},
		{
			[]byte{0x05, 0x01, 0x00, 0x03, 0x04, 'F', 'Q', 'D', 'N', 0x17, 0x4b},
			&CmdRequest{
				socks.Version5,
				socks.CmdConnect,
				socks.Addr{
					Name: "FQDN",
					Port: 5963,
				},
			},
		},
		{
			[]byte{
				0x05, 0x01, 0x00, 0x04,
				0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
				0x17, 0x4b,
			},
			&CmdRequest{
				socks.Version5,
				socks.CmdConnect,
				socks.Addr{
					IP:   net.ParseIP("2001:db8::1"),
					Port: 5963,
				},
			},
		},

		// corrupted requests
		{nil, nil},
		{[]byte{0x05}, nil},
		{[]byte{0x06, 0x01, 0x00, 0x01, 192, 0, 2, 2, 0x17, 0x4b}, nil},
		{[]byte{0x05, 0x02, 0x00, 0x01, 192, 0, 2, 2, 0x17, 0x4b}, nil},
		{[]byte{0x05, 0x01, 0xff, 0x01, 192, 0, 2, 3}, nil},
		{[]byte{0x05, 0x01, 0x00, 0x01, 192, 0, 2, 4}, nil},
		{[]byte{0x05, 0x01, 0x00, 0x03, 0x04, 'F', 'Q', 'D', 'N'}, nil},
		{[]byte{0x05, 0x01, 0x00, 0x05, 192, 0, 2, 1, 0x17, 0x4b}, nil}, // unknown addr type
	} {
		req, err := ParseCmdRequest(tt.wire)
		if !reflect.DeepEqual(req, tt.req) {
			t.Errorf("#%d: got %v, %v; want %v", i, req, err, tt.req)
			continue
		}
	}
}

func TestMarshalAuthAndCmdReply(t *testing.T) {
	// 1. MarshalAuthReply
	authRep, err := MarshalAuthReply(socks.Version5, socks.AuthMethodNotRequired)
	if err != nil || !bytes.Equal(authRep, []byte{0x05, 0x00}) {
		t.Fatalf("MarshalAuthReply failed: %v", authRep)
	}

	// 2. MarshalCmdReply IPv4
	ip4Addr := &socks.Addr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
	cmdRep4, err := MarshalCmdReply(socks.Version5, socks.StatusSucceeded, ip4Addr)
	if err != nil || len(cmdRep4) != 10 {
		t.Fatalf("MarshalCmdReply IPv4 failed: %v", err)
	}

	// 3. MarshalCmdReply IPv6
	ip6Addr := &socks.Addr{IP: net.ParseIP("2001:db8::1"), Port: 8080}
	cmdRep6, err := MarshalCmdReply(socks.Version5, socks.StatusSucceeded, ip6Addr)
	if err != nil || len(cmdRep6) != 22 {
		t.Fatalf("MarshalCmdReply IPv6 failed: %v", err)
	}

	// 4. MarshalCmdReply FQDN
	fqdnAddr := &socks.Addr{Name: "example.com", Port: 80}
	cmdRepFQDN, err := MarshalCmdReply(socks.Version5, socks.StatusSucceeded, fqdnAddr)
	if err != nil || len(cmdRepFQDN) != 4+1+len("example.com")+2 {
		t.Fatalf("MarshalCmdReply FQDN failed: %v", err)
	}

	// 5. FQDN too long error
	longAddr := &socks.Addr{Name: strings.Repeat("a", 256), Port: 80}
	_, err = MarshalCmdReply(socks.Version5, socks.StatusSucceeded, longAddr)
	if err == nil {
		t.Fatalf("expected error for FQDN too long")
	}

	// 6. Unknown address type
	badAddr := &socks.Addr{Port: 80}
	_, err = MarshalCmdReply(socks.Version5, socks.StatusSucceeded, badAddr)
	if err == nil {
		t.Fatalf("expected error for empty address")
	}
}

func TestServer_Lifecycle(t *testing.T) {
	ss, err := NewServer(NoAuthRequired, NoProxyRequired)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer ss.Close()

	if ss.Addr() == nil {
		t.Fatalf("ss.Addr() returned nil")
	}
	if ss.TargetAddr() == nil {
		t.Fatalf("ss.TargetAddr() returned nil")
	}
}
