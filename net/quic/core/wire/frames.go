// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import "fmt"

// FrameType represents a QUIC frame type identifier (RFC 9000 §19).
type FrameType uint64

const (
	FrameTypePadding                 FrameType = 0x00
	FrameTypePing                    FrameType = 0x01
	FrameTypeAck                     FrameType = 0x02
	FrameTypeAckECN                  FrameType = 0x03
	FrameTypeResetStream             FrameType = 0x04
	FrameTypeStopSending             FrameType = 0x05
	FrameTypeCrypto                  FrameType = 0x06
	FrameTypeNewToken                FrameType = 0x07
	FrameTypeStream                  FrameType = 0x08 // 0x08 - 0x0f
	FrameTypeMaxData                 FrameType = 0x10
	FrameTypeMaxStreamData           FrameType = 0x11
	FrameTypeMaxStreamsBidi          FrameType = 0x12
	FrameTypeMaxStreamsUni           FrameType = 0x13
	FrameTypeDataBlocked             FrameType = 0x14
	FrameTypeStreamDataBlocked       FrameType = 0x15
	FrameTypeStreamsBlockedBidi      FrameType = 0x16
	FrameTypeStreamsBlockedUni       FrameType = 0x17
	FrameTypeNewConnectionID         FrameType = 0x18
	FrameTypeRetireConnectionID      FrameType = 0x19
	FrameTypePathChallenge           FrameType = 0x1a
	FrameTypePathResponse            FrameType = 0x1b
	FrameTypeConnectionCloseTransport FrameType = 0x1c
	FrameTypeConnectionCloseApp       FrameType = 0x1d
	FrameTypeHandshakeDone           FrameType = 0x1e
	FrameTypeDatagram                FrameType = 0x30
	FrameTypeDatagramWithLen         FrameType = 0x31
)

func (t FrameType) String() string {
	switch {
	case t == FrameTypePadding:
		return "PADDING"
	case t == FrameTypePing:
		return "PING"
	case t == FrameTypeAck:
		return "ACK"
	case t == FrameTypeAckECN:
		return "ACK_ECN"
	case t == FrameTypeResetStream:
		return "RESET_STREAM"
	case t == FrameTypeStopSending:
		return "STOP_SENDING"
	case t == FrameTypeCrypto:
		return "CRYPTO"
	case t == FrameTypeNewToken:
		return "NEW_TOKEN"
	case t >= 0x08 && t <= 0x0f:
		return fmt.Sprintf("STREAM (0x%02x)", uint64(t))
	case t == FrameTypeMaxData:
		return "MAX_DATA"
	case t == FrameTypeMaxStreamData:
		return "MAX_STREAM_DATA"
	case t == FrameTypeMaxStreamsBidi:
		return "MAX_STREAMS (bidi)"
	case t == FrameTypeMaxStreamsUni:
		return "MAX_STREAMS (uni)"
	case t == FrameTypeDataBlocked:
		return "DATA_BLOCKED"
	case t == FrameTypeStreamDataBlocked:
		return "STREAM_DATA_BLOCKED"
	case t == FrameTypeStreamsBlockedBidi:
		return "STREAMS_BLOCKED (bidi)"
	case t == FrameTypeStreamsBlockedUni:
		return "STREAMS_BLOCKED (uni)"
	case t == FrameTypeNewConnectionID:
		return "NEW_CONNECTION_ID"
	case t == FrameTypeRetireConnectionID:
		return "RETIRE_CONNECTION_ID"
	case t == FrameTypePathChallenge:
		return "PATH_CHALLENGE"
	case t == FrameTypePathResponse:
		return "PATH_RESPONSE"
	case t == FrameTypeConnectionCloseTransport:
		return "CONNECTION_CLOSE (transport)"
	case t == FrameTypeConnectionCloseApp:
		return "CONNECTION_CLOSE (app)"
	case t == FrameTypeHandshakeDone:
		return "HANDSHAKE_DONE"
	case t == FrameTypeDatagram:
		return "DATAGRAM"
	case t == FrameTypeDatagramWithLen:
		return "DATAGRAM_WITH_LEN"
	default:
		return fmt.Sprintf("FrameType(0x%x)", uint64(t))
	}
}

// AckRange represents a gap and following run of acknowledged packets (RFC 9000 §19.3.1).
type AckRange struct {
	Gap    uint64
	Length uint64
}

// ECNCounts contains ECN-CE, ECT(0), and ECT(1) packet counters (RFC 9000 §19.3.2).
type ECNCounts struct {
	ECT0  uint64
	ECT1  uint64
	ECNCE uint64
}

// AckFrame represents an ACK frame (RFC 9000 §19.3).
// To prevent heap allocations, up to 8 ranges are stored inline in InlineRanges.
type AckFrame struct {
	LargestAcked    uint64
	AckDelay        uint64
	FirstAckRange   uint64
	InlineRanges    [8]AckRange
	NumInlineRanges int
	ExtraRanges     []AckRange
	HasECN          bool
	ECN             ECNCounts
}

// RangeCount returns the total number of additional ACK ranges.
func (a *AckFrame) RangeCount() int {
	return a.NumInlineRanges + len(a.ExtraRanges)
}

// Range returns the ACK range at index i.
func (a *AckFrame) Range(i int) AckRange {
	if i < a.NumInlineRanges {
		return a.InlineRanges[i]
	}
	return a.ExtraRanges[i-a.NumInlineRanges]
}

// AppendRange appends an ACK range with zero heap allocation if under 8 inline ranges.
func (a *AckFrame) AppendRange(gap, length uint64) {
	if a.NumInlineRanges < len(a.InlineRanges) {
		a.InlineRanges[a.NumInlineRanges] = AckRange{Gap: gap, Length: length}
		a.NumInlineRanges++
		return
	}
	a.ExtraRanges = append(a.ExtraRanges, AckRange{Gap: gap, Length: length})
}

// Reset clears the ACK frame for reuse.
func (a *AckFrame) Reset() {
	a.LargestAcked = 0
	a.AckDelay = 0
	a.FirstAckRange = 0
	a.NumInlineRanges = 0
	a.ExtraRanges = a.ExtraRanges[:0]
	a.HasECN = false
	a.ECN = ECNCounts{}
}

// ResetStreamFrame represents a RESET_STREAM frame (RFC 9000 §19.4).
type ResetStreamFrame struct {
	StreamID  uint64
	ErrorCode uint64
	FinalSize uint64
}

// StopSendingFrame represents a STOP_SENDING frame (RFC 9000 §19.5).
type StopSendingFrame struct {
	StreamID  uint64
	ErrorCode uint64
}

// CryptoFrame represents a CRYPTO frame (RFC 9000 §19.6).
type CryptoFrame struct {
	Offset uint64
	Data   []byte
}

// NewTokenFrame represents a NEW_TOKEN frame (RFC 9000 §19.7).
type NewTokenFrame struct {
	Token []byte
}

// StreamFrame represents a STREAM frame (RFC 9000 §19.8).
type StreamFrame struct {
	StreamID uint64
	Offset   uint64
	Fin      bool
	HasLen   bool
	Data     []byte
}

// MaxDataFrame represents a MAX_DATA frame (RFC 9000 §19.9).
type MaxDataFrame struct {
	MaximumData uint64
}

// MaxStreamDataFrame represents a MAX_STREAM_DATA frame (RFC 9000 §19.10).
type MaxStreamDataFrame struct {
	StreamID          uint64
	MaximumStreamData uint64
}

// MaxStreamsFrame represents a MAX_STREAMS frame (RFC 9000 §19.11).
type MaxStreamsFrame struct {
	MaxStreams    uint64
	Unidirectional bool
}

// DataBlockedFrame represents a DATA_BLOCKED frame (RFC 9000 §19.12).
type DataBlockedFrame struct {
	MaximumData uint64
}

// StreamDataBlockedFrame represents a STREAM_DATA_BLOCKED frame (RFC 9000 §19.13).
type StreamDataBlockedFrame struct {
	StreamID          uint64
	MaximumStreamData uint64
}

// StreamsBlockedFrame represents a STREAMS_BLOCKED frame (RFC 9000 §19.14).
type StreamsBlockedFrame struct {
	StreamLimit    uint64
	Unidirectional bool
}

// NewConnectionIDFrame represents a NEW_CONNECTION_ID frame (RFC 9000 §19.15).
type NewConnectionIDFrame struct {
	SequenceNumber      uint64
	RetirePriorTo       uint64
	ConnID              []byte
	StatelessResetToken [16]byte
}

// RetireConnectionIDFrame represents a RETIRE_CONNECTION_ID frame (RFC 9000 §19.16).
type RetireConnectionIDFrame struct {
	SequenceNumber uint64
}

// PathChallengeFrame represents a PATH_CHALLENGE frame (RFC 9000 §19.17).
type PathChallengeFrame struct {
	Data [8]byte
}

// PathResponseFrame represents a PATH_RESPONSE frame (RFC 9000 §19.18).
type PathResponseFrame struct {
	Data [8]byte
}

// ConnectionCloseFrame represents a CONNECTION_CLOSE frame (RFC 9000 §19.19).
type ConnectionCloseFrame struct {
	IsApplication bool
	ErrorCode     uint64
	FrameType     uint64 // Triggering frame type (transport close only)
	Reason        []byte
}

// DatagramFrame represents an RFC 9221 DATAGRAM frame.
type DatagramFrame struct {
	HasLen bool
	Data   []byte
}

// Frame represents any QUIC frame decoded without heap allocation.
type Frame struct {
	Type FrameType

	PaddingCount       int
	Ack                AckFrame
	ResetStream        ResetStreamFrame
	StopSending        StopSendingFrame
	Crypto             CryptoFrame
	NewToken           NewTokenFrame
	Stream             StreamFrame
	MaxData            MaxDataFrame
	MaxStreamData      MaxStreamDataFrame
	MaxStreams         MaxStreamsFrame
	DataBlocked        DataBlockedFrame
	StreamDataBlocked  StreamDataBlockedFrame
	StreamsBlocked     StreamsBlockedFrame
	NewConnectionID    NewConnectionIDFrame
	RetireConnectionID RetireConnectionIDFrame
	PathChallenge      PathChallengeFrame
	PathResponse       PathResponseFrame
	ConnectionClose    ConnectionCloseFrame
	Datagram           DatagramFrame
}

// Reset clears the frame for reuse.
func (f *Frame) Reset() {
	f.Type = 0
	f.PaddingCount = 0
	f.Ack.Reset()
	f.Crypto.Data = nil
	f.NewToken.Token = nil
	f.Stream.Data = nil
	f.NewConnectionID.ConnID = nil
	f.ConnectionClose.Reason = nil
	f.Datagram.Data = nil
}
