package tests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// SHARED_KEY is the key the shared-value tests claim, and OTHER is a second
	// name to lock while LOCKED is held.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHARED_KEY = "evaluation.test"
	OTHER      = "other"

	// HOLDING and INHERITED are the two refusals of a nested lock: from the
	// evaluation that took LOCKED, and from one started while it was held.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HOLDING   = `already updating "counter": an update cannot start another`
	INHERITED = `started inside an update of "counter": an update cannot start another`
)

// _Begun starts a run on a new thread and ends it when the test ends.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Begun(t *testing.T) *starlark.Thread {
	t.Helper()

	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			nil,
		)
	)

	t.Cleanup(func() {
		err := finish(nil)
		if err != nil {
			t.Errorf(
				"finish: %v",
				err,
			)
		}
	})

	return thread
}

// _Found is the evaluation on thread, or the end of the test.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Found(
	t *testing.T,
	thread *starlark.Thread,
) common.Evaluation {
	t.Helper()

	evaluation, err := scheduler.EvaluationOf(thread)
	if err != nil {
		t.Fatalf(
			"evaluation: %v",
			err,
		)
	}

	return evaluation
}

// TestEvaluationOf_RefusesAThreadWithNoRun proves the handle refuses a bare
// thread with the error each function that takes a thread gives.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestEvaluationOf_RefusesAThreadWithNoRun(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		_, err = scheduler.EvaluationOf(thread)
	)

	_, locked := scheduler.Lock(
		thread,
		LOCKED,
	)

	var (
		found = errors.Is(
			err,
			scheduler.ErrRun,
		)
		same = errors.Is(
			locked,
			scheduler.ErrRun,
		)
	)

	if !found || !same {
		t.Fatalf(
			"got %v and %v from Lock, want ErrRun from both",
			err,
			locked,
		)
	}
}

// TestEvaluation_AnswersAsTheFunctionsDo is the contract the functions that
// take a thread keep: a handle gives the same shared value
// they give, and the same refusal of a key that holds another type.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-01 13:00: no budget to compare
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLocal, formerly ErrNotLocal (C121, Q349)
func TestEvaluation_AnswersAsTheFunctionsDo(t *testing.T) {
	var (
		thread     = _Begun(t)
		evaluation = _Found(
			t,
			thread,
		)
	)

	made, err := scheduler.SharedIn(
		evaluation,
		SHARED_KEY,
		func() *int {
			return new(int)
		},
	)
	if err != nil {
		t.Fatalf(
			"shared in: %v",
			err,
		)
	}

	found, err := scheduler.Shared(
		thread,
		SHARED_KEY,
		func() *int {
			t.Fatal("a second build of one key")

			return nil
		},
	)
	if err != nil || found != made {
		t.Fatalf(
			"Shared gave %p and %v, want %p",
			found,
			err,
			made,
		)
	}

	var (
		_, wrong = scheduler.SharedIn(
			evaluation,
			SHARED_KEY,
			func() string {
				return ""
			},
		)
		_, refused = scheduler.Shared(
			thread,
			SHARED_KEY,
			func() string {
				return ""
			},
		)
	)

	if !errors.Is(
		wrong,
		scheduler.ErrLocal,
	) || refused == nil || wrong.Error() != refused.Error() {
		t.Fatalf(
			"got %v from the handle and %v from the thread, want one ErrLocal",
			wrong,
			refused,
		)
	}
}

// TestEvaluation_LockRefusesAsLockDoes holds LOCKED and asks for OTHER from
// the holder, from a spawn, and from an attempt. The handle and Lock must give
// one refusal in each place.
//
// The attempt is the case the handle can get wrong. Within puts the attempt's
// own evaluation on the holder's thread, so a handle taken there must refuse
// as one started inside the update, not as the holder.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestEvaluation_LockRefusesAsLockDoes(t *testing.T) {
	cases := map[string]struct {
		reach func(
			thread *starlark.Thread,
			probe starlark.Callable,
		) error
		want string
	}{
		"the holder": {
			reach: func(
				thread *starlark.Thread,
				probe starlark.Callable,
			) error {
				_, err := starlark.Call(
					thread,
					probe,
					nil,
					nil,
				)

				return err
			},
			want: HOLDING,
		},
		"a spawn": {
			reach: func(
				thread *starlark.Thread,
				probe starlark.Callable,
			) error {
				handle, err := scheduler.Fork(
					thread,
					probe,
				)
				if err != nil {
					return err
				}

				_, err = scheduler.Wait(
					thread,
					handle,
				)

				return err
			},
			want: INHERITED,
		},
		"an attempt": {
			reach: func(
				thread *starlark.Thread,
				probe starlark.Callable,
			) error {
				_, err := scheduler.Within(
					thread,
					probe,
				)

				return err
			},
			want: INHERITED,
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					thread       = _Begun(t)
					release, err = _Found(
						t,
						thread,
					).Lock(LOCKED)
				)
				if err != nil {
					t.Fatalf(
						"lock: %v",
						err,
					)
				}

				defer scheduler.Release(release)

				var got []string

				probe := starlark.NewBuiltin(
					"probe",
					func(
						child *starlark.Thread,
						_ *starlark.Builtin,
						_ starlark.Tuple,
						_ []starlark.Tuple,
					) (starlark.Value, error) {
						_, err := scheduler.Lock(
							child,
							OTHER,
						)
						got = append(
							got,
							fmt.Sprint(err),
						)

						evaluation, err := scheduler.EvaluationOf(child)
						if err != nil {
							return nil, err
						}

						_, err = evaluation.Lock(OTHER)
						got = append(
							got,
							fmt.Sprint(err),
						)

						return starlark.None, nil
					},
				)

				err = tc.reach(
					thread,
					probe,
				)
				if err != nil {
					t.Fatalf(
						"reach: %v",
						err,
					)
				}

				want := []string{
					tc.want,
					tc.want,
				}

				if !slices.Equal(
					got,
					want,
				) {
					t.Fatalf(
						"Lock and the handle said %q, want %q from both",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestEvaluation_AHeldNameWaitsUntilCancelled is the slow half of the lock,
// through the handle: a name another evaluation holds is not taken, and the
// wait for it ends with ErrCancelled when the waiter is stopped.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestEvaluation_AHeldNameWaitsUntilCancelled(t *testing.T) {
	var (
		thread = _Begun(t)
		ready  = make(chan struct{})
		result = make(
			chan error,
			1,
		)
		waiter = starlark.NewBuiltin(
			"waiter",
			func(
				child *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				evaluation, err := scheduler.EvaluationOf(child)
				if err != nil {
					result <- err

					return starlark.None, nil
				}

				<-ready

				release, err := evaluation.Lock(LOCKED)
				if err == nil {
					scheduler.Release(release)
				}

				result <- err

				return starlark.None, nil
			},
		)
	)

	handle, err := scheduler.Fork(
		thread,
		waiter,
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	release, err := _Found(
		t,
		thread,
	).Lock(LOCKED)
	if err != nil {
		t.Fatalf(
			"lock: %v",
			err,
		)
	}

	defer scheduler.Release(release)

	close(ready)

	select {
	case err = <-result:
		t.Fatalf(
			"the waiter returned %v while the name was held",
			err,
		)
	case <-time.After(SETTLE):
	}

	handle.Stop()

	select {
	case err = <-result:
	case <-time.After(LIMIT):
		t.Fatal("a stopped waiter still waits for the name")
	}

	var (
		cancelled = errors.Is(
			err,
			scheduler.ErrCancelled,
		) && errors.Is(
			err,
			context.Canceled,
		)
		named = strings.HasPrefix(
			err.Error(),
			WAITING,
		)
	)

	if !cancelled || !named {
		t.Fatalf(
			"got %v, want %s: ErrCancelled: context canceled",
			err,
			WAITING,
		)
	}
}

// TestRegression_EvaluationAllocatesNothing is why the handle exists: one
// lookup instead of three. A handle that cost an allocation would trade one
// cost for another, so finding the evaluation, locking and releasing must
// allocate nothing.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-05 22:05: one allocation, the release of the hold (C58 fix 1, Q85)
//   - 2026-11-05 22:40: none again, with a Hold in place of a closure (C58 fix 2, Q201)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_EvaluationAllocatesNothing(t *testing.T) {
	var (
		thread  = _Begun(t)
		failure error
	)

	allocs := testing.AllocsPerRun(
		ROUNDS,
		func() {
			evaluation, err := scheduler.EvaluationOf(thread)
			if err != nil {
				failure = err

				return
			}

			release, err := evaluation.Lock(LOCKED)
			if err != nil {
				failure = err

				return
			}

			scheduler.Release(release)
		},
	)

	if failure != nil {
		t.Fatalf(
			"lock: %v",
			failure,
		)
	}

	if allocs != 0 {
		t.Fatalf(
			"a lookup, a lock and its release made %v allocations, want 0",
			allocs,
		)
	}
}

// TestLane_IsTheSpineOrTheLaneOfASpawn proves Lane gives the spine for the
// entry thread and for a thread with no run, and gives a spawn's own thread id
// inside that spawn.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestLane_IsTheSpineOrTheLaneOfASpawn(t *testing.T) {
	var (
		thread = _Begun(t)
		inside = make(
			chan string,
			1,
		)
	)

	if got := scheduler.Lane(thread); got != common.SPINE {
		t.Fatalf(
			"entry lane = %q, want %q",
			got,
			common.SPINE,
		)
	}

	if got := scheduler.Lane(nil); got != common.SPINE {
		t.Fatalf(
			"nil thread lane = %q, want %q",
			got,
			common.SPINE,
		)
	}

	handle, err := scheduler.Fork(
		thread,
		starlark.NewBuiltin(
			"child",
			func(
				self *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				inside <- scheduler.Lane(self)

				return starlark.None, nil
			},
		),
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	got := <-inside
	if got != handle.Thread() || got == common.SPINE {
		t.Fatalf(
			"spawn lane = %q, want the handle's %q",
			got,
			handle.Thread(),
		)
	}
}
