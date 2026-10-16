// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import "testing"

func BenchmarkAppendHeader_Short(b *testing.B) {
	buf := make([]byte, 64)
	h := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      []byte{1, 2, 3, 4, 5, 6, 7, 8},
		PacketNumber:    12345,
		PacketNumberLen: 2,
		SpinBit:         true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, _, _ := AppendHeader(buf[:0], &h)
		_ = out
	}
}

func BenchmarkParseHeader_Short(b *testing.B) {
	buf := make([]byte, 64)
	h := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      []byte{1, 2, 3, 4, 5, 6, 7, 8},
		PacketNumber:    12345,
		PacketNumberLen: 2,
		SpinBit:         true,
	}
	pkt, _, _ := AppendHeader(buf[:0], &h)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hdr, _ := ParseHeader(pkt, 8, 12344)
		_ = hdr
	}
}

func BenchmarkAppendStreamFrame(b *testing.B) {
	buf := make([]byte, 256)
	data := []byte("benchmark-stream-frame-payload")
	s := StreamFrame{
		StreamID: 4,
		Offset:   1024,
		Fin:      false,
		HasLen:   true,
		Data:     data,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := AppendStreamFrame(buf[:0], &s)
		_ = out
	}
}

func BenchmarkParseStreamFrame(b *testing.B) {
	buf := make([]byte, 256)
	data := []byte("benchmark-stream-frame-payload")
	s := StreamFrame{
		StreamID: 4,
		Offset:   1024,
		Fin:      false,
		HasLen:   true,
		Data:     data,
	}
	frameBytes := AppendStreamFrame(buf[:0], &s)
	var f Frame

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, _ := ParseNextFrame(frameBytes, &f)
		_ = n
	}
}

func BenchmarkAppendAckFrame(b *testing.B) {
	buf := make([]byte, 256)
	ack := AckFrame{
		LargestAcked:  10000,
		AckDelay:      15,
		FirstAckRange: 5,
		HasECN:        true,
		ECN:           ECNCounts{ECT0: 10, ECT1: 20, ECNCE: 30},
	}
	ack.AppendRange(2, 4)
	ack.AppendRange(5, 1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := AppendAckFrame(buf[:0], &ack)
		_ = out
	}
}

func BenchmarkParseAckFrame(b *testing.B) {
	buf := make([]byte, 256)
	ack := AckFrame{
		LargestAcked:  10000,
		AckDelay:      15,
		FirstAckRange: 5,
		HasECN:        true,
		ECN:           ECNCounts{ECT0: 10, ECT1: 20, ECNCE: 30},
	}
	ack.AppendRange(2, 4)
	ack.AppendRange(5, 1)
	raw := AppendAckFrame(buf[:0], &ack)
	var f Frame

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, _ := ParseNextFrame(raw, &f)
		_ = n
	}
}

func BenchmarkHeaderProtection_AES(b *testing.B) {
	hp, _ := NewAESHeaderProtector(make([]byte, 16))
	sample := make([]byte, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := hp.Mask(sample)
		_ = m
	}
}

func BenchmarkHeaderProtection_ChaCha(b *testing.B) {
	hp, _ := NewChaChaHeaderProtector(make([]byte, 32))
	sample := make([]byte, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := hp.Mask(sample)
		_ = m
	}
}

func BenchmarkPacketSeal_AES128GCM(b *testing.B) {
	key := make([]byte, 16)
	iv := make([]byte, 12)
	hpKey := make([]byte, 16)
	p, _ := NewPacketProtector(TLS_AES_128_GCM_SHA256, key, iv, hpKey)

	h := Header{
		IsLong:          false,
		Type:            PacketType1RTT,
		DestConnID:      []byte{1, 2, 3, 4},
		PacketNumber:    100,
		PacketNumberLen: 2,
	}
	payload := make([]byte, 1200)
	dst := make([]byte, 0, 1500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, _ := p.SealPacket(dst[:0], &h, payload)
		_ = out
	}
}
