package tests

import (
	"errors"
	"math"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestGuard_NilThreadAnswers proves every call that takes a thread answers a
// nil thread as it answers a thread with no run, and does not panic.
//
// Cases from Go-level breaker run 3, findings scheduler-F21, F22, F23, F24,
// F32, F36 and F40 (bugs B075, B076, B077, B078, B084, B088 and B092). The root
// fix is C52's: _Of answers ErrRun for a nil thread. These tests pass on the
// base for that reason; they stay so a change to _Of cannot lose the answer.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestGuard_NilThreadAnswers(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			"Updating",
			func() error {
				_, err := scheduler.Updating(nil)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Context",
			func() error {
				_, err := scheduler.Context(nil)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"AttemptOf is NO_ATTEMPT",
			func() error {
				if scheduler.AttemptOf(nil) != common.NO_ATTEMPT {
					return errors.New("not NO_ATTEMPT")
				}

				return nil
			},
			nil,
		},
		{
			"Fork",
			func() error {
				_, err := scheduler.Fork(
					nil,
					nil,
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"EvaluationOf",
			func() error {
				_, err := scheduler.EvaluationOf(nil)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Lock",
			func() error {
				_, err := scheduler.Lock(
					nil,
					"name",
				)

				return err
			},
			scheduler.ErrRun,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := test.call()
				if !errors.Is(
					got,
					test.want,
				) || (test.want == nil && got != nil) {
					t.Fatalf(
						"got %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// TestGuard_DurationNil proves a nil value is ErrDuration and not a panic, and
// that the answer names no type the value does not have.
//
// Case from Go-level breaker run 3, finding scheduler-F35 (bug B087).
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_DurationNil(t *testing.T) {
	tests := []struct {
		name  string
		value starlark.Value
		ok    bool
	}{
		{
			"nil",
			nil,
			false,
		},
		{
			"string",
			starlark.String("100"),
			false,
		},
		{
			"NaN",
			starlark.Float(math.NaN()),
			false,
		},
		{
			"zero",
			starlark.MakeInt64(0),
			true,
		},
		{
			"thousand",
			starlark.MakeInt64(1000),
			true,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := scheduler.Duration(test.value)
				if test.ok != (err == nil) {
					t.Fatalf(
						"got %v, want ok %v",
						err,
						test.ok,
					)
				}

				if !test.ok && !errors.Is(
					err,
					scheduler.ErrDuration,
				) {
					t.Fatalf(
						"got %v, want ErrDuration",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_NamedNil proves a nil target names nothing of its own, so the
// callee is all there is.
//
// Case from Go-level breaker run 3, finding scheduler-F34 (bug B086).
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_NamedNil(t *testing.T) {
	tests := []struct {
		name   string
		callee string
	}{
		{
			"no callee",
			"",
		},
		{
			"a callee",
			"work",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := scheduler.Named(
					nil,
					test.callee,
				)
				if got != test.callee {
					t.Fatalf(
						"got %q, want %q",
						got,
						test.callee,
					)
				}
			},
		)
	}
}

// TestGuard_LabelledNilChild proves the option does nothing when it has no child
// to label.
//
// Case from Go-level breaker run 3, finding scheduler-F33 (bug B085).
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_LabelledNilChild(t *testing.T) {
	option := scheduler.Labelled(
		"name",
		"binding",
	)

	option(
		nil,
		nil,
	)
}

// TestGuard_EvaluationLockNoLocals proves an evaluation with no locals, and no
// evaluation at all, is ErrRun and not a panic.
//
// Case from Go-level breaker run 3, finding scheduler-F37 (bug B089).
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-07 20:01: uses the concrete types of scheduler/internal/run (C91w5)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_EvaluationLockNoLocals(t *testing.T) {
	var nobody *run.Evaluation

	tests := []struct {
		name       string
		evaluation *run.Evaluation
	}{
		{
			"zero evaluation",
			new(run.Evaluation),
		},
		{
			"nil evaluation",
			nobody,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := test.evaluation.Lock("name")
				if !errors.Is(
					err,
					scheduler.ErrRun,
				) {
					t.Fatalf(
						"got %v, want ErrRun",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_NilHandle proves Stop and Settle do nothing for a nil handle.
//
// Cases from Go-level breaker run 3, findings scheduler-F38 and F49 (bugs B090
// and B100). Settle is the call that panicked under _Abort.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-07 20:01: uses the concrete types of scheduler/internal/run (C91w5)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_NilHandle(t *testing.T) {
	var handle *run.Handle

	t.Run(
		"Stop does nothing",
		func(t *testing.T) {
			handle.Stop()
		},
	)

	t.Run(
		"Settle does nothing",
		func(t *testing.T) {
			scheduler.Settle(
				nil,
				handle,
			)
		},
	)

	t.Run(
		"the other methods answer a finished handle",
		func(t *testing.T) {
			_ = handle.String()
			_ = handle.Thread()
			_ = handle.Name()

			_, over, err := handle.Result()
			if !over || err != nil {
				t.Fatal("want a finished handle with no error")
			}

			select {
			case <-handle.Done():
			case <-time.After(time.Second):
				t.Fatal("Done blocked")
			}
		},
	)
}
