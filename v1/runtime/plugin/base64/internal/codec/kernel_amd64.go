//go:build amd64 && !purego

package codec

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// The kernels are in kernel_amd64.s. An encode kernel reads src[0:n] and writes
// dst; a decode kernel reads src[0:n] and writes dst[0:room]. Each one checks
// its own bounds in its loop test (see the header of the .s file), and ends
// with VZEROUPPER where it used a register wider than 128 bits.
//
// The stubs keep their underscore names: the assembly file declares them by
// these names, and a test calls none of them (C120b).

// _EncodeSSE encodes whole blocks of src[0:run.Size] to dst, 12 bytes a step,
// and returns how many bytes of src it handled. It needs SSSE3.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _EncodeSSE(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _EncodeAVX2 is _EncodeSSE with AVX2 and 24 bytes a step.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _EncodeAVX2(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _EncodeAVX512 is _EncodeSSE with AVX-512 F and BW and 48 bytes a step.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _EncodeAVX512(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _DecodeSSE decodes whole blocks of src[0:run.Size] to dst[0:run.Room], 16
// characters a step, and returns how many characters it handled, or -1 for a
// character that is not in the alphabet. It needs SSSE3.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _DecodeSSE(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _DecodeAVX2 is _DecodeSSE with AVX2 and 32 characters a step. It stays by the
// user's decision (Q93: SSE, AVX2 and AVX-512 are all required). Measured (C51
// fix round 2): alone, it is about twice as fast as the SSE kernel from 64 KiB
// (6.5 us against 14.9 us for 64 KiB, 119 us against 194 us for 1 MiB, CPU
// time per call). In a whole call of a builtin, which allocates its result, it
// is within the noise of the SSE kernel from 160 to 4096 bytes (benchstat
// p 0.096 to 0.91, 30 rounds): no whole-call gain is shown.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-04 18:14: doc comment states the measured result (C51 fix 2)
//   - 2026-11-04 18:48: no whole-call gain is claimed (C51 fix 3)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _DecodeAVX2(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _DecodeAVX512 is _DecodeSSE with AVX-512 F and BW and 64 characters a step.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
//
//go:noescape
func _DecodeAVX512(
	dst *byte,
	src *byte,
	run *_Run,
) int

// _EncodeKernel runs the encode kernels that the level and the size allow, the
// widest first, each on what the one before left. It returns how many bytes of
// src it handled, always a multiple of 3. dst must hold the encoding of the
// whole of src.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
func _EncodeKernel(
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	if len(dst) < len(src)/spec.GROUP_BYTES*spec.GROUP {
		return 0
	}

	var done int

	for _, level := range spec.LEVELS {
		if coding.Level >= level {
			done += _EncodeRun(
				level,
				coding,
				dst[done/spec.GROUP_BYTES*spec.GROUP:],
				src[done:],
			)
		}
	}

	return done
}

// _EncodeRun calls the encode kernel of the level, when src has at least the
// least size that the coding gives it. The kernels are called by name, not
// through a func value: a call through a value would make the compiler move the
// _Run to the heap, one allocation on every call of a builtin.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
func _EncodeRun(
	level cpu.Level,
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	if len(dst) == 0 {
		return 0
	}

	run := _Run{
		Size:     len(src),
		Room:     len(dst),
		Alphabet: coding.Alphabet,
	}

	switch {
	case level == cpu.LEVEL_AVX512 && len(src) >= coding.Sizes.EncodeAVX512:
		return _EncodeAVX512(
			&dst[0],
			&src[0],
			&run,
		)
	case level == cpu.LEVEL_AVX2 && len(src) >= coding.Sizes.EncodeAVX2:
		return _EncodeAVX2(
			&dst[0],
			&src[0],
			&run,
		)
	case level == cpu.LEVEL_SSE && len(src) >= coding.Sizes.EncodeSSE:
		return _EncodeSSE(
			&dst[0],
			&src[0],
			&run,
		)
	}

	return 0
}

// _DecodeKernel runs the decode kernels that the level and the size allow, the
// widest first, each on what the one before left. It returns how many
// characters of src it handled, always a multiple of 4, or -1 when one was not
// in the alphabet. dst must hold the decoding of the whole of src.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
func _DecodeKernel(
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	var done int

	for _, level := range spec.LEVELS {
		if coding.Level < level {
			continue
		}

		handled := _DecodeRun(
			level,
			coding,
			dst[done/spec.GROUP*spec.GROUP_BYTES:],
			src[done:],
		)
		if handled < 0 {
			return -1
		}

		done += handled
	}

	return done
}

// _DecodeRun calls the decode kernel of the level, when src has at least the
// least size that the coding gives it, with all of dst as room. It returns what
// the kernel returns.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 (C120b, Q315)
func _DecodeRun(
	level cpu.Level,
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	if len(dst) == 0 {
		return 0
	}

	run := _Run{
		Size:     len(src),
		Room:     len(dst),
		Alphabet: coding.Alphabet,
	}

	switch {
	case level == cpu.LEVEL_AVX512 && len(src) >= coding.Sizes.DecodeAVX512:
		return _DecodeAVX512(
			&dst[0],
			&src[0],
			&run,
		)
	case level == cpu.LEVEL_AVX2 && len(src) >= coding.Sizes.DecodeAVX2:
		return _DecodeAVX2(
			&dst[0],
			&src[0],
			&run,
		)
	case level == cpu.LEVEL_SSE && len(src) >= coding.Sizes.DecodeSSE:
		return _DecodeSSE(
			&dst[0],
			&src[0],
			&run,
		)
	}

	return 0
}
