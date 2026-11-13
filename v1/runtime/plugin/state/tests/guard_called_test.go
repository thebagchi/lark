package tests

// This file tests that the helpers of the state plugin that name their builtin
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
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/store"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
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
			"_Get",
			func() error {
				_, err := store.Get(
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
			"_Update",
			func() error {
				_, err := store.Update(
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
			"_SetData",
			func() error {
				err := store.SetData(
					new(starlark.Thread),
					nil,
					"",
					nil,
				)

				return err
			},
			"",
			true,
		},
		{
			"_Updated",
			func() error {
				_, err := store.Updated(
					new(starlark.Thread),
					nil,
					new(store.Change),
				)

				return err
			},
			"",
			true,
		},
		{
			"_SetData value not data",
			func() error {
				thread, _, end := _Running()
				defer end()

				err := store.SetData(
					thread,
					nil,
					"k",
					_Failing(),
				)

				return err
			},
			"builtin \"k\": builtin_function_or_method: not data a store can hold: " +
				"value cannot be converted",
			false,
		},
		{
			"_SetData deep value",
			func() error {
				thread, _, end := _Running()
				defer end()

				err := store.SetData(
					thread,
					nil,
					"k",
					_Deep(),
				)

				return err
			},
			"builtin \"k\": value nested too deep",
			false,
		},
		{
			"_Get no run",
			func() error {
				_, err := store.Get(
					new(starlark.Thread),
					nil,
					starlark.Tuple{starlark.String("k")},
					nil,
				)

				return err
			},
			"builtin: no run or nothing to act on",
			false,
		},
		{
			"_Get store clash",
			func() error {
				thread, evaluation, end := _Running()
				defer end()

				_ = evaluation

				_, _ = scheduler.SharedIn(
					evaluation,
					spec.NAME,
					func() int {
						return 1
					},
				)

				_, err := store.Get(
					thread,
					nil,
					starlark.Tuple{starlark.String("k")},
					nil,
				)

				return err
			},
			"builtin: state holds a int: a run local holds another type",
			false,
		},
		{
			"_Get deep entry",
			func() error {
				thread, evaluation, end := _Running()
				defer end()

				_ = evaluation

				var (
					kept, _ = store.Of(evaluation)
					entry   = new(store.Entry)
					held    = _Deep()
				)

				entry.Value.Store(&held)
				kept.Held["k"] = entry

				_, err := store.Get(
					thread,
					nil,
					starlark.Tuple{starlark.String("k")},
					nil,
				)

				return err
			},
			"builtin: value nested too deep",
			false,
		},
		{
			"_Update no run",
			func() error {
				_, err := store.Update(
					new(starlark.Thread),
					nil,
					starlark.Tuple{starlark.String("k"), _Failing()},
					nil,
				)

				return err
			},
			"builtin: no run or nothing to act on",
			false,
		},
		{
			"_Update store clash",
			func() error {
				thread, evaluation, end := _Running()
				defer end()

				_ = evaluation

				_, _ = scheduler.SharedIn(
					evaluation,
					spec.NAME,
					func() int {
						return 1
					},
				)

				_, err := store.Update(
					thread,
					nil,
					starlark.Tuple{starlark.String("k"), _Failing()},
					nil,
				)

				return err
			},
			"builtin: state holds a int: a run local holds another type",
			false,
		},
		{
			"_Updated change fails",
			func() error {
				thread, evaluation, end := _Running()
				defer end()

				_ = evaluation

				_, err := store.Updated(
					thread,
					evaluation,
					&store.Change{
						Name:   "k",
						Held:   new(store.Entry),
						Change: _Failing(),
					},
				)

				return err
			},
			"builtin \"k\": failing",
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

// _Deep is a list nested past common.DEPTH, which a walk of the value refuses.
//
// Revisions:
//   - 2026-11-06 22:30: initial creation (C102 fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Deep() starlark.Value {
	var value starlark.Value = starlark.NewList(nil)

	for range common.DEPTH + 1 {
		value = starlark.NewList([]starlark.Value{value})
	}

	return value
}

// _Running is a thread inside a run, and the call that ends the run.
//
// Revisions:
//   - 2026-11-06 22:30: initial creation (C102 fix round 2)
//   - 2026-11-07 20:07: uses common.Evaluation (C91w5, Q127)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Running() (*starlark.Thread, common.Evaluation, func()) {
	var (
		thread = new(starlark.Thread)
		finish = scheduler.Begin(
			context.Background(),
			thread,
			"guard",
			nil,
		)
	)

	evaluation, _ := scheduler.EvaluationOf(thread)

	return thread, evaluation, func() {
		_ = finish(nil)
	}
}

// _Failing is a callable that always fails.
//
// Revisions:
//   - 2026-11-06 22:30: initial creation (C102 fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Failing() starlark.Callable {
	return starlark.NewBuiltin(
		"failing",
		func(
			*starlark.Thread,
			*starlark.Builtin,
			starlark.Tuple,
			[]starlark.Tuple,
		) (starlark.Value, error) {
			return nil, errors.New("failing")
		},
	)
}
