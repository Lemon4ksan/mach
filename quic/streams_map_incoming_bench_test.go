package quic

import (
	"testing"
	"sync/atomic"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
	"github.com/lemon4ksan/mach/quic/internal/wire"
)

type benchIncomingStream struct{}
func (b *benchIncomingStream) closeForShutdown(error) {}

func BenchmarkIncomingStreamsMap_Get(b *testing.B) {
	m := newIncomingStreamsMap[*benchIncomingStream](
		protocol.StreamTypeBidi,
		func(id protocol.StreamID) *benchIncomingStream {
			return &benchIncomingStream{}
		},
		100000,
		func(f wire.Frame) {},
		protocol.PerspectiveServer,
	)

	// Pre-open 10,000 streams
	for i := 0; i < 10000; i++ {
		id := protocol.StreamID(i*4 + 4) // Bidi server streams start at 4
		m.GetOrOpenStream(id)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		var i uint64
		for pb.Next() {
			// Read randomly from pre-opened streams to simulate hot path
			idx := atomic.AddUint64(&i, 1) % 10000
			id := protocol.StreamID(idx*4 + 4)
			_, _ = m.GetOrOpenStream(id)
		}
	})
}
