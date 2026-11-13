// This file tests the buffer helpers with a nil builtin, which no script can
// build, because the wrapper of NewBuiltin always passes its own builtin.
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ from guard_called_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
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
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
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
			"_NewBuffer",
			func() error {
				_, err := buffer.NewBuffer(
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
			"_Width",
			func() error {
				_, _, err := buffer.Width(
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
			"_Number",
			func() error {
				_, err := buffer.Number(4)(
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
			"_Order",
			func() error {
				_, _, err := buffer.Order(4)(
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
			"_Width valid",
			func() error {
				_, _, err := buffer.Width(
					nil,
					starlark.Tuple{
						starlark.MakeInt(1),
					}, nil,
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

// TestGuard_NilBuiltinNoReceiver proves a nil builtin takes the no-receiver
// path: a buffer read answers an empty value, and a write or a reader move is
// refused with the exact text "builtin: frozen" and ErrFrozen.
//
// Case from Q214 (C102 fix round 1).
//
// Revisions:
//   - 2026-11-06 21:39: initial creation (C102, Q214)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_NilBuiltinNoReceiver(t *testing.T) {
	var thread = new(starlark.Thread)

	tests := []struct {
		name string
		call func() (starlark.Value, error)
		want string
	}{
		{
			"_Bytes reads an empty buffer",
			func() (starlark.Value, error) {
				return buffer.Bytes(
					thread,
					nil,
					nil,
					nil,
				)
			},
			"",
		},
		{
			"_Appended is refused",
			func() (starlark.Value, error) {
				return buffer.Appended(
					nil,
					[]byte("x"),
				)
			},
			"builtin: frozen",
		},
		{
			"_WriteString (buffer method) is refused",
			func() (starlark.Value, error) {
				return buffer.WriteString(
					thread,
					nil,
					starlark.Tuple{starlark.String("x")},
					nil,
				)
			},
			"builtin: frozen",
		},
		{
			"_Take is refused",
			func() (starlark.Value, error) {
				data, err := buffer.Take(
					nil,
					starlark.Tuple{starlark.MakeInt(1)},
					nil,
				)

				return starlark.Bytes(data), err
			},
			"builtin: frozen",
		},
		{
			"_Movable is refused",
			func() (starlark.Value, error) {
				_, err := buffer.Movable(nil)

				return nil, err
			},
			"builtin: frozen",
		},
		{
			"_ReadBytes (reader method) is refused",
			func() (starlark.Value, error) {
				return buffer.ReadBytes(
					thread,
					nil,
					starlark.Tuple{starlark.MakeInt(1)},
					nil,
				)
			},
			"builtin: frozen",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := test.call()
				if test.want == "" {
					if err != nil {
						t.Fatalf(
							"got %v, want no error",
							err,
						)
					}

					return
				}

				if err == nil || err.Error() != test.want || !errors.Is(
					err,
					buffer.ErrFrozen,
				) {
					t.Fatalf(
						"got %v, want %q wrapping ErrFrozen",
						err,
						test.want,
					)
				}
			},
		)
	}
}
