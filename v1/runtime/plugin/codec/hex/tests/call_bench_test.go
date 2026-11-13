package tests

import (
	stdhex "encoding/hex"
	"strconv"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
)

var (
	// CALL_SIZES are the counts of data bytes of the whole-call benchmarks:
	// the sizes the C50 report names.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	CALL_SIZES = []int{16, 1024, 65536, 1 << 20}
)

// BenchmarkCall measures hex.from_bytes and hex.to_bytes as a script calls them,
// on data of CALL_SIZES, with allocations. It uses only the plugin's public
// surface, so the same file runs on a tree from before C50.
//
// Revisions:
//   - 2026-11-03 01:23: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCall(b *testing.B) {
	for _, name := range []string{
		spec.FROM_BYTES,
		spec.TO_BYTES,
	} {
		fn := _Builtin(
			b,
			name,
		)

		for _, size := range CALL_SIZES {
			b.Run(
				name+"/"+strconv.Itoa(size),
				func(b *testing.B) {
					var (
						data   = _Data(size)
						args   = starlark.Tuple{starlark.Bytes(data)}
						thread = &starlark.Thread{
							Name: BENCH,
						}
					)

					if name == spec.TO_BYTES {
						text := stdhex.EncodeToString(data)
						args = starlark.Tuple{starlark.String(text)}
					}

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
