package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// _Foreign is a Handle and an Evaluation the scheduler did not make. It answers
// as a finished handle holding the value 7.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Foreign struct {
	starlark.Value
	stopped bool
}

// Done answers a closed channel.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Foreign) Done() <-chan struct{} {
	over := make(chan struct{})
	close(over)

	return over
}

// Name answers the name of a foreign handle.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Foreign) Name() string {
	return "foreign"
}

// Result answers the value 7, as a finished handle.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Foreign) Result() (starlark.Value, bool, error) {
	return starlark.MakeInt(7), true, nil
}

// Stop does nothing.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Foreign) Stop() {
	f.stopped = true
}

// Thread answers the lane of a foreign handle.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Foreign) Thread() string {
	return "foreign"
}

// Lock answers ErrRun, as an evaluation with no run.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (f *_Foreign) Lock(name string) (common.Hold, error) {
	return common.Hold{}, scheduler.ErrRun
}

// TestForeign_SharedInRefusesWhatItDidNotMake proves SharedIn answers ErrRun
// for a nil Evaluation, a zero one and one another package wrote, and does not
// panic.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestForeign_SharedInRefusesWhatItDidNotMake(t *testing.T) {
	tests := []struct {
		name       string
		evaluation common.Evaluation
	}{
		{
			"nil interface",
			nil,
		},
		{
			"nil pointer",
			(*run.Evaluation)(nil),
		},
		{
			"zero",
			new(run.Evaluation),
		},
		{
			"foreign",
			new(_Foreign),
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := scheduler.SharedIn(
					test.evaluation,
					"k",
					func() int {
						return 1
					},
				)
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

// TestForeign_WaitAndSettleUseTheInterface proves Wait and Settle read a
// Handle through its methods, so a nil, a zero and a foreign handle all end the
// wait at once, and the foreign one gives its value.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestForeign_WaitAndSettleUseTheInterface(t *testing.T) {
	tests := []struct {
		name   string
		handle common.Handle
		want   string
	}{
		{
			"nil interface",
			nil,
			"<nil>",
		},
		{
			"zero",
			new(run.Handle),
			"<nil>",
		},
		{
			"foreign",
			new(_Foreign),
			"7",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					thread     = _Begun(t)
					value, err = scheduler.Wait(
						thread,
						test.handle,
					)
				)
				if err != nil {
					t.Fatalf(
						"Wait: %v",
						err,
					)
				}

				got := "<nil>"
				if value != nil {
					got = value.String()
				}

				if got != test.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						test.want,
					)
				}

				scheduler.Settle(
					thread,
					test.handle,
				)
			},
		)
	}
}

// TestForeign_ReleaseAndOptionsIgnoreWhatTheyDidNotMake proves Release with an
// Owner of another type, and the four option constructors with Evaluations of
// another type or with no locals, do nothing and do not panic.
//
// Revisions:
//   - 2026-11-07 20:06: initial creation (C91w5, Q127, Q128, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func TestForeign_ReleaseAndOptionsIgnoreWhatTheyDidNotMake(t *testing.T) {
	var (
		thread    = _Begun(t)
		held, err = scheduler.Lock(
			thread,
			LOCKED,
		)
	)
	if err != nil {
		t.Fatalf(
			"Lock: %v",
			err,
		)
	}

	scheduler.Release(common.Hold{
		Owner: "not a lock",
		Token: 1,
	})
	scheduler.Release(common.Hold{
		Owner: (*run.Evaluation)(nil),
		Token: 1,
	})

	_, err = scheduler.Lock(
		thread,
		OTHER,
	)
	if !errors.Is(
		err,
		scheduler.ErrNested,
	) {
		t.Fatalf(
			"Lock after a foreign release gave %v, want ErrNested",
			err,
		)
	}

	scheduler.Release(held)

	for _, option := range []common.ForkOption{
		scheduler.Attempt(2),
		scheduler.Catching(),
		scheduler.Inline(),
		scheduler.Labelled(
			"a",
			"b",
		),
	} {
		option(
			new(_Foreign),
			new(_Foreign),
		)
		option(
			nil,
			nil,
		)
		option(
			new(run.Evaluation),
			new(_Foreign),
		)
		option(
			new(run.Evaluation),
			new(run.Evaluation),
		)
	}
}
