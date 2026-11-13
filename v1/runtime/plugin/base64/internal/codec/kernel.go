// Package codec holds the coding of the base64 plugin: the Coding of an
// alphabet, the encode and decode functions, and the kernels with their
// portable, SSE, AVX2 and AVX-512 paths. The plugin type, its registration, its
// streams and its names stay in package base64.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package codec

import (
	"encoding/base64"

	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// Coding is how one builtin writes and reads: the alphabet and the encoding
// of the standard library that has it, and the level and the sizes that choose
// the kernels. A builtin holds one for its life; a test makes one with a lower
// level or smaller sizes.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 and renamed _Coding to Coding, for
//     the tests in tests/ (C120b, Q315, Q314)
type Coding struct {
	Level    cpu.Level
	Sizes    *spec.Sizes
	Alphabet int
	Encoding *base64.Encoding
}

// _Run is what a kernel is told, by pointer so that a kernel takes few
// arguments: Size is the bytes (encode) or characters (decode) it may read, Room
// is the bytes of the destination it may write (decode), and Alphabet is the
// index of the tables it uses. The field offsets, 0, 8 and 16, are read by
// kernel_amd64.s, so _Run keeps its underscore name and its field order, and
// moves with the stubs that take it (C120b).
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
type _Run struct {
	Size     int
	Room     int
	Alphabet int
}

// Encode writes the base64 of src to dst, in the alphabet and padding of the
// coding. dst must hold Encoding.EncodedLen(len(src)) bytes, and must not
// overlap src.
//
// The widest kernel that the level and the size allow runs on whole blocks. The
// rest, and every size below the sizes of the coding, goes to encoding/base64,
// which also writes the padding. A word-at-a-time (SWAR) encoder was written
// and was no faster than the standard library, so none ships (C51 report).
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 and renamed _Encode to Encode, for
//     the tests in tests/ (C120b, Q315, Q314)
func Encode(
	coding *Coding,
	dst []byte,
	src []byte,
) {
	var done int

	if len(src) >= coding.Sizes.EncodeSSE {
		done = _EncodeKernel(
			coding,
			dst,
			src,
		)
	}

	coding.Encoding.Encode(
		dst[done/spec.GROUP_BYTES*spec.GROUP:],
		src[done:],
	)
}

// Decode writes to dst the bytes that the text src stands for, and returns how
// many, with whether every character was in the alphabet and the last group was
// one that the encoding accepts. src has no padding; the encoding is the
// unpadded Strict form of its alphabet. dst must hold Encoding.DecodedLen(len(src))
// bytes and must not overlap src. After a false, dst holds nothing to rely on,
// and the caller reports the error.
//
// The kernels do whole blocks, and stop before the last group, so that
// encoding/base64 reads the end of the text and applies the strict check of the
// pad bits as before (C22).
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 and renamed _Decode to Decode, for
//     the tests in tests/ (C120b, Q315, Q314)
func Decode(
	coding *Coding,
	dst []byte,
	src []byte,
) (int, bool) {
	var done int

	if len(src) >= coding.Sizes.DecodeSSE {
		done = _DecodeKernel(
			coding,
			dst,
			src,
		)
	}

	if done < 0 {
		return 0, false
	}

	var (
		start  = done / spec.GROUP * spec.GROUP_BYTES
		n, err = coding.Encoding.Decode(
			dst[start:],
			src[done:],
		)
	)

	return start + n, err == nil
}
