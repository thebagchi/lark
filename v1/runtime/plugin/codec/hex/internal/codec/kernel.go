package codec

import (
	stdhex "encoding/hex"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// EncodeAt writes the upper-case hex of src to dst, the high digit of a byte
// first, at the level given. dst must hold 2*len(src) bytes, and must not
// overlap src. A test sets the level to run a narrower path on a wide CPU.
//
// The widest kernel that the level has and the size allows runs first. What it
// leaves, and every size below the thresholds, goes to the portable path.
//
// Revisions:
//   - 2026-11-03 01:50: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-05 19:07: takes the place of _EncodeInto, which only passed cpu.LEVEL (C58)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _EncodeAt to
//     EncodeAt, for the tests in tests/ (C120b, Q315, Q314)
func EncodeAt(
	level cpu.Level,
	dst []byte,
	src []byte,
) {
	dst = dst[:len(src)*spec.DIGITS_PER_BYTE]

	if _EncodeKernel(
		level,
		dst,
		src,
	) {
		return
	}

	if len(src) >= spec.ENCODE_SWAR_MIN {
		EncodeSWAR(
			dst,
			src,
		)

		return
	}

	EncodeTable(
		dst,
		src,
	)
}

// DecodeAt writes to dst the bytes that the hex digits of src stand for, at the
// level given, and reports whether every digit was one. len(src) must be even,
// dst must hold len(src)/2 bytes, and the two must not overlap. After a false,
// dst holds nothing to rely on, and the caller reports the error.
//
// The plain path is encoding/hex.Decode. A word-at-a-time (SWAR) decoder was
// written, matched it on every test, and ran 6 to 15 percent slower from 64
// digits to 64 KiB, so the standard library stays (C50 report, section Numbers).
//
// Revisions:
//   - 2026-11-03 01:50: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-05 19:07: takes the place of _DecodeInto, which only passed cpu.LEVEL (C58)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _DecodeAt to
//     DecodeAt, for the tests in tests/ (C120b, Q315, Q314)
func DecodeAt(
	level cpu.Level,
	dst []byte,
	src []byte,
) bool {
	dst = dst[:len(src)/spec.DIGITS_PER_BYTE]

	handled, valid := _DecodeKernel(
		level,
		dst,
		src,
	)
	if handled {
		return valid
	}

	_, err := stdhex.Decode(
		dst,
		src,
	)

	return err == nil
}
