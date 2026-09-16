package wire

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

func TestStreamFrameOverlay_ParsesCorrectly(t *testing.T) {
	classicFrame := &StreamFrame{
		StreamID:       protocol.StreamID(0x1337),
		Offset:         protocol.ByteCount(0x4242),
		Data:           []byte("hello zero-copy world!"),
		DataLenPresent: true,
		Fin:            true,
	}

	buf, err := classicFrame.Append(nil, protocol.Version1)
	if err != nil {
		t.Fatalf("Failed to append classic frame: %v", err)
	}

	typ := FrameType(buf[0])
	overlay, consumed, err := ParseStreamFrameOverlay(buf[1:], typ)
	if err != nil {
		t.Fatalf("Failed to parse overlay: %v", err)
	}

	if consumed+1 != len(buf) {
		t.Errorf("Consumed %d bytes, expected %d", consumed+1, len(buf))
	}

	if overlay.StreamID() != classicFrame.StreamID {
		t.Errorf("StreamID mismatch: got %v, want %v", overlay.StreamID(), classicFrame.StreamID)
	}

	if overlay.Offset() != classicFrame.Offset {
		t.Errorf("Offset mismatch: got %v, want %v", overlay.Offset(), classicFrame.Offset)
	}

	if overlay.Fin() != classicFrame.Fin {
		t.Errorf("Fin mismatch: got %v, want %v", overlay.Fin(), classicFrame.Fin)
	}

	if !bytes.Equal(overlay.Data(), classicFrame.Data) {
		t.Errorf("Data mismatch: got %q, want %q", overlay.Data(), classicFrame.Data)
	}
}
