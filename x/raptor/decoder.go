package raptor

import (
	"context"

	"github.com/lemon4ksan/foundation/silicon/simd/gfni"
)

// Datagram represents an incoming packet (either data or repair symbol).
type Datagram struct {
	SymbolID uint32
	Data     []byte
	IsRepair bool
}

// Decoder handles the decoding of RaptorQ/LT fountain codes.
type Decoder struct {
	datagramChan chan Datagram
	recovered    chan []byte
}

// NewDecoder creates a new instance of Decoder.
func NewDecoder() *Decoder {
	return &Decoder{
		datagramChan: make(chan Datagram, 128),
		recovered:    make(chan []byte, 128),
	}
}

// ProcessAsync asynchronously processes incoming datagrams to recover lost packets.
func (d *Decoder) ProcessAsync(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case dg := <-d.datagramChan:
				if dg.IsRepair {
					temp := make([]byte, len(dg.Data))
					// Stub logic: multiply by 2 using gfni
					gfni.MultiplyGF2P8Vector(temp, dg.Data, 2)

					// Pass the "recovered" packet to the channel (non-blocking)
					select {
					case d.recovered <- temp:
					default:
					}
				}
			}
		}
	}()
}

// Feed feeds an incoming datagram to the decoder.
func (d *Decoder) Feed(dg Datagram) {
	select {
	case d.datagramChan <- dg:
	default:
	}
}

// Recovered returns a channel from which recovered packets can be read.
func (d *Decoder) Recovered() <-chan []byte {
	return d.recovered
}
