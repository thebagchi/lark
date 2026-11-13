package tests

import (
	"context"
	"strconv"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

const (
	// KEYS is the count of tuple keys in the dict of BenchmarkDeepCopy_TupleKeys.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	KEYS = 1000

	// SCALARS is the count of numbers in the scalar tuple.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SCALARS = 8
)

// BenchmarkDeepCopy_TupleKeys measures Copy of three values: a dict of KEYS
// keys that are tuples of 2 numbers, a tuple of SCALARS numbers, and a tuple
// that holds a list. The first two hold nothing a copy needs to change; the
// third must be copied.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from B-P2 of
//     poc/research/plugins_runtime.md
//   - 2026-11-02 16:14: Copy takes a context (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkDeepCopy_TupleKeys(b *testing.B) {
	cases := []struct {
		name  string
		value starlark.Value
	}{
		{
			name:  "dict of " + strconv.Itoa(KEYS) + " tuple keys",
			value: _TupleKeys(b),
		},
		{
			name:  "tuple of scalars",
			value: _ScalarTuple(),
		},
		{
			name:  "tuple holding a list",
			value: _ListTuple(),
		},
	}

	for _, tc := range cases {
		b.Run(
			tc.name,
			func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					_, err := deep.Copy(
						context.Background(),
						tc.value,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _TupleKeys is a dict of KEYS keys, each a tuple of 2 numbers, with a number
// as the value of each.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TupleKeys(b *testing.B) *starlark.Dict {
	b.Helper()

	dict := starlark.NewDict(KEYS)

	for idx := range KEYS {
		err := dict.SetKey(
			starlark.Tuple{
				starlark.MakeInt(idx),
				starlark.MakeInt(idx + 1),
			},
			starlark.MakeInt(idx),
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	return dict
}

// _ScalarTuple is a tuple of SCALARS numbers.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _ScalarTuple() starlark.Tuple {
	tuple := make(
		starlark.Tuple,
		SCALARS,
	)

	for idx := range tuple {
		tuple[idx] = starlark.MakeInt(idx)
	}

	return tuple
}

// _ListTuple is a tuple of a number, a list of 2 numbers, and a string.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _ListTuple() starlark.Tuple {
	return starlark.Tuple{
		starlark.MakeInt(1),
		starlark.NewList([]starlark.Value{
			starlark.MakeInt(2),
			starlark.MakeInt(3),
		}),
		starlark.String("x"),
	}
}
