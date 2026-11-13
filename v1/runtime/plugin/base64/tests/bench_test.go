package tests

import (
	stdbase64 "encoding/base64"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
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
	// SIZES are the counts of data bytes that each benchmark reads.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SIZES = []int{
		16,
		1024,
		65536,
		1048576,
	}
)

// BenchmarkEncode measures base64.encode on bytes of each size. The builtin is
// called directly, as a script's call reaches it, with no script around it.
// The bytes per second are bytes of data, the input.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
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

// BenchmarkDecode measures base64.decode on the padded text of data of each
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
				var (
					text = stdbase64.StdEncoding.EncodeToString(_Data(size))
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

// _Builtin is the member name of the base64 module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
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
