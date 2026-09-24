// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tcp_test

import (
	"encoding/binary"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/lemon4ksan/mach/proto/packet/tcp"
)

func TestStressClampMSSInPlace_OptionsTorture(t *testing.T) {
	// Construct options with all permutations of NOPs, malformed lengths, and MSS options
	r := rand.New(rand.NewPCG(42, 42)) //nolint:gosec

	for trial := 0; trial < 200; trial++ {
		var optBuf []byte
		// Prepend 0..10 NOPs
		nopCount := r.IntN(10)
		for i := 0; i < nopCount; i++ {
			optBuf = append(optBuf, 1)
		}

		// Inject an arbitrary option (can be malformed)
		kind := byte(r.IntN(10))
		switch kind {
		case 0: // EOL
			optBuf = append(optBuf, 0)
		case 1: // NOP
			optBuf = append(optBuf, 1)
		case 2: // MSS
			optLen := byte(r.IntN(6)) // lengths 0..5
			optBuf = append(optBuf, 2, optLen)
			if optLen > 2 {
				for i := byte(0); i < optLen-2; i++ {
					optBuf = append(optBuf, byte(r.IntN(256)))
				}
			}
		default: // other option
			optLen := byte(r.IntN(10))
			optBuf = append(optBuf, kind, optLen)
			if optLen > 2 {
				for i := byte(0); i < optLen-2; i++ {
					optBuf = append(optBuf, byte(r.IntN(256)))
				}
			}
		}

		// Append a valid MSS option at the end
		optBuf = append(optBuf, 2, 4, 0x05, 0xb4) // MSS 1460

		pkt := makeIPv4TCPPacket(0x02, 0, optBuf, 0)
		// ClampMSSInPlace must not panic or hang
		tcp.ClampMSSInPlace(pkt, 1300)
	}
}

func TestStressClampMSSInPlace_OddLengthsAndChecksums(t *testing.T) {
	for payloadLen := 0; payloadLen <= 15; payloadLen++ {
		// IPv4 SYN packet
		pkt4 := makeIPv4TCPPacket(0x02, 1460, nil, payloadLen)
		tcp.ClampMSSInPlace(pkt4, 1300)
		if mss := binary.BigEndian.Uint16(pkt4[42:44]); mss != 1260 {
			t.Fatalf("IPv4 payloadLen %d: clamped MSS = %d, want 1260", payloadLen, mss)
		}
		if !verifyTCPChecksum(pkt4, 4, 20) {
			t.Fatalf("IPv4 payloadLen %d: TCP checksum invalid", payloadLen)
		}

		// IPv6 SYN packet
		pkt6 := makeIPv6TCPPacket(0x02, 1440, nil, payloadLen)
		tcp.ClampMSSInPlace(pkt6, 1400)
		if mss := binary.BigEndian.Uint16(pkt6[62:64]); mss != 1340 {
			t.Fatalf("IPv6 payloadLen %d: clamped MSS = %d, want 1340", payloadLen, mss)
		}
		if !verifyTCPChecksum(pkt6, 6, 40) {
			t.Fatalf("IPv6 payloadLen %d: TCP checksum invalid", payloadLen)
		}
	}
}

func TestStressClampMSSInPlace_Parallel(t *testing.T) {
	const workers = 8
	const iters = 500

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				pkt := makeIPv4TCPPacket(0x02, 1460, nil, (workerID+i)%5)
				tcp.ClampMSSInPlace(pkt, 1300)
				if mss := binary.BigEndian.Uint16(pkt[42:44]); mss != 1260 {
					t.Errorf("worker %d iter %d: clamped MSS = %d, want 1260", workerID, i, mss)
					return
				}
				if !verifyTCPChecksum(pkt, 4, 20) {
					t.Errorf("worker %d iter %d: TCP checksum verification failed", workerID, i)
					return
				}
			}
		}(w)
	}

	wg.Wait()
}
