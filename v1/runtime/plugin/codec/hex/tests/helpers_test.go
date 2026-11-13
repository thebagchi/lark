package tests

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// This file is compiled for the tests only. It gives the external test package
// the hooks that choose a path, so the SSE, AVX2 and portable paths each run on
// a CPU that has AVX-512. It adds no name to the package a program imports.

// Levels are the paths that this CPU and build can run, narrowest first. The
// index is what EncodeAt and DecodeAt take. LevelName names it.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
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

// LevelName is the name of a level, for a subtest name.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func LevelName(level int) string {
	switch cpu.Level(level) {
	case cpu.LEVEL_SWAR:
		return "swar"
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
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func Detected() int {
	return int(cpu.Detect())
}

// EncodeTable is the plain loop, the reference that the other paths must equal.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func EncodeTable(
	dst []byte,
	src []byte,
) {
	codec.EncodeTable(
		dst,
		src,
	)
}

// EncodeAt is the encode path of the level given, on a buffer the test owns, so
// the test can watch the bytes round the destination.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func EncodeAt(
	level int,
	dst []byte,
	src []byte,
) {
	codec.EncodeAt(
		cpu.Level(level),
		dst,
		src,
	)
}

// DecodeAt is the decode path of the level given.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func DecodeAt(
	level int,
	dst []byte,
	src []byte,
) bool {
	return codec.DecodeAt(
		cpu.Level(level),
		dst,
		src,
	)
}

// EncodeSWAR is the portable word path, whatever the level.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func EncodeSWAR(
	dst []byte,
	src []byte,
) {
	codec.EncodeSWAR(
		dst,
		src,
	)
}
