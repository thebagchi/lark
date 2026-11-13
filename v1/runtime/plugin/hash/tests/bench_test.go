package tests

import (
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/spec"
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
	}
)

// BenchmarkSHA256 measures hash.sha256 on bytes of each size. The builtin is
// called directly, as a script's call reaches it, with no script around it.
// The bytes per second are bytes of data, the input.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkSHA256(b *testing.B) {
	fn := _Builtin(
		b,
		spec.SHA256,
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

// BenchmarkDigest measures each unkeyed digest on bytes of each size, the way
// BenchmarkSHA256 does, so the one-shot sums are measured for every algorithm.
//
// Revisions:
//   - 2026-11-03 22:20: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkDigest(b *testing.B) {
	for _, name := range []string{spec.MD5, spec.SHA1, spec.SHA256, spec.SHA512} {
		fn := _Builtin(
			b,
			name,
		)

		for _, size := range SIZES {
			b.Run(
				name+"/"+strconv.Itoa(size),
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
}

// _Builtin is the member name of the hash module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-10-29 00:57: initial creation
//   - 2026-11-03 22:20: takes a testing.TB, so a test finds a builtin too
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
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
