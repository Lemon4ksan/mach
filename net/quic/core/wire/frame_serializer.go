// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

var zeroBuf [64]byte

// AppendPadding appends count PADDING bytes to dst with zero heap allocations.
func AppendPadding(dst []byte, count int) []byte {
	for count > 0 {
		n := count
		if n > len(zeroBuf) {
			n = len(zeroBuf)
		}
		dst = append(dst, zeroBuf[:n]...)
		count -= n
	}
	return dst
}

// AppendPing appends a PING frame to dst.
func AppendPing(dst []byte) []byte {
	return append(dst, byte(FrameTypePing))
}

// AppendAckFrame appends an ACK frame to dst.
func AppendAckFrame(dst []byte, a *AckFrame) []byte {
	if a.HasECN {
		dst = append(dst, byte(FrameTypeAckECN))
	} else {
		dst = append(dst, byte(FrameTypeAck))
	}

	dst = appendVarint(dst, a.LargestAcked)
	dst = appendVarint(dst, a.AckDelay)
	dst = appendVarint(dst, uint64(a.RangeCount()))
	dst = appendVarint(dst, a.FirstAckRange)

	count := a.RangeCount()
	for i := 0; i < count; i++ {
		r := a.Range(i)
		dst = appendVarint(dst, r.Gap)
		dst = appendVarint(dst, r.Length)
	}

	if a.HasECN {
		dst = appendVarint(dst, a.ECN.ECT0)
		dst = appendVarint(dst, a.ECN.ECT1)
		dst = appendVarint(dst, a.ECN.ECNCE)
	}

	return dst
}

// AppendResetStreamFrame appends a RESET_STREAM frame to dst.
func AppendResetStreamFrame(dst []byte, r *ResetStreamFrame) []byte {
	dst = append(dst, byte(FrameTypeResetStream))
	dst = appendVarint(dst, r.StreamID)
	dst = appendVarint(dst, r.ErrorCode)
	dst = appendVarint(dst, r.FinalSize)
	return dst
}

// AppendStopSendingFrame appends a STOP_SENDING frame to dst.
func AppendStopSendingFrame(dst []byte, s *StopSendingFrame) []byte {
	dst = append(dst, byte(FrameTypeStopSending))
	dst = appendVarint(dst, s.StreamID)
	dst = appendVarint(dst, s.ErrorCode)
	return dst
}

// AppendCryptoFrame appends a CRYPTO frame to dst.
func AppendCryptoFrame(dst []byte, c *CryptoFrame) []byte {
	dst = append(dst, byte(FrameTypeCrypto))
	dst = appendVarint(dst, c.Offset)
	dst = appendVarint(dst, uint64(len(c.Data)))
	dst = append(dst, c.Data...)
	return dst
}

// AppendNewTokenFrame appends a NEW_TOKEN frame to dst.
func AppendNewTokenFrame(dst []byte, n *NewTokenFrame) []byte {
	dst = append(dst, byte(FrameTypeNewToken))
	dst = appendVarint(dst, uint64(len(n.Token)))
	dst = append(dst, n.Token...)
	return dst
}

// AppendStreamFrame appends a STREAM frame to dst.
func AppendStreamFrame(dst []byte, s *StreamFrame) []byte {
	typ := byte(FrameTypeStream)
	if s.Offset != 0 {
		typ |= 0x04
	}
	if s.HasLen {
		typ |= 0x02
	}
	if s.Fin {
		typ |= 0x01
	}

	dst = append(dst, typ)
	dst = appendVarint(dst, s.StreamID)
	if s.Offset != 0 {
		dst = appendVarint(dst, s.Offset)
	}
	if s.HasLen {
		dst = appendVarint(dst, uint64(len(s.Data)))
	}
	dst = append(dst, s.Data...)
	return dst
}

// AppendMaxDataFrame appends a MAX_DATA frame to dst.
func AppendMaxDataFrame(dst []byte, m *MaxDataFrame) []byte {
	dst = append(dst, byte(FrameTypeMaxData))
	dst = appendVarint(dst, m.MaximumData)
	return dst
}

// AppendMaxStreamDataFrame appends a MAX_STREAM_DATA frame to dst.
func AppendMaxStreamDataFrame(dst []byte, m *MaxStreamDataFrame) []byte {
	dst = append(dst, byte(FrameTypeMaxStreamData))
	dst = appendVarint(dst, m.StreamID)
	dst = appendVarint(dst, m.MaximumStreamData)
	return dst
}

// AppendMaxStreamsFrame appends a MAX_STREAMS frame to dst.
func AppendMaxStreamsFrame(dst []byte, m *MaxStreamsFrame) []byte {
	if m.Unidirectional {
		dst = append(dst, byte(FrameTypeMaxStreamsUni))
	} else {
		dst = append(dst, byte(FrameTypeMaxStreamsBidi))
	}
	dst = appendVarint(dst, m.MaxStreams)
	return dst
}

// AppendDataBlockedFrame appends a DATA_BLOCKED frame to dst.
func AppendDataBlockedFrame(dst []byte, d *DataBlockedFrame) []byte {
	dst = append(dst, byte(FrameTypeDataBlocked))
	dst = appendVarint(dst, d.MaximumData)
	return dst
}

// AppendStreamDataBlockedFrame appends a STREAM_DATA_BLOCKED frame to dst.
func AppendStreamDataBlockedFrame(dst []byte, s *StreamDataBlockedFrame) []byte {
	dst = append(dst, byte(FrameTypeStreamDataBlocked))
	dst = appendVarint(dst, s.StreamID)
	dst = appendVarint(dst, s.MaximumStreamData)
	return dst
}

// AppendStreamsBlockedFrame appends a STREAMS_BLOCKED frame to dst.
func AppendStreamsBlockedFrame(dst []byte, s *StreamsBlockedFrame) []byte {
	if s.Unidirectional {
		dst = append(dst, byte(FrameTypeStreamsBlockedUni))
	} else {
		dst = append(dst, byte(FrameTypeStreamsBlockedBidi))
	}
	dst = appendVarint(dst, s.StreamLimit)
	return dst
}

// AppendNewConnectionIDFrame appends a NEW_CONNECTION_ID frame to dst.
func AppendNewConnectionIDFrame(dst []byte, n *NewConnectionIDFrame) []byte {
	dst = append(dst, byte(FrameTypeNewConnectionID))
	dst = appendVarint(dst, n.SequenceNumber)
	dst = appendVarint(dst, n.RetirePriorTo)
	dst = append(dst, byte(len(n.ConnID)))
	dst = append(dst, n.ConnID...)
	dst = append(dst, n.StatelessResetToken[:]...)
	return dst
}

// AppendRetireConnectionIDFrame appends a RETIRE_CONNECTION_ID frame to dst.
func AppendRetireConnectionIDFrame(dst []byte, r *RetireConnectionIDFrame) []byte {
	dst = append(dst, byte(FrameTypeRetireConnectionID))
	dst = appendVarint(dst, r.SequenceNumber)
	return dst
}

// AppendPathChallengeFrame appends a PATH_CHALLENGE frame to dst.
func AppendPathChallengeFrame(dst []byte, p *PathChallengeFrame) []byte {
	dst = append(dst, byte(FrameTypePathChallenge))
	dst = append(dst, p.Data[:]...)
	return dst
}

// AppendPathResponseFrame appends a PATH_RESPONSE frame to dst.
func AppendPathResponseFrame(dst []byte, p *PathResponseFrame) []byte {
	dst = append(dst, byte(FrameTypePathResponse))
	dst = append(dst, p.Data[:]...)
	return dst
}

// AppendConnectionCloseFrame appends a CONNECTION_CLOSE frame to dst.
func AppendConnectionCloseFrame(dst []byte, c *ConnectionCloseFrame) []byte {
	if c.IsApplication {
		dst = append(dst, byte(FrameTypeConnectionCloseApp))
		dst = appendVarint(dst, c.ErrorCode)
	} else {
		dst = append(dst, byte(FrameTypeConnectionCloseTransport))
		dst = appendVarint(dst, c.ErrorCode)
		dst = appendVarint(dst, c.FrameType)
	}
	dst = appendVarint(dst, uint64(len(c.Reason)))
	dst = append(dst, c.Reason...)
	return dst
}

// AppendHandshakeDoneFrame appends a HANDSHAKE_DONE frame to dst.
func AppendHandshakeDoneFrame(dst []byte) []byte {
	return append(dst, byte(FrameTypeHandshakeDone))
}

// AppendDatagramFrame appends an RFC 9221 DATAGRAM frame to dst.
func AppendDatagramFrame(dst []byte, d *DatagramFrame) []byte {
	if d.HasLen {
		dst = append(dst, byte(FrameTypeDatagramWithLen))
		dst = appendVarint(dst, uint64(len(d.Data)))
	} else {
		dst = append(dst, byte(FrameTypeDatagram))
	}
	dst = append(dst, d.Data...)
	return dst
}

// AppendFrame appends any QUIC frame to dst according to f.Type.
func AppendFrame(dst []byte, f *Frame) []byte {
	switch f.Type {
	case FrameTypePadding:
		return AppendPadding(dst, f.PaddingCount)
	case FrameTypePing:
		return AppendPing(dst)
	case FrameTypeAck, FrameTypeAckECN:
		return AppendAckFrame(dst, &f.Ack)
	case FrameTypeResetStream:
		return AppendResetStreamFrame(dst, &f.ResetStream)
	case FrameTypeStopSending:
		return AppendStopSendingFrame(dst, &f.StopSending)
	case FrameTypeCrypto:
		return AppendCryptoFrame(dst, &f.Crypto)
	case FrameTypeNewToken:
		return AppendNewTokenFrame(dst, &f.NewToken)
	case FrameTypeMaxData:
		return AppendMaxDataFrame(dst, &f.MaxData)
	case FrameTypeMaxStreamData:
		return AppendMaxStreamDataFrame(dst, &f.MaxStreamData)
	case FrameTypeMaxStreamsBidi, FrameTypeMaxStreamsUni:
		return AppendMaxStreamsFrame(dst, &f.MaxStreams)
	case FrameTypeDataBlocked:
		return AppendDataBlockedFrame(dst, &f.DataBlocked)
	case FrameTypeStreamDataBlocked:
		return AppendStreamDataBlockedFrame(dst, &f.StreamDataBlocked)
	case FrameTypeStreamsBlockedBidi, FrameTypeStreamsBlockedUni:
		return AppendStreamsBlockedFrame(dst, &f.StreamsBlocked)
	case FrameTypeNewConnectionID:
		return AppendNewConnectionIDFrame(dst, &f.NewConnectionID)
	case FrameTypeRetireConnectionID:
		return AppendRetireConnectionIDFrame(dst, &f.RetireConnectionID)
	case FrameTypePathChallenge:
		return AppendPathChallengeFrame(dst, &f.PathChallenge)
	case FrameTypePathResponse:
		return AppendPathResponseFrame(dst, &f.PathResponse)
	case FrameTypeConnectionCloseTransport, FrameTypeConnectionCloseApp:
		return AppendConnectionCloseFrame(dst, &f.ConnectionClose)
	case FrameTypeHandshakeDone:
		return AppendHandshakeDoneFrame(dst)
	case FrameTypeDatagram, FrameTypeDatagramWithLen:
		return AppendDatagramFrame(dst, &f.Datagram)
	default:
		if f.Type >= 0x08 && f.Type <= 0x0f {
			return AppendStreamFrame(dst, &f.Stream)
		}
		panic("wire: unhandled frame type")
	}
}
