package tests

import (
	"encoding/base64"

	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// This file is compiled for the tests only. It gives the external test package
// the hooks that choose a path, so the SSE, AVX2 and portable paths each run on
// a CPU that has AVX-512. It adds no name to the package a program imports.

// Levels are the paths that this CPU and build can run, narrowest first. The
// index is what EncodeAt and DecodeAt take. LevelName names it.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func Levels() []int {
	var out []int

	for level := cpu.LEVEL_SWAR; level <= cpu.Detect(); level++ {
		out = append(
			out,
			int(level),
		)
	}

	return out
}

// LevelName is the name of a level, for a subtest name. The lowest level is the
// standard library alone, as the portable path of this package is.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func LevelName(level int) string {
	switch cpu.Level(level) {
	case cpu.LEVEL_SWAR:
		return "stdlib"
	case cpu.LEVEL_SSE:
		return "sse"
	case cpu.LEVEL_AVX2:
		return "avx2"
	case cpu.LEVEL_AVX512:
		return "avx512"
	}

	return "unknown"
}

// Detected is the highest level that this CPU supports.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func Detected() int {
	return int(cpu.Detect())
}

// _Writing is the encoding that the encoding builtin of an alphabet uses.
// The standard alphabet is padded, and the URL one is not.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Writing(alphabet int) *base64.Encoding {
	if alphabet == spec.ALPHABET_URL {
		return base64.RawURLEncoding
	}

	return base64.StdEncoding
}

// _Reading is the encoding that the decoding builtin of an alphabet uses: no
// padding, in Strict mode.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Reading(alphabet int) *base64.Encoding {
	if alphabet == spec.ALPHABET_URL {
		return base64.RawURLEncoding.Strict()
	}

	return base64.RawStdEncoding.Strict()
}

// _Make is the coding of the alphabet given, at the level and the sizes given.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Make(
	level int,
	alphabet int,
	sizes *spec.Sizes,
	encoding *base64.Encoding,
) *codec.Coding {
	return &codec.Coding{
		Level:    cpu.Level(level),
		Sizes:    sizes,
		Alphabet: alphabet,
		Encoding: encoding,
	}
}

// SMALLEST are the least sizes that a block allows, so that a test runs each
// kernel on every size that it can, not only above the sizes that ship.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	SMALLEST = spec.Sizes{
		EncodeSSE:    1,
		EncodeAVX2:   1,
		EncodeAVX512: 1,
		DecodeSSE:    1,
		DecodeAVX2:   1,
		DecodeAVX512: 1,
	}
)

// EncodeAt is the encode path of the level given, as base64.encode (alphabet
// ALPHABET_STD) or base64.urlencode (ALPHABET_URL) has it, with the smallest
// sizes, on a buffer the test owns, so the test can watch the bytes round the
// destination. dst must hold the whole encoding.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func EncodeAt(
	level int,
	alphabet int,
	dst []byte,
	src []byte,
) {
	codec.Encode(
		_Make(
			level,
			alphabet,
			&SMALLEST,
			_Writing(alphabet),
		),
		dst,
		src,
	)
}

// DecodeAt is the decode path of the level given, with the smallest sizes, over
// text with no padding. It returns the count of bytes written and whether the
// text was accepted.
//
// Revisions:
//   - 2026-11-03 22:50: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func DecodeAt(
	level int,
	alphabet int,
	dst []byte,
	src []byte,
) (int, bool) {
	return codec.Decode(
		_Make(
			level,
			alphabet,
			&SMALLEST,
			_Reading(alphabet),
		),
		dst,
		src,
	)
}

// EncodeShipped is EncodeAt with the sizes that ship: the path a script
// reaches.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func EncodeShipped(
	level int,
	alphabet int,
	dst []byte,
	src []byte,
) {
	codec.Encode(
		_Make(
			level,
			alphabet,
			&spec.SIZES,
			_Writing(alphabet),
		),
		dst,
		src,
	)
}

// DecodeShipped is DecodeAt with the sizes that ship.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func DecodeShipped(
	level int,
	alphabet int,
	dst []byte,
	src []byte,
) (int, bool) {
	return codec.Decode(
		_Make(
			level,
			alphabet,
			&spec.SIZES,
			_Reading(alphabet),
		),
		dst,
		src,
	)
}
