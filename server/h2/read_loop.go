package h2

import (
	"errors"
	"io"
	"net"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

func (sc *ServerConn) readLoop() error {
	var buffered int

	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		if buffered == len(sc.readBuf) {
			// Grow buffer if we have an incomplete frame larger than 32KB
			newBuf := make([]byte, len(sc.readBuf)*2)
			copy(newBuf, sc.readBuf[:buffered])
			sc.readBuf = newBuf
		}

		n, err := sc.conn.Read(sc.readBuf[buffered:])
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return err
		}

		buffered += n

		consumed, err := sc.sm.Feed(sc.readBuf[:buffered])
		if err != nil {
			return err
		}

		if consumed > 0 {
			buffered -= consumed
			if buffered > 0 {
				copy(sc.readBuf, sc.readBuf[consumed:consumed+buffered])
			}
		}
	}
}

// EventHandler implementation

func (sc *ServerConn) OnStreamData(streamID uint32, payload []byte, endStream bool) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	// RFC 9113 §5.1: Receiving frames on idle stream is connection error of type PROTOCOL_ERROR
	if streamID > sc.maxStreamID {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	st, ok := sc.getStream(streamID)
	if !ok {
		// RFC 9113 §5.4.2 & §5.2.2: Even if stream is closed, DATA frames consume connection window
		// and MUST be replenished to prevent connection window leaks!
		if len(payload) > 0 && !sc.Closed() {
			_ = sc.sendWindowUpdateFrame(0, uint32(len(payload)))
		}

		_ = sc.writeRstStream(streamID, coreh2.StreamClosedError)

		return
	}

	// RFC 9113 §5.1: If stream is in half-closed (remote) state, reject with STREAM_CLOSED
	if st.state == streamHalfClosedRemote || st.state == streamClosed {
		if len(payload) > 0 && !sc.Closed() {
			_ = sc.sendWindowUpdateFrame(0, uint32(len(payload)))
		}

		_ = sc.writeRstStream(streamID, coreh2.StreamClosedError)

		return
	}

	if st.isTunnel {
		if len(payload) > 0 {
			if st.pipeWriter != nil {
				_, _ = st.pipeWriter.Write(payload)
			}

			sc.replenishReceiveWindow(st, len(payload))
		}

		if endStream {
			st.endStream = true
			if st.pipeWriter != nil {
				_ = st.pipeWriter.Close()
			}
		}

		return
	}

	st.body.Write(payload)

	if len(payload) > 0 {
		sc.replenishReceiveWindow(st, len(payload))
	}

	if endStream {
		st.endStream = true
		sc.startStream(st)
	}
}

func (sc *ServerConn) OnHeaders(streamID uint32, payload []byte, endHeaders, endStream bool) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	// RFC 9113 §5.1.1: Client-initiated streams MUST use non-zero, odd-numbered stream identifiers
	if streamID == 0 || (streamID%2) == 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	// RFC 9113 §5.1.1: Stream identifiers MUST be strictly monotonically increasing
	if streamID <= sc.maxStreamID {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	sc.maxStreamID = streamID

	if !endHeaders {
		sc.continuationStream = streamID
	}

	st := sc.newServerStream(streamID, endHeaders, endStream)
	st.headerBlock.Write(payload)

	sc.putStream(streamID, st)

	if endHeaders {
		if err := sc.finishHeaderBlock(st); err != nil {
			// RFC 9113 §8.1.1: Malformed requests MUST be treated as a stream error of type PROTOCOL_ERROR.
			if errors.Is(err, coreh2.ProtocolError) {
				_ = sc.writeRstStream(streamID, coreh2.ProtocolError)
				sc.deleteStream(streamID)

				return
			}

			var ec coreh2.ErrorCode
			if errors.As(err, &ec) {
				sc.closeWithGoAway(sc.maxStreamID, ec)
			} else {
				sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
			}
		}
	}
}

func (sc *ServerConn) OnContinuation(streamID uint32, payload []byte, endHeaders bool) {
	// RFC 9113 §6.10: CONTINUATION MUST follow a HEADERS frame without END_HEADERS on the same stream
	if sc.continuationStream == 0 || sc.continuationStream != streamID {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	st, ok := sc.getStream(streamID)
	if !ok {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	// CONTINUATION flood protection (CVE-2024-27983): bound total header block size
	if st.headerBlock.Len()+len(payload) > 65536 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.EnhanceYourCalm)
		return
	}

	st.headerBlock.Write(payload)

	if endHeaders {
		sc.continuationStream = 0

		st.endHeaders = true
		if err := sc.finishHeaderBlock(st); err != nil {
			// RFC 9113 §8.1.1: Malformed requests MUST be treated as a stream error of type PROTOCOL_ERROR.
			if errors.Is(err, coreh2.ProtocolError) {
				_ = sc.writeRstStream(streamID, coreh2.ProtocolError)
				sc.deleteStream(streamID)

				return
			}

			var ec coreh2.ErrorCode
			if errors.As(err, &ec) {
				sc.closeWithGoAway(sc.maxStreamID, ec)
			} else {
				sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
			}
		}
	}
}

func (sc *ServerConn) OnSettings(payload []byte, ack bool) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if ack {
		// RFC 9113 §6.5: Receipt of a SETTINGS frame with the ACK flag set and a length
		// field value other than 0 MUST be treated as a connection error of type FRAME_SIZE_ERROR.
		if len(payload) > 0 {
			sc.closeWithGoAway(sc.maxStreamID, coreh2.FrameSizeError)
		}

		return
	}

	// RFC 9113 §6.5: A SETTINGS frame with a length other than a multiple of 6 octets
	// MUST be treated as a connection error of type FRAME_SIZE_ERROR.
	if len(payload)%6 != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.FrameSizeError)
		return
	}

	for i := 0; i+6 <= len(payload); i += 6 {
		id := uint16(payload[i])<<8 | uint16(payload[i+1])
		val := uint32(payload[i+2])<<24 | uint32(payload[i+3])<<16 | uint32(payload[i+4])<<8 | uint32(payload[i+5])

		switch id {
		case 1: // HEADER_TABLE_SIZE
			sc.hpackDec.SetMaxTableSize(val)

		case 2: // ENABLE_PUSH
			if val != 0 && val != 1 {
				// RFC 9113 §6.5.2: Any value other than 0 or 1 MUST be treated as PROTOCOL_ERROR
				sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
				return
			}

		case 4: // INITIAL_WINDOW_SIZE
			if val > 0x7fffffff {
				// RFC 9113 §6.5.2: Values above 2^31-1 MUST be treated as FLOW_CONTROL_ERROR
				sc.closeWithGoAway(sc.maxStreamID, coreh2.FlowControlError)
				return
			}

			newWin := int32(val)
			delta := newWin - sc.peerInitialWin
			sc.peerInitialWin = newWin

			// RFC 9113 §6.9.2: Adjust flow-control window of all active streams by delta
			for i := range streamShardsCount {
				shard := &sc.streamShards[i]
				shard.mu.Lock()
				for _, st := range shard.streams {
					old := st.sendWindow.Load()
					if int64(old)+int64(delta) > int64(1<<31-1) {
						shard.mu.Unlock()
						sc.closeWithGoAway(sc.maxStreamID, coreh2.FlowControlError)
						return
					}

					st.sendWindow.Store(old + delta)
				}

				shard.mu.Unlock()
			}

		case 5: // MAX_FRAME_SIZE
			if val < 16384 || val > 16777215 {
				// RFC 9113 §6.5.2: Values outside 2^14 to 2^24-1 MUST be treated as PROTOCOL_ERROR
				sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
				return
			}

			sc.peerMaxFrameSize = val
		}
	}

	if err := sc.sendSettings(nil, true); err != nil {
		sc.closeErr = err
		sc.isClosed.Store(true)
	}
}

func (sc *ServerConn) OnPing(payload []byte, ack bool) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if ack {
		return
	}

	if err := sc.writePingAck(payload); err != nil {
		sc.closeErr = err
		sc.isClosed.Store(true)
	}
}

func (sc *ServerConn) OnWindowUpdate(streamID, increment uint32) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if increment == 0 || increment > 0x7fffffff {
		if streamID == 0 {
			sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		} else {
			_ = sc.writeRstStream(streamID, coreh2.ProtocolError)
		}

		return
	}

	if streamID > sc.maxStreamID {
		// RFC 9113 §5.1: WINDOW_UPDATE on idle stream is connection error PROTOCOL_ERROR
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if streamID == 0 {
		if err := sc.updateConnSendWindow(int32(increment)); err != nil {
			sc.closeWithGoAway(sc.maxStreamID, coreh2.FlowControlError)
		}

		return
	}

	st, ok := sc.getStream(streamID)
	if ok {
		if err := sc.updateStreamSendWindow(st, int32(increment)); err != nil {
			// RFC 9113 §6.9.1: Stream flow control overflow is a STREAM ERROR of type FLOW_CONTROL_ERROR
			_ = sc.writeRstStream(streamID, coreh2.FlowControlError)
			if st, ok := sc.deleteStream(streamID); ok {
				if st.cancel != nil {
					st.cancel()
				}
			}
		}
	}
}

func (sc *ServerConn) OnPriority(streamID uint32, payload []byte) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}
}

func (sc *ServerConn) OnUnknownFrame(streamID uint32, frameType, flags uint8, payload []byte) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	// RFC 9113 §6.6: A client MUST NOT send a PUSH_PROMISE frame
	if frameType == 0x05 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}
}

func (sc *ServerConn) OnStreamReset(streamID, errorCode uint32) {
	if sc.continuationStream != 0 {
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if streamID == 0 || streamID > sc.maxStreamID {
		// RFC 9113 §5.1: RST_STREAM on idle stream is connection error PROTOCOL_ERROR
		sc.closeWithGoAway(sc.maxStreamID, coreh2.ProtocolError)
		return
	}

	if st, ok := sc.deleteStream(streamID); ok {
		if st.cancel != nil {
			st.cancel()
		}

		if st.pipeWriter != nil {
			_ = st.pipeWriter.CloseWithError(coreh2.ErrStreamClosed)
		}
	}
}

func (sc *ServerConn) OnGoAway(lastStreamID, errorCode uint32) {
	sc.closeErr = nil
	sc.isClosed.Store(true)
}

func (sc *ServerConn) OnWrite(payload []byte) {
	// Not used
}

func (sc *ServerConn) sendSettings(st *coreh2.Settings, ack bool) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	stFrame := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	if ack {
		stFrame.SetAck(true)
		fr.SetFlags(coreh2.FlagAck)
	}

	if !ack {
		st.CopyTo(stFrame)
	}

	fr.SetBody(stFrame)

	if _, err := fr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}
