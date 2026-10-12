// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"io"
	"iter"

	"github.com/lemon4ksan/mach/net/quic/varint"
)

// ParseNextFrame parses the next frame from b into f without heap allocations.
// It returns the number of bytes consumed from b, or an error if b is truncated or malformed.
// When b is empty, it returns 0, io.EOF.
func ParseNextFrame(b []byte, f *Frame) (int, error) {
	if len(b) == 0 {
		return 0, io.EOF
	}

	// PADDING fast-path: collapse runs of consecutive 0x00 bytes
	if b[0] == 0x00 {
		f.Reset()
		f.Type = FrameTypePadding

		padCount := 0
		for padCount < len(b) && b[padCount] == 0x00 {
			padCount++
		}

		f.PaddingCount = padCount

		return padCount, nil
	}

	rawType, n, err := varint.Decode(b)
	if err != nil {
		return 0, ErrTruncatedFrame
	}

	f.Reset()

	offset := n

	// Handle STREAM frame range (0x08 - 0x0f)
	if rawType >= 0x08 && rawType <= 0x0f {
		f.Type = FrameType(rawType)
		hasOff := (rawType & 0x04) != 0
		hasLen := (rawType & 0x02) != 0
		fin := (rawType & 0x01) != 0

		streamID, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		var streamOffset uint64
		if hasOff {
			streamOffset, n, err = varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n
		}

		var data []byte
		if hasLen {
			dataLen, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n
			if uint64(len(b)-offset) < dataLen {
				return 0, ErrTruncatedFrame
			}

			data = b[offset : offset+int(dataLen)]
			offset += int(dataLen)
		} else {
			data = b[offset:]
			offset = len(b)
		}

		f.Stream = StreamFrame{
			StreamID: streamID,
			Offset:   streamOffset,
			Fin:      fin,
			HasLen:   hasLen,
			Data:     data,
		}

		return offset, nil
	}

	typ := FrameType(rawType)
	f.Type = typ

	switch typ {
	case FrameTypePing:
		return offset, nil

	case FrameTypeAck, FrameTypeAckECN:
		largestAcked, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		ackDelay, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		rangeCount, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		firstRange, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		if firstRange > largestAcked {
			return 0, ErrTruncatedFrame
		}

		f.Ack.LargestAcked = largestAcked
		f.Ack.AckDelay = ackDelay
		f.Ack.FirstAckRange = firstRange

		for i := uint64(0); i < rangeCount; i++ {
			gap, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n

			length, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n

			f.Ack.AppendRange(gap, length)
		}

		if typ == FrameTypeAckECN {
			f.Ack.HasECN = true

			ect0, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n

			ect1, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n

			ecnce, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n

			f.Ack.ECN = ECNCounts{
				ECT0:  ect0,
				ECT1:  ect1,
				ECNCE: ecnce,
			}
		}

		return offset, nil

	case FrameTypeResetStream:
		streamID, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		errorCode, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		finalSize, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		f.ResetStream = ResetStreamFrame{
			StreamID:  streamID,
			ErrorCode: errorCode,
			FinalSize: finalSize,
		}

		return offset, nil

	case FrameTypeStopSending:
		streamID, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		errorCode, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		f.StopSending = StopSendingFrame{
			StreamID:  streamID,
			ErrorCode: errorCode,
		}

		return offset, nil

	case FrameTypeCrypto:
		cryptoOffset, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		length, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		if uint64(len(b)-offset) < length {
			return 0, ErrTruncatedFrame
		}

		f.Crypto = CryptoFrame{
			Offset: cryptoOffset,
			Data:   b[offset : offset+int(length)],
		}
		offset += int(length)

		return offset, nil

	case FrameTypeNewToken:
		length, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		if uint64(len(b)-offset) < length {
			return 0, ErrTruncatedFrame
		}

		f.NewToken = NewTokenFrame{
			Token: b[offset : offset+int(length)],
		}
		offset += int(length)

		return offset, nil

	case FrameTypeMaxData:
		maxData, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.MaxData = MaxDataFrame{MaximumData: maxData}

		return offset, nil

	case FrameTypeMaxStreamData:
		streamID, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		maxStreamData, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.MaxStreamData = MaxStreamDataFrame{
			StreamID:          streamID,
			MaximumStreamData: maxStreamData,
		}

		return offset, nil

	case FrameTypeMaxStreamsBidi, FrameTypeMaxStreamsUni:
		maxStreams, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.MaxStreams = MaxStreamsFrame{
			MaxStreams:     maxStreams,
			Unidirectional: typ == FrameTypeMaxStreamsUni,
		}

		return offset, nil

	case FrameTypeDataBlocked:
		maxData, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.DataBlocked = DataBlockedFrame{MaximumData: maxData}

		return offset, nil

	case FrameTypeStreamDataBlocked:
		streamID, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		maxStreamData, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.StreamDataBlocked = StreamDataBlockedFrame{
			StreamID:          streamID,
			MaximumStreamData: maxStreamData,
		}

		return offset, nil

	case FrameTypeStreamsBlockedBidi, FrameTypeStreamsBlockedUni:
		streamLimit, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.StreamsBlocked = StreamsBlockedFrame{
			StreamLimit:    streamLimit,
			Unidirectional: typ == FrameTypeStreamsBlockedUni,
		}

		return offset, nil

	case FrameTypeNewConnectionID:
		seq, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		retirePriorTo, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		if len(b) < offset+1 {
			return 0, ErrTruncatedFrame
		}

		cidLen := int(b[offset])

		offset++
		if cidLen < 1 || cidLen > 20 || len(b) < offset+cidLen+16 {
			return 0, ErrTruncatedFrame
		}

		f.NewConnectionID.SequenceNumber = seq
		f.NewConnectionID.RetirePriorTo = retirePriorTo
		f.NewConnectionID.ConnID = b[offset : offset+cidLen]
		offset += cidLen

		copy(f.NewConnectionID.StatelessResetToken[:], b[offset:offset+16])
		offset += 16

		return offset, nil

	case FrameTypeRetireConnectionID:
		seq, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n
		f.RetireConnectionID = RetireConnectionIDFrame{SequenceNumber: seq}

		return offset, nil

	case FrameTypePathChallenge:
		if len(b) < offset+8 {
			return 0, ErrTruncatedFrame
		}

		copy(f.PathChallenge.Data[:], b[offset:offset+8])
		offset += 8

		return offset, nil

	case FrameTypePathResponse:
		if len(b) < offset+8 {
			return 0, ErrTruncatedFrame
		}

		copy(f.PathResponse.Data[:], b[offset:offset+8])
		offset += 8

		return offset, nil

	case FrameTypeConnectionCloseTransport, FrameTypeConnectionCloseApp:
		errorCode, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		var triggerFrameType uint64
		if typ == FrameTypeConnectionCloseTransport {
			triggerFrameType, n, err = varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n
		}

		reasonLen, n, err := varint.Decode(b[offset:])
		if err != nil {
			return 0, ErrTruncatedFrame
		}

		offset += n

		if uint64(len(b)-offset) < reasonLen {
			return 0, ErrTruncatedFrame
		}

		f.ConnectionClose = ConnectionCloseFrame{
			IsApplication: typ == FrameTypeConnectionCloseApp,
			ErrorCode:     errorCode,
			FrameType:     triggerFrameType,
			Reason:        b[offset : offset+int(reasonLen)],
		}
		offset += int(reasonLen)

		return offset, nil

	case FrameTypeHandshakeDone:
		return offset, nil

	case FrameTypeDatagram, FrameTypeDatagramWithLen:
		if typ == FrameTypeDatagramWithLen {
			dataLen, n, err := varint.Decode(b[offset:])
			if err != nil {
				return 0, ErrTruncatedFrame
			}

			offset += n
			if uint64(len(b)-offset) < dataLen {
				return 0, ErrTruncatedFrame
			}

			f.Datagram = DatagramFrame{
				HasLen: true,
				Data:   b[offset : offset+int(dataLen)],
			}
			offset += int(dataLen)
		} else {
			f.Datagram = DatagramFrame{
				HasLen: false,
				Data:   b[offset:],
			}
			offset = len(b)
		}

		return offset, nil

	default:
		return 0, ErrInvalidFrameType
	}
}

// Frames returns a push iterator yielding frames sequentially decoded from payload.
// Zero heap allocations.
func Frames(payload []byte) iter.Seq2[*Frame, error] {
	return func(yield func(*Frame, error) bool) {
		var f Frame
		for len(payload) > 0 {
			n, err := ParseNextFrame(payload, &f)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(&f, nil) {
				return
			}

			payload = payload[n:]
		}
	}
}
