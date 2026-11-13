package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/spec"
)

// TestNamed_MatchesRegularCallsOnly gives Named calls to a builtin of three
// parameters, the first two required, and wants the slots of a regular call
// and a refusal of every other.
//
// Success: a row that wants a match sees each value in its slot, and a nil
// slot for a parameter the call left out.
//
// Revisions:
//   - 2026-11-04 00:36: initial creation (C38c)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:54: rows for a keyword entry shorter than two (C117, B246)
func TestNamed_MatchesRegularCallsOnly(t *testing.T) {
	var (
		names = []string{
			"a",
			"b",
			"c" + spec.OPTIONAL,
		}
		one   = starlark.MakeInt(1)
		two   = starlark.MakeInt(2)
		three = starlark.MakeInt(3)

		keyword = func(
			name string,
			value starlark.Value,
		) starlark.Tuple {
			return starlark.Tuple{
				starlark.String(name),
				value,
			}
		}

		tests = []struct {
			name   string
			args   starlark.Tuple
			kwargs []starlark.Tuple
			want   []starlark.Value
		}{
			{
				name: "all positional",
				args: starlark.Tuple{
					one,
					two,
					three,
				},
				want: []starlark.Value{
					one,
					two,
					three,
				},
			},
			{
				name: "optional left out",
				args: starlark.Tuple{
					one,
					two,
				},
				want: []starlark.Value{
					one,
					two,
					nil,
				},
			},
			{
				name: "all keyword, in any order",
				kwargs: []starlark.Tuple{
					keyword(
						"c",
						three,
					),
					keyword(
						"a",
						one,
					),
					keyword(
						"b",
						two,
					),
				},
				want: []starlark.Value{
					one,
					two,
					three,
				},
			},
			{
				name: "positional then keyword",
				args: starlark.Tuple{
					one,
				},
				kwargs: []starlark.Tuple{
					keyword(
						"b",
						two,
					),
				},
				want: []starlark.Value{
					one,
					two,
					nil,
				},
			},
			{
				name: "a required one missing",
				args: starlark.Tuple{
					one,
				},
			},
			{
				name: "a required one missing, an optional one given",
				kwargs: []starlark.Tuple{
					keyword(
						"a",
						one,
					),
					keyword(
						"c",
						three,
					),
				},
			},
			{
				name: "too many positional",
				args: starlark.Tuple{
					one,
					two,
					three,
					one,
				},
			},
			{
				name: "unknown keyword",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					keyword(
						"zz",
						three,
					),
				},
			},
			{
				name: "given by position and by keyword",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					keyword(
						"a",
						three,
					),
				},
			},
			{
				name: "given by keyword twice",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					keyword(
						"c",
						three,
					),
					keyword(
						"c",
						three,
					),
				},
			},
			{
				name: "keyword entry with no values",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					{},
				},
			},
			{
				name: "keyword entry with a name only",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					{
						starlark.String("c"),
					},
				},
			},
			{
				name: "keyword that is not a string",
				args: starlark.Tuple{
					one,
					two,
				},
				kwargs: []starlark.Tuple{
					{
						one,
						three,
					},
				},
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var slots [3]starlark.Value

				ok := unpack.Named(
					test.args,
					test.kwargs,
					names,
					slots[:],
				)
				if ok != (test.want != nil) {
					t.Fatalf(
						"Named answered %t, want %t",
						ok,
						test.want != nil,
					)
				}

				for idx, want := range test.want {
					if slots[idx] != want {
						t.Fatalf(
							"slot %d is %v, want %v",
							idx,
							slots[idx],
							want,
						)
					}
				}
			},
		)
	}
}

// TestNamed_SlotsShorterThanNamesIsNotRegular proves Named answers false and
// does not panic when slots has fewer places than names, which the doc says
// the caller must not do (B246, Q65, Q206).
//
// Success: false for slots of length 0 and 1 with names of length 2, for a
// call by keyword and by position.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B246)
func TestNamed_SlotsShorterThanNamesIsNotRegular(t *testing.T) {
	var (
		names = []string{"a", "b"}
		one   = starlark.MakeInt(1)
		two   = starlark.MakeInt(2)
	)

	for size := range 2 {
		var (
			slots = make(
				[]starlark.Value,
				size,
			)
			ok = unpack.Named(
				nil,
				[]starlark.Tuple{
					{
						starlark.String("b"),
						two,
					},
				},
				names,
				slots,
			)
		)
		if ok {
			t.Fatalf(
				"keyword call with %d slots answered true",
				size,
			)
		}

		ok = unpack.Named(
			starlark.Tuple{
				one,
				two,
			},
			nil,
			names,
			slots,
		)
		if ok {
			t.Fatalf(
				"positional call with %d slots answered true",
				size,
			)
		}
	}
}
