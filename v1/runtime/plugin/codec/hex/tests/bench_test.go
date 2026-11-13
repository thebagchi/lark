package tests

import (
	stdhex "encoding/hex"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
)

const (
	// BENCH is the name of the thread that each benchmark calls a builtin on.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH = "bench"
)

var (
	// SIZES are the counts of data bytes that each benchmark encodes or
	// decodes.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SIZES = []int{
		16,
		1024,
		65536,
	}
)

// BenchmarkFromBytes measures hex.from_bytes on bytes of each size. The
// builtin is called directly, as a script's call reaches it, with no script
// around it. The bytes per second are bytes of data, the input.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkFromBytes(b *testing.B) {
	fn := _Builtin(
		b,
		spec.FROM_BYTES,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				var (
					args = starlark.Tuple{
						starlark.Bytes(_Data(size)),
					}
					thread = &starlark.Thread{
						Name: BENCH,
					}
				)

				b.SetBytes(int64(size))
				b.ReportAllocs()

				for b.Loop() {
					_, err := fn.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkToBytes measures hex.to_bytes on the hex of data of each size. The
// bytes per second are characters of text, the input, two to a byte of data.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkToBytes(b *testing.B) {
	fn := _Builtin(
		b,
		spec.TO_BYTES,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				var (
					text = stdhex.EncodeToString(_Data(size))
					args = starlark.Tuple{
						starlark.String(text),
					}
					thread = &starlark.Thread{
						Name: BENCH,
					}
				)

				b.SetBytes(int64(len(text)))
				b.ReportAllocs()

				for b.Loop() {
					_, err := fn.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkFromBinary measures hex.from_binary on the bits of data of each
// size. The bytes per second are characters of text, the input, eight to a
// byte of data.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkFromBinary(b *testing.B) {
	fn := _Builtin(
		b,
		spec.FROM_BINARY,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				var (
					text = binary.Encode(_Data(size))
					args = starlark.Tuple{
						starlark.String(text),
					}
					thread = &starlark.Thread{
						Name: BENCH,
					}
				)

				b.SetBytes(int64(len(text)))
				b.ReportAllocs()

				for b.Loop() {
					_, err := fn.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkFromInt measures hex.from_int on one number padded to a width of
// each size, in digits.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkFromInt(b *testing.B) {
	fn := _Builtin(
		b,
		spec.FROM_INT,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				var (
					args = starlark.Tuple{
						starlark.MakeInt(48879),
						starlark.MakeInt(size),
					}
					thread = &starlark.Thread{
						Name: BENCH,
					}
				)

				b.SetBytes(int64(size))
				b.ReportAllocs()

				for b.Loop() {
					_, err := fn.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _Builtin is the member name of the hex module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-02 16:55: takes a testing.TB, so a test finds the builtin too (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Builtin(
	tb testing.TB,
	name string,
) *starlark.Builtin {
	tb.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		tb.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env[spec.NAME].(*starlarkstruct.Module)
	if !ok {
		tb.Fatalf(
			"%s is not a module",
			spec.NAME,
		)
	}

	fn, ok := module.Members[name].(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"%s.%s is not a builtin",
			spec.NAME,
			name,
		)
	}

	return fn
}

// _Data is size bytes that run through every byte value, so each value of a
// byte is in the input once size is at least 256.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Data(size int) []byte {
	data := make(
		[]byte,
		size,
	)

	for idx := range data {
		data[idx] = byte(idx)
	}

	return data
}
