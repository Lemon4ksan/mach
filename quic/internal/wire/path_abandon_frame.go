package wire

import (
	"io"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/foundation/encoding/varint"
)

// A PathAbandonFrame is a PATH_ABANDON frame
type PathAbandonFrame struct {
	PathID    uint64
	ErrorCode uint64
}

func parsePathAbandonFrame(b []byte, _ protocol.Version) (*PathAbandonFrame, int, error) {
	f := &PathAbandonFrame{}
	var parsed int

	pathID, l, err := varint.Parse(b)
	if err != nil {
		return nil, parsed, err
	}
	f.PathID = pathID
	b = b[l:]
	parsed += l

	errorCode, l, err := varint.Parse(b)
	if err != nil {
		return nil, parsed, err
	}
	f.ErrorCode = errorCode
	b = b[l:]
	parsed += l

	reasonLen, l, err := varint.Parse(b)
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
	b = varint.Append(b, uint64(FrameTypePathAbandon))
	b = varint.Append(b, f.PathID)
	b = varint.Append(b, f.ErrorCode)
	b = varint.Append(b, 0)
	return b, nil
}

// Length of a written frame
func (f *PathAbandonFrame) Length(_ protocol.Version) protocol.ByteCount {
	return protocol.ByteCount(varint.Len(uint64(FrameTypePathAbandon)) +
		varint.Len(f.PathID) +
		varint.Len(f.ErrorCode) +
		varint.Len(0))
}
