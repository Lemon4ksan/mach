package quic

import (
	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// PathScheduler selects which path to use for outgoing packets.
type PathScheduler interface {
	// SelectPath decides which path should be used for the next packet.
	// It can return nil if no paths are available.
	SelectPath(paths map[protocol.PathID]struct{}) protocol.PathID // simplified for now, as `path` is internal
}

// RoundRobinScheduler is a simple scheduler that cycles through available paths.
type RoundRobinScheduler struct {
	lastPath protocol.PathID
}

func (s *RoundRobinScheduler) SelectPath(paths map[protocol.PathID]struct{}) protocol.PathID {
	if len(paths) == 0 {
		return protocol.InitialPathID
	}

	var first protocol.PathID
	foundFirst := false
	var next protocol.PathID
	foundNext := false

	for id := range paths {
		if !foundFirst {
			first = id
			foundFirst = true
		}
		if id > s.lastPath && (!foundNext || id < next) {
			next = id
			foundNext = true
		}
	}

	if foundNext {
		s.lastPath = next
		return next
	}
	s.lastPath = first
	return first
}
