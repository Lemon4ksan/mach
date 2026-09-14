package wire

import (
	"io"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/mach/quic/quicvarint"
)

// A PathAbandonFrame is a PATH_ABANDON frame
type PathAbandonFrame struct {
	PathID    uint64
	ErrorCode uint64
}

func parsePathAbandonFrame(b []byte, _ protocol.Version) (*PathAbandonFrame, int, error) {
	f := &PathAbandonFrame{}
	var parsed int

	pathID, l, err := quicvarint.Parse(b)
	if err != nil {
		return nil, parsed, err
	}
	f.PathID = pathID
	b = b[l:]
	parsed += l

	errorCode, l, err := quicvarint.Parse(b)
	if err != nil {
		return nil, parsed, err
	}
	f.ErrorCode = errorCode
	b = b[l:]
	parsed += l

	reasonLen, l, err := quicvarint.Parse(b)
	if err != nil {
		return nil, parsed, err
	}
	b = b[l:]
	parsed += l

	if uint64(len(b)) < reasonLen {
		return nil, parsed, io.EOF
	}
	// We ignore the Reason Phrase as per instructions
	parsed += int(reasonLen)

	return f, parsed, nil
}

func (f *PathAbandonFrame) Append(b []byte, _ protocol.Version) ([]byte, error) {
	b = quicvarint.Append(b, uint64(FrameTypePathAbandon))
	b = quicvarint.Append(b, f.PathID)
	b = quicvarint.Append(b, f.ErrorCode)
	b = quicvarint.Append(b, 0)
	return b, nil
}

// Length of a written frame
func (f *PathAbandonFrame) Length(_ protocol.Version) protocol.ByteCount {
	return protocol.ByteCount(quicvarint.Len(uint64(FrameTypePathAbandon)) +
		quicvarint.Len(f.PathID) +
		quicvarint.Len(f.ErrorCode) +
		quicvarint.Len(0))
}
