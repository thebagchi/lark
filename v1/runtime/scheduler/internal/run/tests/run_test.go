package tests

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestOf_RefusesAThreadWithNoRun proves a builtin called on a thread nothing
// set up, or on none, is told so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:01: initial creation
//   - 2026-11-08 22:45: moved to the external package, through EvaluationOf, with a
//     row for no thread (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestOf_RefusesAThreadWithNoRun(t *testing.T) {
	tests := []struct {
		name   string
		thread *starlark.Thread
	}{
		{
			name: "a thread nothing set up",
			thread: &starlark.Thread{
				Name: SCRIPT_NAME,
			},
		},
		{
			name:   "no thread",
			thread: nil,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := run.EvaluationOf(tt.thread)
				if !errors.Is(
					err,
					run.ErrRun,
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

// TestNumber_NamesAThreadAfterItsParent proves an id says whose child it is,
// and that the spine contributes no prefix. The second row is the defect a
// shared counter has: a sibling started after a nephew would take the higher
// number and the ids would stop describing the tree.
//
// The first step spawns the spine's first child, which spawns a child of its own.
// A gate puts that grandchild after the spine's second child, or before it.
//
// Revisions:
//   - 2026-09-19 21:58: initial creation, as TestTrack_NumbersFromAfterTheSpine
//   - 2026-09-21 00:59: an id names its parent rather than counting from after
//     the spine
//   - 2026-09-21 00:59: initial creation
//   - 2026-09-21 08:09: numbering is its own call, since nothing tracks handles
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-09 01:07: merged TestNumber_CountsPerParentRatherThanPerRun as the second row, and
//     moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestNumber_NamesAThreadAfterItsParent(t *testing.T) {
	tests := []struct {
		name        string
		nephewFirst bool
		want        []string
	}{
		{
			name:        "the spine's children and a child of the first",
			nephewFirst: false,
			want:        []string{"thread_1", "thread_2", "thread_1_1"},
		},
		{
			name:        "a nephew started first does not take the sibling's number",
			nephewFirst: true,
			want:        []string{"thread_1", "thread_1_1", "thread_2"},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					thread, _ = _Open(
						t.Context(),
						t,
						nil,
					)
					gate   = make(chan struct{})
					grand  string
					nested error
					first  = starlark.NewBuiltin(
						"first",
						func(
							child *starlark.Thread,
							_ *starlark.Builtin,
							_ starlark.Tuple,
							_ []starlark.Tuple,
						) (starlark.Value, error) {
							if !tt.nephewFirst {
								<-gate
							}

							handle, err := run.Fork(
								child,
								_Callable(
									t,
									WORKER,
								),
							)
							if err != nil {
								nested = err

								return starlark.None, nil
							}

							grand = handle.Thread()

							_, nested = run.Wait(
								child,
								handle,
							)

							return starlark.None, nil
						},
					)
					got []string
				)

				a, err := run.Fork(
					thread,
					first,
				)
				if err != nil {
					t.Fatal(err)
				}

				got = append(
					got,
					a.Thread(),
				)

				if tt.nephewFirst {
					if _, err := run.Wait(
						thread,
						a,
					); err != nil {
						t.Fatal(err)
					}

					got = append(
						got,
						grand,
					)
				}

				b, err := run.Fork(
					thread,
					_Callable(
						t,
						WORKER,
					),
				)
				if err != nil {
					t.Fatal(err)
				}

				got = append(
					got,
					b.Thread(),
				)

				if !tt.nephewFirst {
					close(gate)

					if _, err := run.Wait(
						thread,
						a,
					); err != nil {
						t.Fatal(err)
					}

					got = append(
						got,
						grand,
					)
				}

				if nested != nil {
					t.Fatalf(
						"the child's spawn: %v",
						nested,
					)
				}

				if !slices.Equal(
					got,
					tt.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

// TestNumber_GivesRacingSpawnsDistinctIds proves numbering happens under a
// lock. Without it two spawns can read one number, and a graph would show two
// threads as one.
//
// Revisions:
//   - 2026-09-19 21:59: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-09 01:07: moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestNumber_GivesRacingSpawnsDistinctIds(t *testing.T) {
	const RACING_COUNT = 64

	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		target = _Callable(
			t,
			WORKER,
		)
		group sync.WaitGroup
		guard sync.Mutex
		seen  = map[string]bool{}
	)

	for range RACING_COUNT {
		group.Go(func() {
			handle, err := run.Fork(
				thread,
				target,
			)
			if err != nil {
				t.Error(err)

				return
			}

			guard.Lock()
			defer guard.Unlock()

			seen[handle.Thread()] = true
		})
	}

	group.Wait()

	if len(seen) != RACING_COUNT {
		t.Fatalf(
			"%d spawns got %d ids",
			RACING_COUNT,
			len(seen),
		)
	}
}

// TestOf_FindsTheRunOnAThread proves a builtin can reach its evaluation, since
// the interpreter hands a builtin nothing else.
//
// Revisions:
//   - 2026-09-19 22:01: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-09 01:07: moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOf_FindsTheRunOnAThread(t *testing.T) {
	var (
		first, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		second, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		build = func() *int {
			return new(int)
		}
	)

	var (
		found, err = run.EvaluationOf(first)
		again, _   = run.EvaluationOf(first)
		other, _   = run.EvaluationOf(second)
	)
	if err != nil {
		t.Fatalf(
			"EvaluationOf: %v",
			err,
		)
	}

	var (
		held, _ = run.SharedIn(
			found,
			SHARED_FIRST,
			build,
		)
		same, _ = run.SharedIn(
			again,
			SHARED_FIRST,
			build,
		)
		apart, _ = run.SharedIn(
			other,
			SHARED_FIRST,
			build,
		)
	)

	if held != same || held == apart {
		t.Fatal("the evaluation of a thread is not the evaluation of its own run")
	}
}

// TestFail_EndsTheRunUnlessSomethingIsCatching is the one rule Fail exists
// for: the same failure ends the run from a plain evaluation and ends nothing
// from one that is catching.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-09 01:07: moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses ForkOption and Fork, formerly BesideOption and Beside (C129, Q396)
func TestFail_EndsTheRunUnlessSomethingIsCatching(t *testing.T) {
	tests := []struct {
		name    string
		options []common.ForkOption
		ended   bool
	}{
		{
			name:    "catching",
			options: []common.ForkOption{run.Catching()},
			ended:   false,
		},
		{
			name:  "plain",
			ended: true,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					spine  = new(starlark.Thread)
					finish = run.Begin(
						t.Context(),
						spine,
						common.ENTRY,
						nil,
					)
					cause   error
					stopped error
					probe   = starlark.NewBuiltin(
						"probe",
						func(
							child *starlark.Thread,
							_ *starlark.Builtin,
							_ starlark.Tuple,
							_ []starlark.Tuple,
						) (starlark.Value, error) {
							cause = run.Fail(
								child,
								run.ErrNested,
							)

							ctx, err := run.Context(child)
							if err != nil {
								return nil, err
							}

							stopped = ctx.Err()

							return starlark.None, nil
						},
					)
				)

				handle, err := run.Fork(
					spine,
					probe,
					tt.options...,
				)
				if err != nil {
					t.Fatal(err)
				}

				<-handle.Done()

				if !errors.Is(
					cause,
					run.ErrNested,
				) {
					t.Fatalf(
						"want the cause back, got %v",
						cause,
					)
				}

				if (stopped != nil) != tt.ended {
					t.Fatalf(
						"the run's context was %v, want ended %v",
						stopped,
						tt.ended,
					)
				}

				outcome := finish(nil)
				if errors.Is(
					outcome,
					run.ErrNested,
				) != tt.ended {
					t.Fatalf(
						"the run ended with %v, want the cause %v",
						outcome,
						tt.ended,
					)
				}
			},
		)
	}
}

// TestEnd_ExactlyOneOfRacingOutcomesWins proves a run has one outcome when
// many threads end it at once: the outcome is one of the errors handed in, it
// never changes after it is read, and the run is stopped.
//
// The threads wait on one gate so they all reach Fail together. Run it with
// -race -count=200 to see the interleavings.
//
// Revisions:
//   - 2026-11-02 10:56: initial creation
//   - 2026-11-09 01:07: moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestEnd_ExactlyOneOfRacingOutcomesWins(t *testing.T) {
	const RACING_COUNT = 64

	var (
		spine  = new(starlark.Thread)
		finish = run.Begin(
			t.Context(),
			spine,
			common.ENTRY,
			nil,
		)
		gate = make(chan struct{})
		sent = make(
			[]error,
			RACING_COUNT,
		)
		handles = make(
			[]common.Handle,
			RACING_COUNT,
		)
	)

	for index := range RACING_COUNT {
		sent[index] = fmt.Errorf(
			"outcome %d",
			index,
		)

		handle, err := run.Fork(
			spine,
			starlark.NewBuiltin(
				"end",
				func(
					child *starlark.Thread,
					_ *starlark.Builtin,
					_ starlark.Tuple,
					_ []starlark.Tuple,
				) (starlark.Value, error) {
					<-gate

					_ = run.Fail(
						child,
						sent[index],
					)

					return starlark.None, nil
				},
			),
		)
		if err != nil {
			t.Fatal(err)
		}

		handles[index] = handle
	}

	close(gate)

	for _, handle := range handles {
		<-handle.Done()
	}

	ctx, err := run.Context(spine)
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Err() == nil {
		t.Fatal("ending the run left it running")
	}

	var (
		won     = finish(nil)
		matches int
	)

	for _, one := range sent {
		if errors.Is(
			won,
			one,
		) {
			matches++
		}
	}

	if matches != 1 {
		t.Fatalf(
			"the outcome %v matches %d of the %d errors sent, want 1",
			won,
			matches,
			RACING_COUNT,
		)
	}
}

// TestEnd_KeepsTheFirstOutcome proves, one after another, the rule the race
// above leaves to chance: the first error recorded stays, a later one is
// dropped, and a nil ends the run without recording an outcome.
//
// Revisions:
//   - 2026-11-02 10:56: initial creation
//   - 2026-11-09 01:07: moved to the external package, through exported names (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnd_KeepsTheFirstOutcome(t *testing.T) {
	var (
		first  = errors.New("first")
		second = errors.New("second")
	)

	tests := []struct {
		name string
		ends []error
		want error
	}{
		{
			name: "none",
			want: nil,
		},
		{
			name: "one",
			ends: []error{first},
			want: first,
		},
		{
			name: "two",
			ends: []error{first, second},
			want: first,
		},
		{
			name: "a nil first records nothing",
			ends: []error{nil, second},
			want: second,
		},
		{
			name: "a nil later changes nothing",
			ends: []error{first, nil},
			want: first,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					spine  = new(starlark.Thread)
					finish = run.Begin(
						t.Context(),
						spine,
						common.ENTRY,
						nil,
					)
				)

				for _, end := range tt.ends {
					_ = run.Fail(
						spine,
						end,
					)
				}

				ctx, err := run.Context(spine)
				if err != nil {
					t.Fatal(err)
				}

				if (len(tt.ends) > 0) != (ctx.Err() != nil) {
					t.Fatalf(
						"%d ends left the context at %v",
						len(tt.ends),
						ctx.Err(),
					)
				}

				got := finish(nil)
				if tt.want == nil && got != nil || !errors.Is(
					got,
					tt.want,
				) && tt.want != nil {
					t.Fatalf(
						"got %v, want %v",
						got,
						tt.want,
					)
				}
			},
		)
	}
}
