package tests

import (
	stdbase32 "encoding/base32"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/base32/internal/spec"
)

const (
	// BENCH is the name of the thread that each benchmark calls a builtin on.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH = "bench"
)

var (
	// SIZES are the counts of data bytes that each benchmark reads. 16 is the
	// size at which the plan's decode measurement was taken.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SIZES = []int{
		16,
		1024,
		65536,
	}
)

// BenchmarkEncode measures base32.encode on bytes of each size. The builtin is
// called directly, as a script's call reaches it, with no script around it.
// The bytes per second are bytes of data, the input.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkEncode(b *testing.B) {
	fn := _Builtin(
		b,
		spec.ENCODE,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				_Measure(
					b,
					fn,
					starlark.Bytes(_Data(size)),
					size,
				)
			},
		)
	}
}

// BenchmarkDecode measures base32.decode on the padded text of data of each
// size. The bytes per second are characters of text, the input.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkDecode(b *testing.B) {
	fn := _Builtin(
		b,
		spec.DECODE,
	)

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				text := stdbase32.StdEncoding.EncodeToString(_Data(size))

				_Measure(
					b,
					fn,
					starlark.String(text),
					len(text),
				)
			},
		)
	}
}

// _Measure calls fn on the one argument once per iteration and ends the
// benchmark at the first error.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Measure(
	b *testing.B,
	fn *starlark.Builtin,
	arg starlark.Value,
	input int,
) {
	b.Helper()

	var (
		args = starlark.Tuple{
			arg,
		}
		thread = &starlark.Thread{
			Name: BENCH,
		}
	)

	b.SetBytes(int64(input))
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
}

// _Builtin is the member name of the base32 module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-03 06:45: takes a testing.TB, so a test finds the builtin too (C37)
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
//   - 2026-11-02 00:00: initial creation
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
