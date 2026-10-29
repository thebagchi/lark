package scheduler_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// SHARED_KEY is the key the shared-value tests claim, and OTHER is a second
	// name to lock while LOCKED is held.
	SHARED_KEY = "evaluation.test"
	OTHER      = "other"

	// HOLDING and INHERITED are the two refusals of a nested lock: from the
	// evaluation that took LOCKED, and from one started while it was held.
	HOLDING   = `already updating "counter": an update cannot start another`
	INHERITED = `started inside an update of "counter": an update cannot start another`
)

// _Begun starts a run on a new thread and ends it when the test ends.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
func _Begun(t *testing.T) *starlark.Thread {
	t.Helper()

	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			spelling.ENTRY,
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
func _Found(
	t *testing.T,
	thread *starlark.Thread,
) *scheduler.Evaluation {
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
			scheduler.ErrNoRun,
		)
		same = errors.Is(
			locked,
			scheduler.ErrNoRun,
		)
	)

	if !found || !same {
		t.Fatalf(
			"got %v and %v from Lock, want ErrNoRun from both",
			err,
			locked,
		)
	}
}

// TestEvaluation_AnswersAsTheFunctionsDo is the contract the functions that
// take a thread keep: a handle gives the same budget and the same shared value
// they give, and the same refusal of a key that holds another type.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
func TestEvaluation_AnswersAsTheFunctionsDo(t *testing.T) {
	var (
		thread     = _Begun(t)
		evaluation = _Found(
			t,
			thread,
		)
	)

	if evaluation.Allowance() != scheduler.Allowance(thread) {
		t.Fatal("the handle and the thread charge two budgets")
	}

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
		scheduler.ErrNotLocal,
	) || refused == nil || wrong.Error() != refused.Error() {
		t.Fatalf(
			"got %v from the handle and %v from the thread, want one ErrNotLocal",
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
				handle, err := scheduler.Beside(
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

				defer release()

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
					release()
				}

				result <- err

				return starlark.None, nil
			},
		)
	)

	handle, err := scheduler.Beside(
		thread,
		waiter,
	)
	if err != nil {
		t.Fatalf(
			"beside: %v",
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

	defer release()

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

			release()
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
