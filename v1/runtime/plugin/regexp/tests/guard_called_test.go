package tests

// This file tests that the helpers of the regexp plugin that name their builtin
// in an error do not panic when the builtin is nil. No script can build that
// input, because the wrapper of NewBuiltin always passes its own builtin, so
// the tests call the helpers directly (C102, Q213).
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_called_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/regexp/internal/match"
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
			"_Quote",
			func() error {
				_, err := match.Quote(
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
			"_Sub",
			func() error {
				_, err := new(match.Regexp).Sub(
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
			"_Split",
			func() error {
				_, err := new(match.Regexp).Split(
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
			"_Given",
			func() error {
				_, _, err := new(match.Regexp).Given(
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
			"_Sub bad pattern",
			func() error {
				_, err := new(match.Regexp).Sub(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.String("("),
						starlark.String(""),
						starlark.String(""),
					}, nil,
				)

				return err
			},
			"builtin got \"(\": not a pattern this reads: error parsing " +
				"regexp: missing closing ): `(`",
			false,
		},
		{
			"_Split bad pattern",
			func() error {
				_, err := new(match.Regexp).Split(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.String("("),
						starlark.String(""),
					}, nil,
				)

				return err
			},
			"builtin got \"(\": not a pattern this reads: error parsing " +
				"regexp: missing closing ): `(`",
			false,
		},
		{
			"_Given bad pattern",
			func() error {
				_, _, err := new(match.Regexp).Given(
					nil,
					starlark.Tuple{
						starlark.String("("),
						starlark.String(""),
					}, nil,
				)

				return err
			},
			"builtin got \"(\": not a pattern this reads: error parsing " +
				"regexp: missing closing ): `(`",
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
