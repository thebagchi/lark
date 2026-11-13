//go:build amd64 && !purego

package codec

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// The kernels are in kernel_amd64.s. Each one reads src[0:n] and writes only
// the destination bytes of that range, and each one ends with VZEROUPPER where
// it used a register wider than 128 bits. A wrapper below checks the sizes
// before the call, because the kernel does not.
//
// The stubs keep their underscore names: the assembly file declares them by
// these names, and a test calls none of them (C120b).

// _EncodeSSE writes the hex of src[0:n] to dst[0:2n]. It needs SSSE3 and n of
// 16 or more.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _EncodeSSE(
	dst *byte,
	src *byte,
	n int,
)

// _EncodeAVX2 is _EncodeSSE with AVX2 and 32 bytes a step.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _EncodeAVX2(
	dst *byte,
	src *byte,
	n int,
)

// _EncodeAVX512 is _EncodeSSE with AVX-512 F and BW, 32 bytes in and 64 digits
// out a step. It needs n of 32 or more.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _EncodeAVX512(
	dst *byte,
	src *byte,
	n int,
)

// _DecodeSSE writes to dst[0:n/2] the bytes of the digits src[0:n], and returns
// false when a digit is not hexadecimal. It needs SSSE3, an even n, and n of 32
// or more.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _DecodeSSE(
	dst *byte,
	src *byte,
	n int,
) bool

// _DecodeAVX2 is _DecodeSSE with AVX2.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _DecodeAVX2(
	dst *byte,
	src *byte,
	n int,
) bool

// _DecodeAVX512 is _DecodeSSE with AVX-512 F and BW. It needs n of 64 or more.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
//
//go:noescape
func _DecodeAVX512(
	dst *byte,
	src *byte,
	n int,
) bool

// _EncodeKernel runs the widest encode kernel that level and the size allow,
// and reports whether one ran. dst must hold 2*len(src) bytes.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _EncodeKernel(
	level cpu.Level,
	dst []byte,
	src []byte,
) bool {
	var size = len(src)

	if size == 0 || len(dst) < size*spec.DIGITS_PER_BYTE {
		return false
	}

	switch {
	case level >= cpu.LEVEL_AVX512 && size >= spec.ENCODE_AVX512_MIN:
		_EncodeAVX512(
			&dst[0],
			&src[0],
			size,
		)
	case level >= cpu.LEVEL_AVX2 && size >= spec.ENCODE_AVX2_MIN:
		_EncodeAVX2(
			&dst[0],
			&src[0],
			size,
		)
	case level >= cpu.LEVEL_SSE && size >= spec.ENCODE_SSE_MIN:
		_EncodeSSE(
			&dst[0],
			&src[0],
			size,
		)
	default:
		return false
	}

	return true
}

// _DecodeKernel runs the widest decode kernel that level and the size allow.
// It reports whether one ran, and whether every digit was valid. len(src) must
// be even and dst must hold len(src)/2 bytes.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _DecodeKernel(
	level cpu.Level,
	dst []byte,
	src []byte,
) (bool, bool) {
	var size = len(src)

	if size == 0 || size%spec.DIGITS_PER_BYTE != 0 || len(dst) < size/spec.DIGITS_PER_BYTE {
		return false, false
	}

	switch {
	case level >= cpu.LEVEL_AVX512 && size >= spec.DECODE_AVX512_MIN:
		return true, _DecodeAVX512(
			&dst[0],
			&src[0],
			size,
		)
	case level >= cpu.LEVEL_AVX2 && size >= spec.DECODE_AVX2_MIN:
		return true, _DecodeAVX2(
			&dst[0],
			&src[0],
			size,
		)
	case level >= cpu.LEVEL_SSE && size >= spec.DECODE_SSE_MIN:
		return true, _DecodeSSE(
			&dst[0],
			&src[0],
			size,
		)
	}

	return false, false
}
