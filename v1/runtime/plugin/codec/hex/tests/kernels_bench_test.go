package tests

import (
	stdhex "encoding/hex"
	"strconv"
	"testing"
)

var (
	// KERNEL_SIZES are the sizes, in data bytes, at which the kernels are
	// compared. They run from below every threshold to 1 MiB.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	KERNEL_SIZES = []int{4, 8, 16, 24, 32, 48, 64, 96, 128, 256, 1024, 65536, 1 << 20}
)

// BenchmarkKernels_Encode measures one path writing hex into a buffer that the
// benchmark owns, so no allocation is in the time. "table" is the loop that
// Encode had before C50. The other names are the levels the CPU has.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkKernels_Encode(b *testing.B) {
	for _, size := range KERNEL_SIZES {
		var (
			data = _Data(size)
			dst  = make(
				[]byte,
				size*2,
			)
		)

		b.Run(
			"table/"+strconv.Itoa(size),
			func(b *testing.B) {
				b.SetBytes(int64(size))

				for b.Loop() {
					EncodeTable(
						dst,
						data,
					)
				}
			},
		)

		for _, level := range Levels() {
			b.Run(
				LevelName(level)+"/"+strconv.Itoa(size),
				func(b *testing.B) {
					b.SetBytes(int64(size))

					for b.Loop() {
						EncodeAt(
							level,
							dst,
							data,
						)
					}
				},
			)
		}
	}
}

// BenchmarkKernels_Decode is BenchmarkKernels_Encode for decode. "stdlib" is
// encoding/hex.Decode, which to_bytes called before C50. The size is the count
// of digits.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkKernels_Decode(b *testing.B) {
	for _, size := range KERNEL_SIZES {
		var (
			digits = []byte(stdhex.EncodeToString(_Data(size / 2)))
			dst    = make(
				[]byte,
				size/2,
			)
		)

		b.Run(
			"stdlib/"+strconv.Itoa(size),
			func(b *testing.B) {
				b.SetBytes(int64(size))

				for b.Loop() {
					_, _ = stdhex.Decode(
						dst,
						digits,
					)
				}
			},
		)

		for _, level := range Levels() {
			b.Run(
				LevelName(level)+"/"+strconv.Itoa(size),
				func(b *testing.B) {
					b.SetBytes(int64(size))

					for b.Loop() {
						DecodeAt(
							level,
							dst,
							digits,
						)
					}
				},
			)
		}
	}
}
