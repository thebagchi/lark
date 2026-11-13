package tests

import (
	stdbase64 "encoding/base64"
	"strconv"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
)

var (
	// KERNEL_SIZES are the sizes, in data bytes, at which the kernels are
	// compared. They start below the least threshold (64 bytes) and end at 1 MiB, past
	// the highest (1024).
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	KERNEL_SIZES = []int{
		12, 16, 24, 28, 32, 48, 52, 64, 96, 128, 256, 1024, 4096, 65536, 1 << 20,
	}
)

// BenchmarkKernels_Encode measures one path writing base64 into a buffer that
// the benchmark owns, so no allocation is in the time. "stdlib" is the level
// with no kernel: encoding/base64 alone. The other names are the levels that
// the CPU has. The bytes per second are data bytes.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkKernels_Encode(b *testing.B) {
	for _, size := range KERNEL_SIZES {
		var (
			data = _Data(size)
			dst  = make(
				[]byte,
				stdbase64.StdEncoding.EncodedLen(size),
			)
		)

		for _, level := range Levels() {
			b.Run(
				LevelName(level)+"/"+strconv.Itoa(size),
				func(b *testing.B) {
					b.SetBytes(int64(size))

					for b.Loop() {
						EncodeAt(
							level,
							spec.ALPHABET_STD,
							dst,
							data,
						)
					}
				},
			)
		}
	}
}

// BenchmarkKernels_Decode measures one path reading the bare text of data of
// each size into a buffer that the benchmark owns. The bytes per second are
// characters of text.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkKernels_Decode(b *testing.B) {
	for _, size := range KERNEL_SIZES {
		var (
			text = _Bare(
				spec.ALPHABET_STD,
				_Data(size),
			)
			dst = make(
				[]byte,
				stdbase64.RawStdEncoding.DecodedLen(len(text)),
			)
		)

		for _, level := range Levels() {
			b.Run(
				LevelName(level)+"/"+strconv.Itoa(size),
				func(b *testing.B) {
					b.SetBytes(int64(len(text)))

					for b.Loop() {
						_, ok := DecodeAt(
							level,
							spec.ALPHABET_STD,
							dst,
							text,
						)
						if !ok {
							b.Fatal("refused")
						}
					}
				},
			)
		}
	}
}
