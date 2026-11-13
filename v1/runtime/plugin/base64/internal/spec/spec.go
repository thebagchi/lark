// Package spec is internal, not public: the names, alphabets, kernel sizes and CPU
// levels of the base64 plugin.
//
// Revisions:
//   - 2026-11-08 00:59: initial creation (C91w12c, Q228, Q243)
package spec

// NAME is the module, and the four names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	NAME      = "base64"
	ENCODE    = "encode"
	DECODE    = "decode"
	URLENCODE = "urlencode"
	URLDECODE = "urldecode"

	// GROUP is how many characters one group of the encoding holds, which the
	// padding of RFC 4648 section 4 fills.
	//
	// Revisions:
	//   - 2026-11-02 16:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	GROUP = 4
)

// The alphabets, as the kernels name them: the index of the record that holds
// the tables of one alphabet.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	// ALPHABET_STD is the alphabet of RFC 4648 section 4: "+" and "/".
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	ALPHABET_STD = 0

	// ALPHABET_URL is the URL and filename safe alphabet of section 5: "-" and
	// "_".
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	ALPHABET_URL = 1
)

// The sizes below which a kernel is not worth its start. A size counts data
// bytes for encode and characters for decode. A kernel needs a whole block to
// read, which its own loop test checks, so a size below the block is safe and
// only slow.
//
// The sizes come from the whole call of a builtin (C51 fix round 1), not from
// the kernel alone: a call allocates its result and boxes it, which costs about
// 150 ns, so a kernel that wins by 20 ns does not win the call. Measured on an
// i7-12800H as CPU time per call, the minimum of 15 batches of three runs: the
// SSE kernels tie with encoding/base64 up to about 64 bytes, and are faster in
// the minimum of the runs from 96 (encode 271 ns to 246 ns, decode of 128
// characters 353 ns to 314 ns, 128 bytes 316 ns to 247 ns and 481 ns to
// 346 ns). The spread of those runs was large, so read them as a trend. AVX2
// over SSE, as the tester measured it (benchstat, 15 to 30 interleaved rounds):
// encode has no difference below 512 bytes and wins from 512 (512 bytes -26%,
// p 0.029; 4096 bytes -30%, p 0.007). Decode shows no whole-call gain at any
// size from 160 to 4096 bytes (p 0.096 to 0.91). The AVX2 decode kernel stays
// because the user requires SSE, AVX2 and AVX-512 (Q93); its size 256 is the
// point where the kernel alone is ahead of SSE, and no gain is claimed for the
// call. The kernels alone beat the standard library much earlier (SSE from 28
// bytes); the SSE sizes are the whole-call break-even.
//
// The AVX-512 sizes ENCODE_AVX512_MIN and DECODE_AVX512_MIN (1024 each, twice
// the AVX2 start, because the block is twice as wide) are UNMEASURED GUESSES.
// Intel SDE emulates AVX-512 and cannot time real hardware, and this CPU has
// none. Measure them on an AVX-512 machine:
//
//	go test -run '^$' -bench 'Kernels_(Encode|Decode)' -benchmem -count 6 \
//	    ./v1/runtime/plugin/base64
//
// Compare the avx512 rows with the avx2 rows at 64 to 4096, and move the
// constant to the size where avx512 first wins.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	ENCODE_SSE_MIN    = 64
	ENCODE_AVX2_MIN   = 512
	ENCODE_AVX512_MIN = 1024

	DECODE_SSE_MIN    = 96
	DECODE_AVX2_MIN   = 256
	DECODE_AVX512_MIN = 1024
)

// Sizes are the least sizes at which each kernel runs. The SSE sizes are the
// least of the three, so a text below them skips the kernels with one compare.
// A test passes the least size a block allows, to run every kernel on every
// size it can; the builtins pass SIZES.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec from _Sizes, as Sizes (C91w12c, Q228)
type Sizes struct {
	EncodeSSE    int
	EncodeAVX2   int
	EncodeAVX512 int
	DecodeSSE    int
	DecodeAVX2   int
	DecodeAVX512 int
}

// SIZES are the sizes that ship. Package vars are capital snake by the
// repository's convention check; nothing writes this one.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
var (
	SIZES = Sizes{
		EncodeSSE:    ENCODE_SSE_MIN,
		EncodeAVX2:   ENCODE_AVX2_MIN,
		EncodeAVX512: ENCODE_AVX512_MIN,
		DecodeSSE:    DECODE_SSE_MIN,
		DecodeAVX2:   DECODE_AVX2_MIN,
		DecodeAVX512: DECODE_AVX512_MIN,
	}
)

// The bytes of a group and the characters that stand for it.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	GROUP_BYTES = 3
)
