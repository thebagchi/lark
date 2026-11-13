package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// TestBuiltins_AllocateNoMoreThanTheResult counts the allocations of each call
// through the builtin, and wants no more than the result itself takes: the
// bytes of to_bytes and the interface that holds them, and the int of
// from_bytes. A local of the argument reading that goes to the heap, a copy of
// the data, or a big.Int for a word is one more.
//
// Success: every row is at or under its ceiling. The ceilings are the counts
// measured at change C38c: from_bytes 0 for two bytes, 2 for eight; to_bytes 2.
//
// Revisions:
//   - 2026-11-04 00:51: initial creation (C38c, plan D2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBuiltins_AllocateNoMoreThanTheResult(t *testing.T) {
	const RUNS = 100

	var (
		two   = starlark.Bytes("\x01\x02")
		eight = starlark.Bytes("\x01\x02\x03\x04\x05\x06\x07\x08")
		tests = []struct {
			name    string
			builtin string
			args    starlark.Tuple
			kwargs  []starlark.Tuple
			most    float64
		}{
			{
				name:    "from_bytes of two bytes",
				builtin: "from_bytes",
				args: starlark.Tuple{
					two,
				},
				most: 0,
			},
			{
				name:    "from_bytes of two bytes, little",
				builtin: "from_bytes",
				args: starlark.Tuple{
					two,
					starlark.String(common.LITTLE),
					starlark.True,
				},
				most: 0,
			},
			{
				name:    "from_bytes of two bytes, by keyword",
				builtin: "from_bytes",
				args: starlark.Tuple{
					two,
				},
				kwargs: []starlark.Tuple{
					{
						starlark.String("order"),
						starlark.String(common.LITTLE),
					},
				},
				most: 0,
			},
			{
				name:    "from_bytes of eight bytes",
				builtin: "from_bytes",
				args: starlark.Tuple{
					eight,
				},
				most: 2,
			},
			{
				name:    "to_bytes of two bytes",
				builtin: "to_bytes",
				args: starlark.Tuple{
					starlark.MakeInt(300),
					starlark.MakeInt(2),
				},
				most: 2,
			},
			{
				name:    "to_bytes of eight bytes, signed, by keyword",
				builtin: "to_bytes",
				args: starlark.Tuple{
					starlark.MakeInt(-300),
					starlark.MakeInt(8),
				},
				kwargs: []starlark.Tuple{
					{
						starlark.String("signed"),
						starlark.True,
					},
				},
				most: 2,
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						test.builtin,
					)
					thread = &starlark.Thread{
						Name: BENCH,
					}
					failed error
				)

				got := testing.AllocsPerRun(
					RUNS,
					func() {
						_, err := fn.CallInternal(
							thread,
							test.args,
							test.kwargs,
						)
						if err != nil {
							failed = err
						}
					},
				)

				if failed != nil {
					t.Fatal(failed)
				}

				if got > test.most {
					t.Fatalf(
						"%s made %.0f allocations, want at most %.0f",
						test.name,
						got,
						test.most,
					)
				}
			},
		)
	}
}
