package hex_test

import (
	stdhex "encoding/hex"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
)

const (
	// BENCH is the name of the thread that each benchmark calls a builtin on.
	BENCH = "bench"
)

var (
	// SIZES are the counts of data bytes that each benchmark encodes or
	// decodes.
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
func BenchmarkFromBytes(b *testing.B) {
	fn := _Builtin(
		b,
		hex.FROM_BYTES,
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
				b.ResetTimer()

				for range b.N {
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
func BenchmarkToBytes(b *testing.B) {
	fn := _Builtin(
		b,
		hex.TO_BYTES,
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
				b.ResetTimer()

				for range b.N {
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
func _Builtin(
	b *testing.B,
	name string,
) *starlark.Builtin {
	b.Helper()

	env, err := plugin.Environment(plugin.DEFAULT)
	if err != nil {
		b.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env[hex.NAME].(*starlarkstruct.Module)
	if !ok {
		b.Fatalf(
			"%s is not a module",
			hex.NAME,
		)
	}

	fn, ok := module.Members[name].(*starlark.Builtin)
	if !ok {
		b.Fatalf(
			"%s.%s is not a builtin",
			hex.NAME,
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
