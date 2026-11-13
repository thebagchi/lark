// Package decode holds the decoding side of the base32 plugin: the decode
// builtin, the reader of its text and the check that the pad bits are zero. The
// plugin type, its registration and the encoder stay in package base32.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package decode

import (
	"encoding/base32"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/base32/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Decode reads base32, padded or not.
//
// Padding off rather than required, for the reason base64's reader gives: both
// spellings of one value are in the wild, and a decoder that accepted only its
// own output would refuse half of what a script is handed.
//
// Three refusals, each a clause of RFC 4648:
//
//   - Excess or wrong-count "=" and any CR or LF: section 3.3, "Implementations
//     MUST reject the encoded data if it contains characters outside the base
//     alphabet ... unless the specification referring to this document
//     explicitly states otherwise". lark states no such rule.
//     unpack.CheckPadding checks them, as Go's decoder skips CR and LF.
//   - A final group of 1, 3 or 6 characters: section 6 lists the five valid
//     final groups (8, 2, 4, 5 and 7 characters). Go's no-padding path ends its
//     loop on such a group without an error, so "MZX" gave empty bytes: data
//     was lost without a word.
//   - Pad bits that are not zero: section 3.5, "decoders MAY chose to reject an
//     encoding if the pad bits have not been set to zero". Go's base32 has no
//     Strict mode, so _PadBitsZero reads them.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b32decode
//   - 2026-09-24 00:53: a member of the base32 module
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 16:45: refuse excess padding, final groups of 1, 3 and 6, non-zero pad bits and
//     line breaks (C22)
//   - 2026-11-03 00:30: hands the decoded bytes over, with no copy of them (C37)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base32 and renamed _Decode to Decode, for
//     the tests in tests/ (C120b, Q315, Q314)
func Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	var (
		bare, ok = unpack.CheckPadding(
			text,
			spec.GROUP,
		)
		data []byte
	)

	if ok {
		data, ok = _Read(bare)
	}

	if !ok {
		return nil, common.Refused(
			common.Called(fn),
			text,
			unpack.ErrEncoded,
		)
	}

	return starlark.Bytes(unpack.Handover(data)), nil
}

// _Read decodes base32 text that has no padding, and reports whether the text
// was a whole encoding.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation, split from _Decode (C22)
//   - 2026-11-02 16:45: refuse a 0xFF byte, which Go takes for padding (C22)
//   - 2026-11-02 16:45: the constant for it is unexported (C22)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base32 (C120b, Q315)
func _Read(bare string) ([]byte, bool) {
	low := len(bare) * spec.BITS_PER_CHARACTER % spec.BITS_PER_OCTET
	if low >= spec.BITS_PER_CHARACTER {
		return nil, false
	}

	// Go's no-padding decoder sets its pad character to -1, which byte() turns
	// into 0xFF, so it takes a 0xFF byte for padding and stops reading. The
	// fuzz target found "22222\xff\xff\xff" accepted as "22222". 0xFF is outside
	// the alphabet, and RFC 4648 section 3.3 says to reject such characters.
	const PAD_LOOKALIKE = 0xFF // the byte Go's no-padding decoder takes for padding

	if strings.IndexByte(
		bare,
		PAD_LOOKALIKE,
	) >= 0 {
		return nil, false
	}

	data, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(bare)
	if err != nil {
		return nil, false
	}

	if low != 0 && !_PadBitsZero(
		bare,
		low,
	) {
		return nil, false
	}

	return data, true
}

// _PadBitsZero reports whether the low bits low of the last character of bare
// are zero. The character is in the alphabet, as the decoder has read it.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 section 3.5)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base32 (C120b, Q315)
func _PadBitsZero(
	bare string,
	low int,
) bool {
	value := strings.IndexByte(
		spec.ALPHABET,
		bare[len(bare)-1],
	)

	return value&(1<<low-1) == 0
}
