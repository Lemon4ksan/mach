// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socks_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

// pipeDialer creates a net.Pipe in-memory pair to avoid OS socket overhead.
func pipeDialer(serverHandler func(net.Conn)) *socks.Dialer {
	d := socks.NewDialer("tcp", "127.0.0.1:1080")
	d.ProxyDial = func(ctx context.Context, network, address string) (net.Conn, error) {
		cli, srv := net.Pipe()
		go func() {
			serverHandler(srv)
		}()
		return cli, nil
	}
	return d
}

// TestSOCKS5_TruncatedAuthSelection tests server drops/truncation during initial auth negotiation.
func TestSOCKS5_TruncatedAuthSelection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		response   []byte
		closeEarly bool
	}{
		{
			name:       "DropImmediately_0Bytes",
			response:   nil,
			closeEarly: true,
		},
		{
			name:       "Truncated_1Byte_VersionOnly",
			response:   []byte{socks.Version5},
			closeEarly: true,
		},
		{
			name:       "InvalidVersion_SOCKS4",
			response:   []byte{0x04, 0x00},
			closeEarly: false,
		},
		{
			name:       "NoAcceptableMethods_0xFF",
			response:   []byte{socks.Version5, 0xff},
			closeEarly: false,
		},
		{
			name:       "UnsupportedMethod_0x99",
			response:   []byte{socks.Version5, 0x99},
			closeEarly: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := pipeDialer(func(srv net.Conn) {
				defer srv.Close()
				buf := make([]byte, 128)
				// Read client greeting
				n, err := srv.Read(buf)
				if err != nil || n == 0 {
					return
				}
				if len(tc.response) > 0 {
					_, _ = srv.Write(tc.response)
				}
				if tc.closeEarly {
					return
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			conn, err := d.DialContext(ctx, "tcp", "example.com:80")
			if conn != nil {
				conn.Close()
				t.Fatalf("expected error on truncated/invalid auth response, got valid conn")
			}
			if err == nil {
				t.Fatalf("expected error on truncated/invalid auth response, got nil")
			}
		})
	}
}

// TestSOCKS5_TruncatedAuthSubnegotiation tests drops during Username/Password subnegotiation (RFC 1929).
func TestSOCKS5_TruncatedAuthSubnegotiation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		authReply   []byte
		closeBefore bool
	}{
		{
			name:        "DropBeforeSubnegReply_0Bytes",
			authReply:   nil,
			closeBefore: true,
		},
		{
			name:        "TruncatedSubnegReply_1Byte",
			authReply:   []byte{0x01}, // only version, missing status
			closeBefore: false,
		},
		{
			name:        "InvalidSubnegVersion_0x02",
			authReply:   []byte{0x02, 0x00},
			closeBefore: false,
		},
		{
			name:        "AuthFailureStatus_0x01",
			authReply:   []byte{0x01, 0x01},
			closeBefore: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := pipeDialer(func(srv net.Conn) {
				defer srv.Close()
				buf := make([]byte, 256)
				// 1. Initial greeting
				n, err := srv.Read(buf)
				if err != nil || n == 0 {
					return
				}
				// Reply agreeing to Username/Password (0x02)
				_, _ = srv.Write([]byte{socks.Version5, byte(socks.AuthMethodUsernamePassword)})

				// 2. Read subnegotiation credentials
				n, err = srv.Read(buf)
				if err != nil || n == 0 {
					return
				}

				if tc.closeBefore {
					return
				}
				if len(tc.authReply) > 0 {
					_, _ = srv.Write(tc.authReply)
				}
			})

			d.AuthMethods = []socks.AuthMethod{socks.AuthMethodUsernamePassword}
			d.Authenticate = (&socks.UsernamePassword{
				Username: "testuser",
				Password: "testpassword",
			}).Authenticate

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			conn, err := d.DialContext(ctx, "tcp", "example.com:80")
			if conn != nil {
				conn.Close()
				t.Fatalf("expected error on truncated subnegotiation, got valid conn")
			}
			if err == nil {
				t.Fatalf("expected error on truncated subnegotiation, got nil")
			}
		})
	}
}

// TestSOCKS5_TruncatedConnectReply tests server truncation during the SOCKS5 connect command reply.
func TestSOCKS5_TruncatedConnectReply(t *testing.T) {
	t.Parallel()

	// Standard SOCKS5 connect reply format:
	// VER(1) | REP(1) | RSV(1) | ATYP(1) | BND.ADDR(var) | BND.PORT(2)
	ipv4ReplySuccess := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x04, 0x38} // 127.0.0.1:1080

	cases := []struct {
		name       string
		replyBytes []byte
	}{
		{
			name:       "DropImmediatelyAfterCmd_0Bytes",
			replyBytes: nil,
		},
		{
			name:       "Truncated_1Byte_VerOnly",
			replyBytes: ipv4ReplySuccess[:1],
		},
		{
			name:       "Truncated_2Bytes_VerAndRep",
			replyBytes: ipv4ReplySuccess[:2],
		},
		{
			name:       "Truncated_3Bytes_VerRepRsv",
			replyBytes: ipv4ReplySuccess[:3],
		},
		{
			name:       "Truncated_4Bytes_AtypIPv4_MissingAddrAndPort",
			replyBytes: ipv4ReplySuccess[:4],
		},
		{
			name:       "Truncated_6Bytes_PartialIPv4",
			replyBytes: ipv4ReplySuccess[:6],
		},
		{
			name:       "Truncated_8Bytes_MissingPort",
			replyBytes: ipv4ReplySuccess[:8],
		},
		{
			name:       "InvalidReservedField_0xFF",
			replyBytes: []byte{0x05, 0x00, 0xFF, 0x01, 127, 0, 0, 1, 0x04, 0x38},
		},
		{
			name:       "UnsupportedAddressType_0x02",
			replyBytes: []byte{0x05, 0x00, 0x00, 0x02, 127, 0, 0, 1, 0x04, 0x38},
		},
		{
			name:       "Truncated_FQDN_LengthSays10_Only3BytesSent",
			replyBytes: []byte{0x05, 0x00, 0x00, 0x03, 10, 'a', 'b', 'c'},
		},
		{
			name:       "Truncated_IPv6_HeaderOnly",
			replyBytes: []byte{0x05, 0x00, 0x00, 0x04, 1, 2, 3, 4},
		},
		{
			name:       "ServerFailureReply_0x01",
			replyBytes: []byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
		},
		{
			name:       "ConnectionRefusedReply_0x05",
			replyBytes: []byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := pipeDialer(func(srv net.Conn) {
				defer srv.Close()
				buf := make([]byte, 256)
				// 1. Greeting
				_, err := srv.Read(buf)
				if err != nil {
					return
				}
				// Method selected: No Auth
				_, _ = srv.Write([]byte{socks.Version5, 0x00})

				// 2. Cmd connect
				_, err = srv.Read(buf)
				if err != nil {
					return
				}

				if len(tc.replyBytes) > 0 {
					_, _ = srv.Write(tc.replyBytes)
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			conn, err := d.DialContext(ctx, "tcp", "10.0.0.1:443")
			if conn != nil {
				conn.Close()
				t.Fatalf("expected error on truncated connect reply, got valid conn")
			}
			if err == nil {
				t.Fatalf("expected error on truncated connect reply, got nil")
			}
		})
	}
}

// TestSOCKS5_FQDNZeroLengthBoundAddr tests edge case where BND.ADDR is FQDN of length 0.
func TestSOCKS5_FQDNZeroLengthBoundAddr(t *testing.T) {
	t.Parallel()

	d := pipeDialer(func(srv net.Conn) {
		defer srv.Close()
		buf := make([]byte, 256)
		if _, err := srv.Read(buf); err != nil {
			return
		}
		_, _ = srv.Write([]byte{socks.Version5, 0x00})

		if _, err := srv.Read(buf); err != nil {
			return
		}
		// FQDN with len=0 and port 8080 (0x1f90)
		_, _ = srv.Write([]byte{0x05, 0x00, 0x00, 0x03, 0x00, 0x1f, 0x90})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", "target.domain:80")
	if err != nil {
		t.Fatalf("expected success with 0-length FQDN bound addr, got: %v", err)
	}
	defer conn.Close()

	socksConn, ok := conn.(*socks.Conn)
	if !ok {
		t.Fatalf("expected *socks.Conn, got %T", conn)
	}
	addr := socksConn.BoundAddr()
	if addr == nil {
		t.Fatalf("expected non-nil bound address")
	}
	sAddr, ok := addr.(*socks.Addr)
	if !ok {
		t.Fatalf("expected *socks.Addr, got %T", addr)
	}
	if sAddr.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", sAddr.Port)
	}
}

// TestSOCKS5_SimulatedConnectionDropsByteByByte systematically truncates at every byte boundary
// ensuring no panics, infinite loops, or goroutine leaks occur.
func TestSOCKS5_SimulatedConnectionDropsByteByByte(t *testing.T) {
	t.Parallel()

	// A valid full handshake stream for auth 0x00 and IPv4 target
	// Server sends:
	// Auth Selection: [0x05, 0x00] (2 bytes)
	// Connect Reply:  [0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x04, 0x38] (10 bytes)
	// Total server bytes: 12 bytes.

	fullServerBytes := []byte{
		0x05, 0x00, // Auth
		0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x04, 0x38, // Connect reply
	}

	for cut := 0; cut < len(fullServerBytes); cut++ {
		cut := cut
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			t.Parallel()

			d := pipeDialer(func(srv net.Conn) {
				defer srv.Close()
				buf := make([]byte, 256)

				// Step 1: Greeting
				_, err := srv.Read(buf)
				if err != nil {
					return
				}
				if cut < 2 {
					if cut > 0 {
						_, _ = srv.Write(fullServerBytes[:cut])
					}
					return
				}
				// Write full auth reply
				_, _ = srv.Write(fullServerBytes[:2])

				// Step 2: Connect command
				_, err = srv.Read(buf)
				if err != nil {
					return
				}
				if cut > 2 {
					_, _ = srv.Write(fullServerBytes[2:cut])
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			conn, err := d.DialContext(ctx, "tcp", "example.com:80")
			if conn != nil {
				conn.Close()
				t.Fatalf("expected error when server cut at byte %d, but got valid conn", cut)
			}
			if err == nil {
				t.Fatalf("expected error when server cut at byte %d, but got nil", cut)
			}
		})
	}
}

// TestSOCKS5_ConcurrentChaosAndRace runs concurrent dials with simulated random packet drops,
// cancellations, and timeouts under -race.
func TestSOCKS5_ConcurrentChaosAndRace(t *testing.T) {
	t.Parallel()

	concurrency := 30
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()

			d := pipeDialer(func(srv net.Conn) {
				defer srv.Close()
				buf := make([]byte, 256)

				// Flaky behavior depending on workerID
				switch workerID % 4 {
				case 0:
					// Immediate drop
					return
				case 1:
					// Truncated greeting reply
					_, _ = srv.Read(buf)
					_, _ = srv.Write([]byte{socks.Version5})
					return
				case 2:
					// Slow/timeout
					_, _ = srv.Read(buf)
					time.Sleep(50 * time.Millisecond)
					return
				case 3:
					// Success
					if _, err := srv.Read(buf); err != nil {
						return
					}
					_, _ = srv.Write([]byte{socks.Version5, 0x00})
					if _, err := srv.Read(buf); err != nil {
						return
					}
					_, _ = srv.Write([]byte{0x05, 0x00, 0x00, 0x01, 10, 0, 0, 1, 0x01, 0xBB})
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			conn, err := d.DialContext(ctx, "tcp", "10.0.0.1:443")
			if err == nil {
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()
	}

	wg.Wait()
}

// TestPoC_EXP1214_SOCKS5_RogueServer_DisallowedAuthMethod verifies that when the client offers AuthMethodNotRequired (0x00),
// if the proxy server returns a different method (e.g. 0x02 Username/Password or 0x01 GSSAPI),
// the client MUST reject it immediately per RFC 1928 Section 3 instead of skipping auth and proceeding to CONNECT.
func TestPoC_EXP1214_SOCKS5_RogueServer_DisallowedAuthMethod(t *testing.T) {
	t.Parallel()

	d := socks.NewDialer("tcp", "127.0.0.1:1080")
	// Anonymous client: d.Authenticate is nil, offers only 0x00 (AuthMethodNotRequired)
	d.ProxyDial = func(ctx context.Context, network, address string) (net.Conn, error) {
		cli, srv := net.Pipe()
		go func() {
			defer srv.Close()
			buf := make([]byte, 128)
			// Read greeting: client sends [0x05, 0x01, 0x00]
			n, err := srv.Read(buf)
			if err != nil || n == 0 {
				return
			}
			// Server responds that 0x02 (Username/Password) is required!
			_, _ = srv.Write([]byte{socks.Version5, 0x02})

			// Server now expects RFC 1929 subnegotiation [0x01, ...], NOT SOCKS5 CONNECT [0x05, 0x01, ...]!
			n, err = srv.Read(buf)
			if err == nil && n > 0 {
				if buf[0] == socks.Version5 && buf[1] == 0x01 {
					// Client erroneously sent CONNECT command!
					t.Errorf(
						"EXP-1214: SOCKS5 client sent CONNECT command instead of failing when server required auth method 0x02!",
					)
				}
			}
		}()
		return cli, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", "example.com:80")
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatalf("expected error when server selected unoffered auth method 0x02, got nil")
	}
}
