// Package spec is internal, not public: the names, digit sizes, SWAR masks and kernel
// sizes of the hex plugin.
//
// Revisions:
//   - 2026-11-08 00:59: initial creation (C91w12c, Q228, Q243)
package spec

// NAME is the module, and the six names it holds.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	NAME        = "hex"
	FROM_BYTES  = "from_bytes"
	TO_BYTES    = "to_bytes"
	FROM_INT    = "from_int"
	TO_INT      = "to_int"
	FROM_BINARY = "from_binary"
	TO_BINARY   = "to_binary"

	// BITS_PER_DIGIT is how many bits one digit holds, DIGITS_PER_BYTE how
	// many digits make a whole byte, and BASE what the digits count in.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	BITS_PER_DIGIT  = 4
	DIGITS_PER_BYTE = 2
	BASE            = 16

	// UPPER is the digits in the order of their values, in the one case the
	// runtime writes, and LOW_DIGIT the mask of the low digit of a byte.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	UPPER     = "0123456789ABCDEF"
	LOW_DIGIT = 0x0f

	// PREFIX is what every reader here takes off before reading, in either
	// case, and PAD the digit an odd count of digits is padded with.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	PREFIX = "0x"
	PAD    = "0"
)

// The sizes below which a path is not worth its start. A size counts data
// bytes for encode and digits for decode. Each is at least the size the kernel
// needs to read one whole block, which is checked in the kernel's own wrapper.
// The crossovers for SWAR, SSE and AVX2 were measured on an i7-12800H; the
// AVX-512 sizes ENCODE_AVX512_MIN (128 bytes) and DECODE_AVX512_MIN (256
// digits) are UNMEASURED GUESSES. Intel SDE emulates AVX-512 and cannot time
// real hardware, and this CPU has none. Measure them on an AVX-512 machine:
//
//	go test -run '^$' -bench 'Kernels_(Encode|Decode)' -benchmem -count 6 \
//	    ./v1/runtime/plugin/codec/hex
//
// Compare the avx512 rows with the avx2 rows at 64 to 1024, and move the
// constant to the size where avx512 first wins.
//
// Revisions:
//   - 2026-11-03 21:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	ENCODE_SWAR_MIN = 8

	ENCODE_SSE_MIN    = 16
	ENCODE_AVX2_MIN   = 32
	ENCODE_AVX512_MIN = 128

	DECODE_SSE_MIN    = 32
	DECODE_AVX2_MIN   = 32
	DECODE_AVX512_MIN = 256
)

// The constants of the SWAR encoder. A word holds 8 bytes, or 4 lanes of 16
// bits. A mask name says which bits of the word it keeps.
//
// Revisions:
//   - 2026-11-03 21:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	// SWAR_IN is how many data bytes one step reads. They make 8 digits, which
	// are one word.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	SWAR_IN = 4

	SWAR_ONES   = 0x0101010101010101
	SWAR_LANE16 = 0x0000ffff0000ffff
	SWAR_LANE8  = 0x00ff00ff00ff00ff
	SWAR_LANE4  = 0x000f000f000f000f

	// SWAR_ADD_SIX makes bit 4 of a nibble of 10 or more set, and
	// SWAR_ASCII_ZERO is the character '0' in every byte.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	SWAR_ADD_SIX    = 0x0606060606060606
	SWAR_ASCII_ZERO = 0x3030303030303030

	// The shifts of the lane arithmetic: a lane of 16 bits, and a byte.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	SHIFT_LANE16 = 16
	SHIFT_BYTE   = 8

	// LETTER_GAP is how far the letter A lies above the digit 9 in ASCII, less
	// one: '9' is 0x39 and 'A' is 0x41.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	LETTER_GAP = 7

	// CARRY_SHIFT is the bit that an add of 6 sets in a nibble of 10 or more.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	CARRY_SHIFT = 4
)
