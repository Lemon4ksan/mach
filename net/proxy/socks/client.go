// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socks

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

var socksBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 512)
		return &b
	},
}

func isIPAddress(host string) bool {
	if len(host) == 0 {
		return false
	}

	dots := 0
	hasNonDigit := false

	for i := 0; i < len(host); i++ {
		c := host[i]
		switch {
		case c == '.':
			dots++
		case c == ':':
			return true
		case c < '0' || c > '9':
			hasNonDigit = true
		}
	}

	return !hasNonDigit && dots == 3
}

// Handshake executes the SOCKS5 handshake without returning a bound address (RFC 1928).
// It performs 0 heap allocations on the hot path.
func (d *Dialer) Handshake(ctx context.Context, c net.Conn, address string) error {
	host, port, err := splitHostPortNoAlloc(address)
	if err != nil {
		return err
	}

	if len(host) > 255 {
		return errors.New("FQDN too long")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var stop func() bool
	if ctx.Done() != nil {
		if dl, ok := ctx.Deadline(); ok {
			_ = c.SetDeadline(dl)
		}

		stop = context.AfterFunc(ctx, func() {
			_ = c.SetDeadline(time.Unix(1, 0))
		})
		defer func() {
			stop()

			_ = c.SetDeadline(time.Time{})
		}()
	}

	bufPtr := socksBufPool.Get().(*[]byte)
	defer socksBufPool.Put(bufPtr)

	buf := *bufPtr

	// 1. Initial Greeting
	buf[0] = Version5

	var n int
	if len(d.AuthMethods) == 0 || d.Authenticate == nil {
		buf[1] = 1
		buf[2] = byte(AuthMethodNotRequired)
		n = 3
	} else {
		ams := d.AuthMethods
		if len(ams) > 255 {
			return errors.New("too many authentication methods")
		}

		buf[1] = byte(len(ams))
		for i, am := range ams {
			buf[2+i] = byte(am)
		}

		n = 2 + len(ams)
	}

	if _, err := c.Write(buf[:n]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return err
	}

	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return err
	}

	if buf[0] != Version5 {
		return errors.New("unexpected protocol version " + strconv.Itoa(int(buf[0])))
	}

	am := AuthMethod(buf[1])
	if am == AuthMethodNoAcceptableMethods {
		return errors.New("no acceptable authentication methods")
	}

	if d.Authenticate == nil {
		if am != AuthMethodNotRequired {
			return errors.New("unsupported or unoffered authentication method: " + strconv.Itoa(int(am)))
		}
	} else {
		found := false
		for _, m := range d.AuthMethods {
			if m == am {
				found = true
				break
			}
		}

		if !found {
			return errors.New("server selected unoffered authentication method: " + strconv.Itoa(int(am)))
		}

		if err := d.Authenticate(ctx, c, am); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			return err
		}
	}

	// 2. CONNECT / BIND Command
	buf[0] = Version5
	buf[1] = byte(d.cmd)
	buf[2] = 0 // RSV

	if isIPAddress(host) {
		if addr, err := netip.ParseAddr(host); err == nil {
			if addr.Is4() {
				buf[3] = AddrTypeIPv4
				a4 := addr.As4()
				copy(buf[4:8], a4[:])

				n = 8
			} else {
				buf[3] = AddrTypeIPv6
				a16 := addr.As16()
				copy(buf[4:20], a16[:])

				n = 20
			}
		} else {
			if len(host) > 255 {
				return errors.New("FQDN too long")
			}

			buf[3] = AddrTypeFQDN
			buf[4] = byte(len(host))
			copy(buf[5:], host)
			n = 5 + len(host)
		}
	} else {
		if len(host) > 255 {
			return errors.New("FQDN too long")
		}

		buf[3] = AddrTypeFQDN
		buf[4] = byte(len(host))
		copy(buf[5:], host)
		n = 5 + len(host)
	}

	buf[n] = byte(port >> 8)
	buf[n+1] = byte(port)
	tot := n + 2

	if _, err := c.Write(buf[:tot]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return err
	}

	// 3. Response
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return err
	}

	if buf[0] != Version5 {
		return errors.New("unexpected protocol version " + strconv.Itoa(int(buf[0])))
	}

	if cmdErr := Reply(buf[1]); cmdErr != StatusSucceeded {
		return errors.New("unknown error " + cmdErr.String())
	}

	if buf[2] != 0 {
		return errors.New("non-zero reserved field")
	}

	// Read and discard bound address
	switch buf[3] {
	case AddrTypeIPv4:
		if _, err := io.ReadFull(c, buf[:6]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			return err
		}

	case AddrTypeIPv6:
		if _, err := io.ReadFull(c, buf[:18]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			return err
		}

	case AddrTypeFQDN:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			return err
		}

		l := int(buf[0])
		if _, err := io.ReadFull(c, buf[:l+2]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			return err
		}

	default:
		return errors.New("unknown address type " + strconv.Itoa(int(buf[3])))
	}

	return nil
}

func (d *Dialer) connect(ctx context.Context, c net.Conn, address string) (net.Addr, error) {
	host, port, err := splitHostPortNoAlloc(address)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var stop func() bool
	if ctx.Done() != nil {
		if dl, ok := ctx.Deadline(); ok {
			_ = c.SetDeadline(dl)
		}

		stop = context.AfterFunc(ctx, func() {
			_ = c.SetDeadline(time.Unix(1, 0))
		})
		defer func() {
			stop()

			_ = c.SetDeadline(time.Time{})
		}()
	}

	bufPtr := socksBufPool.Get().(*[]byte)
	defer socksBufPool.Put(bufPtr)

	buf := *bufPtr

	// 1. Initial Greeting
	buf[0] = Version5

	var n int
	if len(d.AuthMethods) == 0 || d.Authenticate == nil {
		buf[1] = 1
		buf[2] = byte(AuthMethodNotRequired)
		n = 3
	} else {
		ams := d.AuthMethods
		if len(ams) > 255 {
			return nil, errors.New("too many authentication methods")
		}

		buf[1] = byte(len(ams))
		for i, am := range ams {
			buf[2+i] = byte(am)
		}

		n = 2 + len(ams)
	}

	if _, err := c.Write(buf[:n]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		return nil, err
	}

	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		return nil, err
	}

	if buf[0] != Version5 {
		return nil, errors.New("unexpected protocol version " + strconv.Itoa(int(buf[0])))
	}

	am := AuthMethod(buf[1])
	if am == AuthMethodNoAcceptableMethods {
		return nil, errors.New("no acceptable authentication methods")
	}

	if d.Authenticate == nil {
		if am != AuthMethodNotRequired {
			return nil, errors.New("unsupported or unoffered authentication method: " + strconv.Itoa(int(am)))
		}
	} else {
		found := false
		for _, m := range d.AuthMethods {
			if m == am {
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("server selected unoffered authentication method: " + strconv.Itoa(int(am)))
		}

		if err := d.Authenticate(ctx, c, am); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}

			return nil, err
		}
	}

	// 2. CONNECT / BIND Command
	buf[0] = Version5
	buf[1] = byte(d.cmd)
	buf[2] = 0 // RSV

	if isIPAddress(host) {
		if addr, err := netip.ParseAddr(host); err == nil {
			if addr.Is4() {
				buf[3] = AddrTypeIPv4
				a4 := addr.As4()
				copy(buf[4:8], a4[:])

				n = 8
			} else {
				buf[3] = AddrTypeIPv6
				a16 := addr.As16()
				copy(buf[4:20], a16[:])

				n = 20
			}
		} else {
			if len(host) > 255 {
				return nil, errors.New("FQDN too long")
			}

			buf[3] = AddrTypeFQDN
			buf[4] = byte(len(host))
			copy(buf[5:], host)
			n = 5 + len(host)
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("FQDN too long")
		}

		buf[3] = AddrTypeFQDN
		buf[4] = byte(len(host))
		copy(buf[5:], host)
		n = 5 + len(host)
	}

	buf[n] = byte(port >> 8)
	buf[n+1] = byte(port)
	tot := n + 2

	if _, err := c.Write(buf[:tot]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		return nil, err
	}

	// 3. Response
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		return nil, err
	}

	if buf[0] != Version5 {
		return nil, errors.New("unexpected protocol version " + strconv.Itoa(int(buf[0])))
	}

	if cmdErr := Reply(buf[1]); cmdErr != StatusSucceeded {
		return nil, errors.New("unknown error " + cmdErr.String())
	}

	if buf[2] != 0 {
		return nil, errors.New("non-zero reserved field")
	}

	var a Addr
	switch buf[3] {
	case AddrTypeIPv4:
		if _, err := io.ReadFull(c, buf[:6]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}

			return nil, err
		}

		a.IP = net.IPv4(buf[0], buf[1], buf[2], buf[3])
		a.Port = int(buf[4])<<8 | int(buf[5])

	case AddrTypeIPv6:
		if _, err := io.ReadFull(c, buf[:18]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}

			return nil, err
		}

		ip := make(net.IP, 16)
		copy(ip, buf[:16])
		a.IP = ip
		a.Port = int(buf[16])<<8 | int(buf[17])

	case AddrTypeFQDN:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}

			return nil, err
		}

		l := int(buf[0])
		if _, err := io.ReadFull(c, buf[:l+2]); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}

			return nil, err
		}

		a.Name = string(buf[:l])
		a.Port = int(buf[l])<<8 | int(buf[l+1])

	default:
		return nil, errors.New("unknown address type " + strconv.Itoa(int(buf[3])))
	}

	return &a, nil
}

func splitHostPortNoAlloc(address string) (host string, port int, err error) {
	colon := -1
	for i := len(address) - 1; i >= 0; i-- {
		if address[i] == ':' {
			colon = i
			break
		}
	}

	if colon < 0 {
		return "", 0, errors.New("missing port in address")
	}

	host = address[:colon]
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}

	portStr := address[colon+1:]
	if len(portStr) == 0 {
		return "", 0, errors.New("empty port in address")
	}

	p := 0
	for i := 0; i < len(portStr); i++ {
		b := portStr[i]
		if b < '0' || b > '9' {
			return "", 0, errors.New("invalid port")
		}

		p = p*10 + int(b-'0')
		if p > 65535 {
			return "", 0, errors.New("port out of range")
		}
	}

	if p == 0 {
		return "", 0, errors.New("port out of range 0")
	}

	return host, p, nil
}

func splitHostPort(address string) (string, int, error) {
	return splitHostPortNoAlloc(address)
}
