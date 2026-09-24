// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// ============================================================================
// Milestone 2 Adversarial Oracle Helpers
// ============================================================================

type oracleSliceMode int

const (
	oracleSliceSingleChunk oracleSliceMode = iota
	oracleSliceOctetByOctet
	oracleSliceTwoBytes
	oracleSliceThreeBytes
	oracleSliceRandomStep
)

func (m oracleSliceMode) String() string {
	switch m {
	case oracleSliceSingleChunk:
		return "SingleChunk"
	case oracleSliceOctetByOctet:
		return "OctetByOctet"
	case oracleSliceTwoBytes:
		return "TwoBytes"
	case oracleSliceThreeBytes:
		return "ThreeBytes"
	case oracleSliceRandomStep:
		return "RandomStep"
	default:
		return fmt.Sprintf("SliceMode(%d)", int(m))
	}
}

// oracleSnapshot captures instruction state at the exact moment OnInstructionDecoded is invoked.
type oracleSnapshot struct {
	instruction *Instruction
	sBit        bool
	varint      uint64
	varint2     uint64
	name        string
	value       string
}

// oracleDecoderDelegate captures all callbacks and snapshots decoder fields.
type oracleDecoderDelegate struct {
	decoder     *InstructionDecoder
	snapshots   []oracleSnapshot
	errorCodes  []InstructionDecoderErrorCode
	errorMsgs   []string
	allowDecode bool
}

func newOracleDecoderDelegate() *oracleDecoderDelegate {
	return &oracleDecoderDelegate{
		allowDecode: true,
	}
}

func (d *oracleDecoderDelegate) SetDecoder(dec *InstructionDecoder) {
	d.decoder = dec
}

func (d *oracleDecoderDelegate) OnInstructionDecoded(instruction *Instruction) bool {
	if d.decoder != nil {
		d.snapshots = append(d.snapshots, oracleSnapshot{
			instruction: instruction,
			sBit:        d.decoder.SBit(),
			varint:      d.decoder.Varint(),
			varint2:     d.decoder.Varint2(),
			name:        d.decoder.Name(),
			value:       d.decoder.Value(),
		})
	}
	return d.allowDecode
}

func (d *oracleDecoderDelegate) OnInstructionDecodingError(errorCode InstructionDecoderErrorCode, errorMessage string) {
	d.errorCodes = append(d.errorCodes, errorCode)
	d.errorMsgs = append(d.errorMsgs, errorMessage)
}

// feedDecoderSlices feeds encoded bytes into decoder using the specified slicing strategy.
func feedDecoderSlices(decoder *InstructionDecoder, data []byte, mode oracleSliceMode, rng *rand.Rand) bool {
	if len(data) == 0 {
		return decoder.Decode(nil)
	}

	switch mode {
	case oracleSliceSingleChunk:
		return decoder.Decode(data)

	case oracleSliceOctetByOctet:
		for i := 0; i < len(data); i++ {
			if !decoder.Decode(data[i : i+1]) {
				return false
			}
		}
		return true

	case oracleSliceTwoBytes:
		for offset := 0; offset < len(data); {
			chunkSize := 2
			if offset+chunkSize > len(data) {
				chunkSize = len(data) - offset
			}
			if !decoder.Decode(data[offset : offset+chunkSize]) {
				return false
			}
			offset += chunkSize
		}
		return true

	case oracleSliceThreeBytes:
		for offset := 0; offset < len(data); {
			chunkSize := 3
			if offset+chunkSize > len(data) {
				chunkSize = len(data) - offset
			}
			if !decoder.Decode(data[offset : offset+chunkSize]) {
				return false
			}
			offset += chunkSize
		}
		return true

	case oracleSliceRandomStep:
		for offset := 0; offset < len(data); {
			step := 1 + rng.Intn(5)
			if offset+step > len(data) {
				step = len(data) - offset
			}
			if !decoder.Decode(data[offset : offset+step]) {
				return false
			}
			offset += step
		}
		return true

	default:
		panic(fmt.Sprintf("unknown slice mode: %d", mode))
	}
}

// ============================================================================
// Test 1: Opcode Table Coverage & Disjointness Oracle
// ============================================================================

func TestM2Oracle_OpcodeTableCoverage(t *testing.T) {
	// Verify that in all 4 languages, every one of the 256 possible byte values
	// maps to exactly one instruction, and that the matched instruction corresponds
	// strictly to the specification partition.

	t.Run("EncoderStreamLanguage", func(t *testing.T) {
		lang := EncoderStreamLanguage()
		require.NotNil(t, lang)
		require.Equal(t, 4, len(*lang))

		counts := make(map[*Instruction]int)
		for b := 0; b <= 255; b++ {
			byteVal := byte(b)
			var matched *Instruction
			matchCount := 0

			for _, inst := range *lang {
				if (byteVal & inst.Opcode.Mask) == inst.Opcode.Value {
					matchCount++
					matched = inst
				}
			}

			require.Equal(
				t,
				1,
				matchCount,
				fmt.Sprintf("EncoderStream byte 0x%02x matched %d instructions", byteVal, matchCount),
			)
			counts[matched]++

			// Check RFC 9204 opcode assignment
			switch {
			case byteVal >= 0x80:
				assert.Equal(t, InsertWithNameReferenceInstruction(), matched)
			case byteVal >= 0x40:
				assert.Equal(t, InsertWithoutNameReferenceInstruction(), matched)
			case byteVal >= 0x20:
				assert.Equal(t, SetDynamicTableCapacityInstruction(), matched)
			default:
				assert.Equal(t, DuplicateInstruction(), matched)
			}
		}

		assert.Equal(t, 128, counts[InsertWithNameReferenceInstruction()])
		assert.Equal(t, 64, counts[InsertWithoutNameReferenceInstruction()])
		assert.Equal(t, 32, counts[SetDynamicTableCapacityInstruction()])
		assert.Equal(t, 32, counts[DuplicateInstruction()])
	})

	t.Run("DecoderStreamLanguage", func(t *testing.T) {
		lang := DecoderStreamLanguage()
		require.NotNil(t, lang)
		require.Equal(t, 3, len(*lang))

		counts := make(map[*Instruction]int)
		for b := 0; b <= 255; b++ {
			byteVal := byte(b)
			var matched *Instruction
			matchCount := 0

			for _, inst := range *lang {
				if (byteVal & inst.Opcode.Mask) == inst.Opcode.Value {
					matchCount++
					matched = inst
				}
			}

			require.Equal(
				t,
				1,
				matchCount,
				fmt.Sprintf("DecoderStream byte 0x%02x matched %d instructions", byteVal, matchCount),
			)
			counts[matched]++

			switch {
			case byteVal >= 0x80:
				assert.Equal(t, HeaderAcknowledgementInstruction(), matched)
			case byteVal >= 0x40:
				assert.Equal(t, StreamCancellationInstruction(), matched)
			default:
				assert.Equal(t, InsertCountIncrementInstruction(), matched)
			}
		}

		assert.Equal(t, 128, counts[HeaderAcknowledgementInstruction()])
		assert.Equal(t, 64, counts[StreamCancellationInstruction()])
		assert.Equal(t, 64, counts[InsertCountIncrementInstruction()])
	})

	t.Run("PrefixLanguage", func(t *testing.T) {
		lang := PrefixLanguage()
		require.NotNil(t, lang)
		require.Equal(t, 1, len(*lang))

		counts := make(map[*Instruction]int)
		for b := 0; b <= 255; b++ {
			byteVal := byte(b)
			var matched *Instruction
			matchCount := 0

			for _, inst := range *lang {
				if (byteVal & inst.Opcode.Mask) == inst.Opcode.Value {
					matchCount++
					matched = inst
				}
			}

			require.Equal(
				t,
				1,
				matchCount,
				fmt.Sprintf("Prefix byte 0x%02x matched %d instructions", byteVal, matchCount),
			)
			counts[matched]++
			assert.Equal(t, PrefixInstruction(), matched)
		}

		assert.Equal(t, 256, counts[PrefixInstruction()])
	})

	t.Run("RequestStreamLanguage", func(t *testing.T) {
		lang := RequestStreamLanguage()
		require.NotNil(t, lang)
		require.Equal(t, 5, len(*lang))

		counts := make(map[*Instruction]int)
		for b := 0; b <= 255; b++ {
			byteVal := byte(b)
			var matched *Instruction
			matchCount := 0

			for _, inst := range *lang {
				if (byteVal & inst.Opcode.Mask) == inst.Opcode.Value {
					matchCount++
					matched = inst
				}
			}

			require.Equal(
				t,
				1,
				matchCount,
				fmt.Sprintf("RequestStream byte 0x%02x matched %d instructions", byteVal, matchCount),
			)
			counts[matched]++

			switch {
			case byteVal >= 0x80:
				assert.Equal(t, IndexedHeaderFieldInstruction(), matched)
			case byteVal >= 0x40:
				assert.Equal(t, LiteralHeaderFieldNameReferenceInstruction(), matched)
			case byteVal >= 0x20:
				assert.Equal(t, LiteralHeaderFieldInstruction(), matched)
			case byteVal >= 0x10:
				assert.Equal(t, IndexedHeaderFieldPostBaseInstruction(), matched)
			default:
				assert.Equal(t, LiteralHeaderFieldPostBaseNameReferenceInstruction(), matched)
			}
		}

		assert.Equal(t, 128, counts[IndexedHeaderFieldInstruction()])
		assert.Equal(t, 64, counts[LiteralHeaderFieldNameReferenceInstruction()])
		assert.Equal(t, 32, counts[LiteralHeaderFieldInstruction()])
		assert.Equal(t, 16, counts[IndexedHeaderFieldPostBaseInstruction()])
		assert.Equal(t, 16, counts[LiteralHeaderFieldPostBaseNameReferenceInstruction()])
	})
}

// ============================================================================
// Test 2: Exhaustive Roundtrip Oracle for All 13 Instructions & Field Variations
// ============================================================================

type oracleTestCase struct {
	name     string
	language *Language
	inst     *InstructionWithValues
}

func generateExhaustiveOracleTestCases() []oracleTestCase {
	var cases []oracleTestCase

	// Common test strings
	asciiShort := "hello"
	asciiMedium := "application/json; charset=utf-8"
	huffShorter := "foo"         // Huffman encodes to 2 bytes (< 3)
	huffEqual := "bar"           // Huffman encodes to 3 bytes (== 3)
	huffLonger := "\xff\xfe\xfd" // Huffman encoding expands
	binaryWithNull := string([]byte{0x00, 0x01, 0x00, 0xff, 0x7f, 0x80, 0xaa})
	binaryAllBytes := func() string {
		b := make([]byte, 256)
		for i := 0; i < 256; i++ {
			b[i] = byte(i)
		}
		return string(b)
	}()
	longString := strings.Repeat("x-header-chunk-value-", 30) // ~630 bytes

	// Common varint test boundaries
	testVarints := []uint64{
		0,
		1,
		2,
		7, 8,
		15, 16,
		31, 32,
		63, 64,
		127, 128,
		255, 256,
		1000,
		16383, 16384,
		65535, 65536,
		1 << 20,
		1 << 32,
		1 << 48,
		1 << 62,
		math.MaxUint64 - 1,
		math.MaxUint64,
	}

	// 1. InsertWithNameReference (RFC 9204 §4.3.1): S-bit, Varint(6), Value(7)
	for _, sBit := range []bool{false, true} {
		for _, vi := range []uint64{0, 1, 62, 63, 64, 127, 16384, math.MaxUint64} {
			for _, val := range []string{"", asciiShort, huffShorter, huffEqual, huffLonger, binaryWithNull} {
				inst := InstructionWithValuesInsertWithNameReference(sBit, vi, val)
				cases = append(cases, oracleTestCase{
					name:     fmt.Sprintf("1_InsertWithNameRef_S%t_VI%d_Len%d", sBit, vi, len(val)),
					language: EncoderStreamLanguage(),
					inst:     inst,
				})
			}
		}
	}

	// 2. InsertWithoutNameReference (RFC 9204 §4.3.2): Name(5), Value(7)
	nameSamples := []string{"", "a", ":path", "custom-header-key", huffShorter, huffEqual, binaryWithNull}
	valueSamples := []string{"", "1", "application/octet-stream", huffShorter, binaryWithNull, longString}
	for _, name := range nameSamples {
		for _, val := range valueSamples {
			inst := InstructionWithValuesInsertWithoutNameReference(name, val)
			cases = append(cases, oracleTestCase{
				name:     fmt.Sprintf("2_InsertWithoutNameRef_NLen%d_VLen%d", len(name), len(val)),
				language: EncoderStreamLanguage(),
				inst:     inst,
			})
		}
	}

	// 3. Duplicate (RFC 9204 §4.3.3): Varint(5)
	for _, vi := range testVarints {
		inst := InstructionWithValuesDuplicate(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("3_Duplicate_VI%d", vi),
			language: EncoderStreamLanguage(),
			inst:     inst,
		})
	}

	// 4. SetDynamicTableCapacity (RFC 9204 §4.3.4): Varint(5)
	for _, vi := range testVarints {
		inst := InstructionWithValuesSetDynamicTableCapacity(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("4_SetDynamicTableCapacity_VI%d", vi),
			language: EncoderStreamLanguage(),
			inst:     inst,
		})
	}

	// 5. HeaderAcknowledgement (RFC 9204 §4.4.1): Varint(7)
	for _, vi := range testVarints {
		inst := InstructionWithValuesHeaderAcknowledgement(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("5_HeaderAcknowledgement_VI%d", vi),
			language: DecoderStreamLanguage(),
			inst:     inst,
		})
	}

	// 6. StreamCancellation (RFC 9204 §4.4.2): Varint(6)
	for _, vi := range testVarints {
		inst := InstructionWithValuesStreamCancellation(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("6_StreamCancellation_VI%d", vi),
			language: DecoderStreamLanguage(),
			inst:     inst,
		})
	}

	// 7. InsertCountIncrement (RFC 9204 §4.4.3): Varint(6)
	for _, vi := range testVarints {
		inst := InstructionWithValuesInsertCountIncrement(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("7_InsertCountIncrement_VI%d", vi),
			language: DecoderStreamLanguage(),
			inst:     inst,
		})
	}

	// 8. Prefix (RFC 9204 §4.5.1): Varint(8), S-bit, Varint2(7)
	for _, sBit := range []bool{false, true} {
		for _, vi := range []uint64{0, 1, 254, 255, 256, 65536, math.MaxUint64} {
			for _, vi2 := range []uint64{0, 1, 126, 127, 128, 10000, math.MaxUint64} {
				inst := InstructionWithValuesPrefix(vi)
				inst.SetSBit(sBit)
				inst.SetVarint2(vi2)
				cases = append(cases, oracleTestCase{
					name:     fmt.Sprintf("8_Prefix_VI%d_S%t_VI2%d", vi, sBit, vi2),
					language: PrefixLanguage(),
					inst:     inst,
				})
			}
		}
	}

	// 9. IndexedHeaderField (RFC 9204 §4.5.2): S-bit, Varint(6)
	for _, sBit := range []bool{false, true} {
		for _, vi := range testVarints {
			inst := InstructionWithValuesIndexedHeaderField(sBit, vi)
			cases = append(cases, oracleTestCase{
				name:     fmt.Sprintf("9_IndexedHeaderField_S%t_VI%d", sBit, vi),
				language: RequestStreamLanguage(),
				inst:     inst,
			})
		}
	}

	// 10. LiteralHeaderFieldNameReference (RFC 9204 §4.5.4): S-bit, Varint(4), Value(7)
	for _, sBit := range []bool{false, true} {
		for _, vi := range []uint64{0, 1, 14, 15, 16, 127, 65535, math.MaxUint64} {
			for _, val := range []string{"", asciiMedium, huffShorter, binaryWithNull, binaryAllBytes} {
				inst := InstructionWithValuesLiteralHeaderFieldNameReference(sBit, vi, val)
				cases = append(cases, oracleTestCase{
					name:     fmt.Sprintf("10_LiteralHeaderFieldNameRef_S%t_VI%d_VLen%d", sBit, vi, len(val)),
					language: RequestStreamLanguage(),
					inst:     inst,
				})
			}
		}
	}

	// 11. LiteralHeaderField (RFC 9204 §4.5.6): Name(3), Value(7)
	for _, name := range []string{"", ":status", "x-auth-key", huffShorter, binaryWithNull} {
		for _, val := range []string{"", "200", asciiMedium, binaryWithNull, longString} {
			inst := InstructionWithValuesLiteralHeaderField(name, val)
			cases = append(cases, oracleTestCase{
				name:     fmt.Sprintf("11_LiteralHeaderField_NLen%d_VLen%d", len(name), len(val)),
				language: RequestStreamLanguage(),
				inst:     inst,
			})
		}
	}

	// 12. IndexedHeaderFieldPostBase (RFC 9204 §4.5.3): Varint(4)
	for _, vi := range testVarints {
		inst := InstructionWithValuesIndexedHeaderFieldPostBase(vi)
		cases = append(cases, oracleTestCase{
			name:     fmt.Sprintf("12_IndexedHeaderFieldPostBase_VI%d", vi),
			language: RequestStreamLanguage(),
			inst:     inst,
		})
	}

	// 13. LiteralHeaderFieldPostBaseNameReference (RFC 9204 §4.5.5): Varint(3), Value(7)
	for _, vi := range []uint64{0, 1, 6, 7, 8, 127, 65536, math.MaxUint64} {
		for _, val := range []string{"", "pb-value", huffShorter, binaryWithNull, longString} {
			inst := InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(vi, val)
			cases = append(cases, oracleTestCase{
				name:     fmt.Sprintf("13_LiteralHeaderFieldPostBaseNameRef_VI%d_VLen%d", vi, len(val)),
				language: RequestStreamLanguage(),
				inst:     inst,
			})
		}
	}

	return cases
}

func TestM2Oracle_ExhaustiveRoundtrip_AllInstructions(t *testing.T) {
	testCases := generateExhaustiveOracleTestCases()
	require.NotEmpty(t, testCases)

	huffmanModes := []HuffmanEncoding{HuffmanEncodingEnabled, HuffmanEncodingDisabled}
	slicingModes := []oracleSliceMode{
		oracleSliceSingleChunk,
		oracleSliceOctetByOctet,
		oracleSliceTwoBytes,
		oracleSliceThreeBytes,
		oracleSliceRandomStep,
	}

	rng := rand.New(rand.NewSource(133742))

	for _, tc := range testCases {
		for _, huff := range huffmanModes {
			for _, sliceMode := range slicingModes {
				// Encode
				encoder := NewInstructionEncoder(huff)
				encoded := encoder.Encode(tc.inst, nil)
				require.NotEmpty(
					t,
					encoded,
					fmt.Sprintf("%s [%s, %s] produced empty encoded bytes", tc.name, huff, sliceMode),
				)

				// Decode
				delegate := newOracleDecoderDelegate()
				decoder := NewInstructionDecoder(tc.language, delegate)
				delegate.SetDecoder(decoder)

				require.True(t, decoder.AtInstructionBoundary())
				ok := feedDecoderSlices(decoder, encoded, sliceMode, rng)

				require.True(t, ok, fmt.Sprintf("%s [%s, %s] decode returned false", tc.name, huff, sliceMode))
				assert.False(
					t,
					decoder.HasError(),
					fmt.Sprintf("%s [%s, %s] decoder has error: %v", tc.name, huff, sliceMode, delegate.errorMsgs),
				)
				require.True(
					t,
					decoder.AtInstructionBoundary(),
					fmt.Sprintf("%s [%s, %s] not at boundary at EOF", tc.name, huff, sliceMode),
				)

				// Verify callback
				require.Equal(
					t,
					1,
					len(delegate.snapshots),
					fmt.Sprintf("%s [%s, %s] decoded snapshot count mismatch", tc.name, huff, sliceMode),
				)
				snap := delegate.snapshots[0]

				// Strict value assertions
				assert.Equal(
					t,
					tc.inst.Instruction(),
					snap.instruction,
					fmt.Sprintf("%s instruction mismatch", tc.name),
				)
				assert.Equal(t, tc.inst.SBit(), snap.sBit, fmt.Sprintf("%s SBit mismatch", tc.name))
				assert.Equal(t, tc.inst.Varint(), snap.varint, fmt.Sprintf("%s Varint mismatch", tc.name))
				assert.Equal(t, tc.inst.Varint2(), snap.varint2, fmt.Sprintf("%s Varint2 mismatch", tc.name))
				assert.Equal(t, tc.inst.Name(), snap.name, fmt.Sprintf("%s Name mismatch", tc.name))
				assert.Equal(t, tc.inst.Value(), snap.value, fmt.Sprintf("%s Value mismatch", tc.name))
			}
		}
	}
}

// ============================================================================
// Test 3: Multi-Instruction Continuous Stream Roundtrip Oracle
// ============================================================================

func TestM2Oracle_MultiInstructionStreamRoundtrip(t *testing.T) {
	// Real QUIC encoder and decoder streams deliver multiple consecutive instructions
	// in a continuous byte stream. The decoder must continuously reset state and parse
	// all instructions across multiple chunk boundaries without losing position.

	type streamTestPlan struct {
		name     string
		language *Language
		sequence []*InstructionWithValues
	}

	plans := []streamTestPlan{
		{
			name:     "EncoderStreamSequence",
			language: EncoderStreamLanguage(),
			sequence: []*InstructionWithValues{
				InstructionWithValuesSetDynamicTableCapacity(4096),
				InstructionWithValuesInsertWithoutNameReference(":authority", "example.com"),
				InstructionWithValuesInsertWithNameReference(true, 0, "dynamic-val-1"),
				InstructionWithValuesDuplicate(0),
				InstructionWithValuesInsertWithoutNameReference("content-type", "application/json"),
				InstructionWithValuesInsertWithNameReference(false, 2, "dynamic-val-2"),
				InstructionWithValuesDuplicate(3),
				InstructionWithValuesSetDynamicTableCapacity(8192),
				InstructionWithValuesInsertWithoutNameReference("x-custom-bin", string([]byte{0x00, 0xff, 0x7f})),
				InstructionWithValuesDuplicate(math.MaxUint64),
			},
		},
		{
			name:     "DecoderStreamSequence",
			language: DecoderStreamLanguage(),
			sequence: []*InstructionWithValues{
				InstructionWithValuesHeaderAcknowledgement(0),
				InstructionWithValuesInsertCountIncrement(1),
				InstructionWithValuesStreamCancellation(4),
				InstructionWithValuesHeaderAcknowledgement(1024),
				InstructionWithValuesInsertCountIncrement(63),
				InstructionWithValuesInsertCountIncrement(64),
				InstructionWithValuesStreamCancellation(99999),
				InstructionWithValuesHeaderAcknowledgement(math.MaxUint64),
				InstructionWithValuesInsertCountIncrement(math.MaxUint64),
			},
		},
		{
			name:     "RequestStreamSequence",
			language: RequestStreamLanguage(),
			sequence: []*InstructionWithValues{
				InstructionWithValuesIndexedHeaderField(true, 1),
				InstructionWithValuesIndexedHeaderField(false, 42),
				InstructionWithValuesLiteralHeaderFieldNameReference(true, 15, "gzip"),
				InstructionWithValuesLiteralHeaderFieldNameReference(false, 100, "chunked"),
				InstructionWithValuesLiteralHeaderField(":status", "200"),
				InstructionWithValuesLiteralHeaderField("content-length", "1048576"),
				InstructionWithValuesIndexedHeaderFieldPostBase(0),
				InstructionWithValuesIndexedHeaderFieldPostBase(15),
				InstructionWithValuesIndexedHeaderFieldPostBase(16),
				InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(0, "post-base-val-1"),
				InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(7, "post-base-val-2"),
				InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(math.MaxUint64, "post-base-max"),
				InstructionWithValuesIndexedHeaderField(false, math.MaxUint64),
			},
		},
	}

	huffmanModes := []HuffmanEncoding{HuffmanEncodingEnabled, HuffmanEncodingDisabled}
	slicingModes := []oracleSliceMode{
		oracleSliceSingleChunk,
		oracleSliceOctetByOctet,
		oracleSliceTwoBytes,
		oracleSliceThreeBytes,
		oracleSliceRandomStep,
	}

	rng := rand.New(rand.NewSource(998877))

	for _, plan := range plans {
		t.Run(plan.name, func(t *testing.T) {
			for _, huff := range huffmanModes {
				t.Run(huff.String(), func(t *testing.T) {
					for _, sliceMode := range slicingModes {
						t.Run(sliceMode.String(), func(t *testing.T) {
							// Encode all instructions into single concatenated byte stream
							encoder := NewInstructionEncoder(huff)
							var streamBuffer []byte
							for _, item := range plan.sequence {
								streamBuffer = encoder.Encode(item, streamBuffer)
							}
							require.NotEmpty(t, streamBuffer)

							// Single decoder instance for the entire stream
							delegate := newOracleDecoderDelegate()
							decoder := NewInstructionDecoder(plan.language, delegate)
							delegate.SetDecoder(decoder)

							ok := feedDecoderSlices(decoder, streamBuffer, sliceMode, rng)
							require.True(t, ok)
							assert.False(t, decoder.HasError(), fmt.Sprintf("decoder error: %v", delegate.errorMsgs))
							require.True(t, decoder.AtInstructionBoundary())

							// Verify count and individual snapshots
							require.Equal(
								t,
								len(plan.sequence),
								len(delegate.snapshots),
								"mismatch in decoded instruction count",
							)
							for i, expected := range plan.sequence {
								actual := delegate.snapshots[i]
								assert.Equal(
									t,
									expected.Instruction(),
									actual.instruction,
									fmt.Sprintf("idx %d instruction mismatch", i),
								)
								assert.Equal(t, expected.SBit(), actual.sBit, fmt.Sprintf("idx %d sBit mismatch", i))
								assert.Equal(
									t,
									expected.Varint(),
									actual.varint,
									fmt.Sprintf("idx %d varint mismatch", i),
								)
								assert.Equal(
									t,
									expected.Varint2(),
									actual.varint2,
									fmt.Sprintf("idx %d varint2 mismatch", i),
								)
								assert.Equal(t, expected.Name(), actual.name, fmt.Sprintf("idx %d name mismatch", i))
								assert.Equal(t, expected.Value(), actual.value, fmt.Sprintf("idx %d value mismatch", i))
							}
						})
					}
				})
			}
		})
	}
}

// ============================================================================
// Test 4: Randomized Property & Slicing Stress Oracle
// ============================================================================

func TestM2Oracle_RandomizedPropertyStress(t *testing.T) {
	rng := rand.New(rand.NewSource(20260919))

	randomString := func(maxLen int) string {
		length := rng.Intn(maxLen)
		b := make([]byte, length)
		for i := 0; i < length; i++ {
			// Mix of printable ascii, null, and high bytes
			pick := rng.Intn(4)
			switch pick {
			case 0:
				b[i] = 0x00
			case 1:
				b[i] = byte('a' + rng.Intn(26))
			case 2:
				b[i] = byte(0x80 + rng.Intn(128))
			case 3:
				b[i] = byte('0' + rng.Intn(10))
			}
		}
		return string(b)
	}

	randomVarint := func() uint64 {
		switch rng.Intn(6) {
		case 0:
			return uint64(rng.Intn(128))
		case 1:
			return uint64(rng.Intn(65536))
		case 2:
			return uint64(rng.Int63n(1 << 30))
		case 3:
			return uint64(rng.Int63())
		case 4:
			return math.MaxUint64 - uint64(rng.Intn(10))
		default:
			return math.MaxUint64
		}
	}

	for iter := 0; iter < 100; iter++ {
		instType := rng.Intn(13)
		var inst *InstructionWithValues
		var lang *Language

		switch instType {
		case 0:
			lang = EncoderStreamLanguage()
			inst = InstructionWithValuesInsertWithNameReference(rng.Intn(2) == 1, randomVarint(), randomString(100))
		case 1:
			lang = EncoderStreamLanguage()
			inst = InstructionWithValuesInsertWithoutNameReference(randomString(50), randomString(150))
		case 2:
			lang = EncoderStreamLanguage()
			inst = InstructionWithValuesDuplicate(randomVarint())
		case 3:
			lang = EncoderStreamLanguage()
			inst = InstructionWithValuesSetDynamicTableCapacity(randomVarint())
		case 4:
			lang = DecoderStreamLanguage()
			inst = InstructionWithValuesHeaderAcknowledgement(randomVarint())
		case 5:
			lang = DecoderStreamLanguage()
			inst = InstructionWithValuesStreamCancellation(randomVarint())
		case 6:
			lang = DecoderStreamLanguage()
			inst = InstructionWithValuesInsertCountIncrement(randomVarint())
		case 7:
			lang = PrefixLanguage()
			inst = InstructionWithValuesPrefix(randomVarint())
			inst.SetSBit(rng.Intn(2) == 1)
			inst.SetVarint2(randomVarint())
		case 8:
			lang = RequestStreamLanguage()
			inst = InstructionWithValuesIndexedHeaderField(rng.Intn(2) == 1, randomVarint())
		case 9:
			lang = RequestStreamLanguage()
			inst = InstructionWithValuesLiteralHeaderFieldNameReference(
				rng.Intn(2) == 1,
				randomVarint(),
				randomString(80),
			)
		case 10:
			lang = RequestStreamLanguage()
			inst = InstructionWithValuesLiteralHeaderField(randomString(40), randomString(100))
		case 11:
			lang = RequestStreamLanguage()
			inst = InstructionWithValuesIndexedHeaderFieldPostBase(randomVarint())
		case 12:
			lang = RequestStreamLanguage()
			inst = InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(randomVarint(), randomString(75))
		}

		huff := HuffmanEncoding(rng.Intn(2))
		encoder := NewInstructionEncoder(huff)
		encoded := encoder.Encode(inst, nil)
		require.NotEmpty(t, encoded)

		sliceMode := oracleSliceMode(rng.Intn(5))
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(lang, delegate)
		delegate.SetDecoder(decoder)

		ok := feedDecoderSlices(decoder, encoded, sliceMode, rng)
		require.True(t, ok, fmt.Sprintf("iter %d failed decode", iter))
		assert.False(t, decoder.HasError(), fmt.Sprintf("iter %d error: %v", iter, delegate.errorMsgs))
		require.True(t, decoder.AtInstructionBoundary(), fmt.Sprintf("iter %d not at boundary", iter))

		require.Equal(t, 1, len(delegate.snapshots), fmt.Sprintf("iter %d snapshot count mismatch", iter))
		snap := delegate.snapshots[0]
		assert.Equal(t, inst.Instruction(), snap.instruction)
		assert.Equal(t, inst.SBit(), snap.sBit)
		assert.Equal(t, inst.Varint(), snap.varint)
		assert.Equal(t, inst.Varint2(), snap.varint2)
		assert.Equal(t, inst.Name(), snap.name)
		assert.Equal(t, inst.Value(), snap.value)
	}
}

// ============================================================================
// Test 5: Adversarial Decoder Error Boundaries & Invariant Enforcement
// ============================================================================

func TestM2Oracle_DecoderErrorBoundaries(t *testing.T) {
	t.Run("TruncatedVarintAtEOF", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(DecoderStreamLanguage(), delegate)

		// HeaderAck opcode 0x80 with continuation bit set (0x80 | 0x7f = 0xff), followed by 0x81 (continuation)
		truncatedData := []byte{0xff, 0x81}
		ok := decoder.Decode(truncatedData)
		require.True(t, ok)
		assert.False(t, decoder.AtInstructionBoundary())
		assert.False(t, decoder.HasError())

		// Calling EndDecoding must detect truncated state
		decoder.EndDecoding()
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCodes))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCodes[0])
		assert.Equal(t, "Truncated instruction.", delegate.errorMsgs[0])
	})

	t.Run("TruncatedStringLiteralAtEOF", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		// InsertWithoutNameReference: opcode 0x40.
		// Name: not Huffman, length 10 -> byte 0x4a (0x40 | 10).
		// Provide only 4 bytes of name:
		data := []byte{0x4a, 't', 'e', 's', 't'}
		ok := decoder.Decode(data)
		require.True(t, ok)
		assert.False(t, decoder.AtInstructionBoundary())

		decoder.EndDecoding()
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCodes))
		assert.Equal(t, "Truncated instruction.", delegate.errorMsgs[0])
	})

	t.Run("VarintOverflowReporting", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(DecoderStreamLanguage(), delegate)

		// 11 consecutive continuation bytes for 64-bit varint
		overflowBytes := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		ok := decoder.Decode(overflowBytes)
		assert.False(t, ok)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCodes))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCodes[0])
		assert.Equal(t, "Encoded integer too large.", delegate.errorMsgs[0])

		// Subsequent decode calls must fail immediately
		assert.False(t, decoder.Decode([]byte{0x00}))
	})

	t.Run("StringLiteralLengthLimitExceeded", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		// InsertWithoutNameReference: opcode 0x40.
		// Name prefix 5: 0x5f (1f = 31).
		// Continuation bytes encoding length = 1024*1024 + 1 = 1048577.
		// 1048577 - 31 = 1048546 = 0xfffe2 -> little endian 7-bit:
		// 1048546 % 128 = 98 (0x62) -> 0xe2
		// 1048546 >> 7 = 8191 % 128 = 127 (0x7f) -> 0xff
		// 8191 >> 7 = 63 (0x3f) -> 0x3f
		tooLongLengthBytes := []byte{0x5f, 0xe2, 0xff, 0x3f}
		ok := decoder.Decode(tooLongLengthBytes)
		assert.False(t, ok)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCodes))
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, delegate.errorCodes[0])
		assert.Equal(t, "String literal too long.", delegate.errorMsgs[0])
	})

	t.Run("MalformedHuffmanRejected", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		// InsertWithoutNameReference: opcode 0x40.
		// Name: Huffman flag (bit 5 = 0x20), length 2 (0x40 | 0x20 | 0x02 = 0x62).
		// Invalid Huffman payload "c1ff" (invalid EOS padding)
		badHuffman := []byte{0x62, 0xc1, 0xff}
		ok := decoder.Decode(badHuffman)
		assert.False(t, ok)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCodes))
		assert.Equal(t, InstructionDecoderHuffmanEncodingError, delegate.errorCodes[0])
		assert.Equal(t, "Error in Huffman-encoded string.", delegate.errorMsgs[0])
	})

	t.Run("DelegateRejectionStopsDecoding", func(t *testing.T) {
		delegate := newOracleDecoderDelegate()
		delegate.allowDecode = false // simulate semantic validation rejection
		decoder := NewInstructionDecoder(DecoderStreamLanguage(), delegate)
		delegate.SetDecoder(decoder)

		inst := InstructionWithValuesHeaderAcknowledgement(10)
		encoder := NewInstructionEncoder(HuffmanEncodingDisabled)
		encoded := encoder.Encode(inst, nil)

		ok := decoder.Decode(encoded)
		assert.False(t, ok, "decoder should stop when delegate returns false")
		assert.True(t, decoder.HasError(), "decoder should record error on delegate rejection")
	})
}
