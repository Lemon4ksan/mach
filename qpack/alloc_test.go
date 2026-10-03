package qpack_test

import (
	"testing"

	"github.com/lemon4ksan/mach/qpack"
)

type mockDelegate struct{}

func (mockDelegate) OnInstructionDecoded(inst *qpack.Instruction) bool                             { return true }
func (mockDelegate) OnInstructionDecodingError(code qpack.InstructionDecoderErrorCode, msg string) {}

func TestAlloc(t *testing.T) {
	dec := qpack.NewInstructionDecoder(qpack.PrefixLanguage(), mockDelegate{})
	data := []byte{0x00}
	allocs := testing.AllocsPerRun(1000, func() {
		dec.Decode(data)
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
