package tests

// This file tests that the helpers of the jsonpath plugin that name their
// builtin in an error do not panic when the builtin is nil. No script can build
// that input, because the wrapper of NewBuiltin always passes its own builtin,
// so the tests call the helpers directly (C102, Q213).
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_called_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/patch"
)

// TestGuard_CalledNilBuiltin proves each helper that names its builtin in an
// error does not panic when the builtin is nil.
//
// Case from Q213 (C102), same as C101 for core.
//
// A row marked quiet checks only that the call does not panic. Every other row
// checks the error text exactly, and an empty want means no error.
//
// The rows that follow the first group prove a nil builtin is named "builtin" at
// the sites that sit past the argument check: each case passes valid
// arguments and the state its site needs, so the call reaches the site.
//
// Case from the C102 test round 1: a nil case that stops at an earlier
// statement leaves the site unproved.
//
// Revisions:
//   - 2026-11-06 21:28: initial creation (C102)
//   - 2026-11-06 22:10: initial creation (C102 fix round 2)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-08 22:46: merged with TestGuard_CalledNilBuiltinReached into one table (C65b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_CalledNilBuiltin(t *testing.T) {
	tests := []struct {
		name  string
		call  func() error
		want  string
		quiet bool
	}{
		{
			"_PatchJSON",
			func() error {
				_, err := patch.Apply(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_ExtractJSON",
			func() error {
				_, err := patch.ExtractJSON(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_MatchJSON",
			func() error {
				_, err := patch.MatchJSON(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_LenJSON",
			func() error {
				_, err := patch.LenJSON(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_FindKey",
			func() error {
				_, err := patch.FindKey(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_PatchArgs",
			func() error {
				_, _, err := patch.Unpack(
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_MatchArgs",
			func() error {
				_, _, _, err := patch.MatchArgs(
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_Target",
			func() error {
				_, _, err := patch.Target(
					nil,
					nil,
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_PatchJSON operation not a dict",
			func() error {
				_, err := patch.Apply(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.NewDict(0),
						starlark.NewList([]starlark.Value{starlark.None}),
					}, nil,
				)

				return err
			},
			"builtin: operation 0 is NoneType: unknown patch operation: " +
				"malformed patch operation",
			false,
		},
		{
			"_PatchJSON operation refused",
			func() error {
				_, err := patch.Apply(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.NewDict(0),
						starlark.NewList([]starlark.Value{
							starlark.NewDict(0),
						}),
					}, nil,
				)

				return err
			},
			"builtin: operation 0: \"op\": patch operation is missing a field: " +
				"malformed patch operation",
			false,
		},
		{
			"_ExtractJSON bad pointer",
			func() error {
				_, err := patch.ExtractJSON(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.NewDict(0),
						starlark.String("x"),
					}, nil,
				)

				return err
			},
			"builtin: \"x\" does not start with \"/\": not a json pointer",
			false,
		},
		{
			"_MatchJSON bad pointer",
			func() error {
				_, err := patch.MatchJSON(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.NewDict(0),
						starlark.String("x"),
						starlark.String("v"),
					}, nil,
				)

				return err
			},
			"builtin: \"x\" does not start with \"/\": not a json pointer",
			false,
		},
		{
			"_MatchJSON values refused",
			func() error {
				_, err := patch.MatchJSON(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						_Cyclic(),
						starlark.String(""),
						_Cyclic(),
					}, nil,
				)

				return err
			},
			"builtin: comparison exceeded maximum recursion depth",
			false,
		},
		{
			"_LenJSON bad pointer",
			func() error {
				_, err := patch.LenJSON(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.NewDict(0),
						starlark.String("x"),
					}, nil,
				)

				return err
			},
			"builtin: \"x\" does not start with \"/\": not a json pointer",
			false,
		},
		{
			"_LenJSON no length",
			func() error {
				_, err := patch.LenJSON(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.MakeInt(1),
						starlark.String(""),
					}, nil,
				)

				return err
			},
			"builtin: int has no length: cannot be indexed",
			false,
		},
		{
			"_FindKey cyclic document",
			func() error {
				_, err := patch.FindKey(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						_Cyclic(),
						starlark.String("k"),
					}, nil,
				)

				return err
			},
			"builtin: cycle in value",
			false,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				defer func() {
					if raised := recover(); raised != nil {
						t.Fatalf(
							"panicked: %v",
							raised,
						)
					}
				}()

				var (
					err = test.call()
					got string
				)
				if err != nil {
					got = err.Error()
				}

				if !test.quiet && got != test.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// _Cyclic is a list that holds itself, which a walk of the value refuses.
//
// Revisions:
//   - 2026-11-06 22:30: initial creation (C102 fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Cyclic() *starlark.List {
	list := starlark.NewList(nil)
	_ = list.Append(list)

	return list
}
