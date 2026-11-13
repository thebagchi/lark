package tests

import (
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
)

const (
	// WHO is the name a refusal would give. The benchmark text is all bits,
	// so no refusal comes.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	WHO = "bench"

	// BENCH is the name of the thread that each builtin benchmark calls on.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH = "bench"
)

var (
	// SIZES are the counts of bytes that each benchmark encodes or decodes.
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

// BenchmarkEncode measures Encode on data of each size. The bytes per second
// are bytes of data, the input.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkEncode(b *testing.B) {
	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				data := _Data(size)

				b.SetBytes(int64(len(data)))
				b.ReportAllocs()

				for b.Loop() {
					binary.Encode(data)
				}
			},
		)
	}
}

// BenchmarkDecode measures Decode on the encoding of data of each size. The
// bytes per second are characters of text, the input, eight to a byte of
// data.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkDecode(b *testing.B) {
	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				text := binary.Encode(_Data(size))

				b.SetBytes(int64(len(text)))
				b.ReportAllocs()

				for b.Loop() {
					_, err := binary.Decode(
						WHO,
						text,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _Data is size bytes that run through every byte value, so each value of a
// byte is in the input once size is at least 256.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
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

// BenchmarkFromBytes measures binary.from_bytes on bytes of each size, through
// the builtin, so the copy of the argument shows in the allocations. The bytes
// per second are bytes of data, the input.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
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
				b.SetBytes(int64(size))

				_Call(
					b,
					fn,
					starlark.Tuple{
						starlark.Bytes(_Data(size)),
					},
					nil,
				)
			},
		)
	}
}

// BenchmarkToBytes measures binary.to_bytes on the bits of data of each size,
// through the builtin. The bytes per second are characters of text, the input.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
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
				text := binary.Encode(_Data(size))

				b.SetBytes(int64(len(text)))

				_Call(
					b,
					fn,
					starlark.Tuple{
						starlark.String(text),
					},
					nil,
				)
			},
		)
	}
}

// BenchmarkFromInt measures binary.from_int on one number padded to a width of
// each size, in digits, through the builtin.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
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
				b.SetBytes(int64(size))

				_Call(
					b,
					fn,
					starlark.Tuple{
						starlark.MakeInt(48879),
						starlark.MakeInt(size),
					},
					nil,
				)
			},
		)
	}
}

// _Builtin is the member name of the binary module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
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

// _Call calls fn on args once per iteration, as a script's call reaches it,
// and ends the benchmark at the first error.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Call(
	b *testing.B,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) {
	b.Helper()

	thread := &starlark.Thread{
		Name: BENCH,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := fn.CallInternal(
			thread,
			args,
			kwargs,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
