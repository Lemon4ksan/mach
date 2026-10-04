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
	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return
	}

	st.body.Write(payload)

	if len(payload) > 0 {
		sc.replenishReceiveWindow(streamID, len(payload))
	}

	if endStream {
		st.endStream = true
		sc.startStream(st)
	}
}

func (sc *ServerConn) OnHeaders(streamID uint32, payload []byte, endHeaders, endStream bool) {
	// RFC 9113 §5.1.1: Client-initiated streams MUST use non-zero, odd-numbered stream identifiers
	if streamID == 0 || (streamID%2) == 0 {
		sc.closeErr = coreh2.ProtocolError
		sc.isClosed.Store(true)
		return
	}

	st := sc.newServerStream(streamID, endHeaders, endStream)
	st.headerBlock.Write(payload)

	sc.streamsMu.Lock()
	sc.streams[streamID] = st
	sc.streamsMu.Unlock()

	if endHeaders {
		if err := sc.finishHeaderBlock(st); err != nil {
			sc.closeErr = err
			sc.isClosed.Store(true)
		}
	}
}

func (sc *ServerConn) OnContinuation(streamID uint32, payload []byte, endHeaders bool) {
	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		sc.closeErr = errors.New("h2: CONTINUATION on unknown stream")
		sc.isClosed.Store(true)
		return
	}

	st.headerBlock.Write(payload)

	if endHeaders {
		st.endHeaders = true
		if err := sc.finishHeaderBlock(st); err != nil {
			sc.closeErr = err
			sc.isClosed.Store(true)
		}
	}
}

func (sc *ServerConn) OnSettings(payload []byte, ack bool) {
	if ack {
		return
	}

	for i := 0; i+6 <= len(payload); i += 6 {
		id := uint16(payload[i])<<8 | uint16(payload[i+1])
		val := uint32(payload[i+2])<<24 | uint32(payload[i+3])<<16 | uint32(payload[i+4])<<8 | uint32(payload[i+5])

		switch id {
		case 5: // MAX_FRAME_SIZE
			if val >= 16384 && val <= 16777215 {
				sc.peerMaxFrameSize = val
			}

		case 4: // INITIAL_WINDOW_SIZE
			if val > 0 && val <= 0x7fffffff {
				sc.peerInitialWin = int32(val)
			}
		}
	}

	if err := sc.sendSettings(nil, true); err != nil {
		sc.closeErr = err
		sc.isClosed.Store(true)
	}
}

func (sc *ServerConn) OnPing(payload []byte, ack bool) {
	if ack {
		return
	}

	if err := sc.writePingAck(payload); err != nil {
		sc.closeErr = err
		sc.isClosed.Store(true)
	}
}

func (sc *ServerConn) OnWindowUpdate(streamID, increment uint32) {
	if increment == 0 || increment > 0x7fffffff {
		sc.closeErr = coreh2.ProtocolError
		sc.isClosed.Store(true)
		return
	}

	if streamID == 0 {
		if err := sc.updateConnSendWindow(int32(increment)); err != nil {
			sc.closeErr = err
			sc.isClosed.Store(true)
		}

		return
	}

	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if ok {
		if err := sc.updateStreamSendWindow(st, int32(increment)); err != nil {
			sc.closeErr = err
			sc.isClosed.Store(true)
		}
	}
}

func (sc *ServerConn) OnStreamReset(streamID, errorCode uint32) {
	sc.streamsMu.Lock()
	if st, ok := sc.streams[streamID]; ok {
		if st.cancel != nil {
			st.cancel()
		}

		delete(sc.streams, streamID)
	}

	sc.streamsMu.Unlock()
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
	} else {
		st.CopyTo(stFrame)
	}

	fr.SetBody(stFrame)

	if _, err := fr.WriteTo(sc.bw); err != nil {
		return err
	}

	return sc.bw.Flush()
}
