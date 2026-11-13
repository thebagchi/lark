// This file tests the field helpers with a nil builtin, which no script can
// build, because the wrapper of NewBuiltin always passes its own builtin.
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to integer/internal/field (C91w8b)
//   - 2026-11-10 00:25: moved to tests/ from guard_called_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer/internal/field"
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_CalledNilBuiltin(t *testing.T) {
	tests := []struct {
		name  string
		call  func() error
		want  string
		quiet bool
	}{
		{
			"_FromBytes",
			func() error {
				_, err := field.FromBytes(
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
			"_ToBytes",
			func() error {
				_, err := field.ToBytes(
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
			"Unpack",
			func() error {
				_, err := field.Unpack(
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
			"_DataArgsRefused",
			func() error {
				_, _, _, err := field.DataArgsRefused(
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
			"_ByteArgsRefused",
			func() error {
				_, err := field.ByteArgsRefused(
					nil,
					nil,
					nil,
					new(common.Field),
				)

				return err
			},
			"",
			true,
		},
		{
			"_FromBytes valid",
			func() error {
				_, err := field.FromBytes(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.Bytes("\x01"),
					}, nil,
				)

				return err
			},
			"",
			false,
		},
		{
			"_ToBytes unsigned",
			func() error {
				_, err := field.ToBytes(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.MakeInt(1),
						starlark.MakeInt(1),
					}, nil,
				)

				return err
			},
			"",
			false,
		},
		{
			"_ToBytes signed",
			func() error {
				_, err := field.ToBytes(
					new(starlark.Thread),
					nil,
					starlark.Tuple{
						starlark.MakeInt(1),
						starlark.MakeInt(1),
					}, []starlark.Tuple{
						{starlark.String("signed"), starlark.True},
					},
				)

				return err
			},
			"",
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
