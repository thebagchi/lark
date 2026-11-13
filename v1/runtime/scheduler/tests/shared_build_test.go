package tests

import (
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// BUILD_LIMIT is how long a build may take before the test calls it a hang.
	//
	// Revisions:
	//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
	BUILD_LIMIT = 5 * time.Second
)

// _Within runs body on its own goroutine and fails the test when it has not
// returned within BUILD_LIMIT, which is how a deadlock shows.
//
// Revisions:
//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
func _Within(
	t *testing.T,
	what string,
	body func(),
) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		body()
	}()

	select {
	case <-done:
	case <-time.After(BUILD_LIMIT):
		t.Fatalf(
			"hang: %s",
			what,
		)
	}
}

// TestShared_ABuildMayAskForAnotherKey proves Q296: build runs with the run
// mutex released, so a build that calls Shared for another key returns, and
// both keys are held afterwards.
//
// Revisions:
//   - 2026-11-10 11:50: initial creation, from the breaker case scheduler-F3 (C116, B238, Q296)
func TestShared_ABuildMayAskForAnotherKey(t *testing.T) {
	thread := _Begun(t)

	_Within(
		t,
		"Shared build that calls Shared for another key",
		func() {
			got, err := scheduler.Shared(
				thread,
				"outer",
				func() int {
					inner, err := scheduler.Shared(
						thread,
						"inner",
						func() int {
							return 2
						},
					)
					if err != nil {
						t.Errorf(
							"inner: %v",
							err,
						)
					}

					return inner + 1
				},
			)
			if err != nil || got != 3 {
				t.Errorf(
					"outer = %d, %v, want 3, nil",
					got,
					err,
				)
			}
		},
	)

	inner, err := scheduler.Shared[int](
		thread,
		"inner",
		nil,
	)
	if err != nil || inner != 2 {
		t.Fatalf(
			"inner = %d, %v, want 2, nil",
			inner,
			err,
		)
	}
}

// TestShared_ABuildMayStartAThread proves a build that starts a thread
// returns: numbering the thread takes the run mutex, which build no longer
// holds (Q296).
//
// Revisions:
//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestShared_ABuildMayStartAThread(t *testing.T) {
	thread := _Begun(t)

	_Within(
		t,
		"Shared build that starts a thread",
		func() {
			_, err := scheduler.Shared(
				thread,
				"spawning",
				func() int {
					_, err := scheduler.Fork(
						thread,
						starlark.NewBuiltin(
							"child",
							func(
								*starlark.Thread,
								*starlark.Builtin,
								starlark.Tuple,
								[]starlark.Tuple,
							) (starlark.Value, error) {
								return starlark.None, nil
							},
						),
					)
					if err != nil {
						t.Errorf(
							"fork: %v",
							err,
						)
					}

					return 1
				},
			)
			if err != nil {
				t.Errorf(
					"shared: %v",
					err,
				)
			}
		},
	)
}

// TestShared_BuildsOnceWhenManyThreadsRace proves each key has one once: of
// many callers racing to be first, one builds and all see its value.
//
// Revisions:
//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
func TestShared_BuildsOnceWhenManyThreadsRace(t *testing.T) {
	const CALLERS = 16

	var (
		thread = _Begun(t)
		group  sync.WaitGroup
		mutex  sync.Mutex
		built  int
		seen   = make(
			[]*int,
			CALLERS,
		)
	)

	for index := range CALLERS {
		group.Go(func() {
			value, err := scheduler.Shared(
				thread,
				"once",
				func() *int {
					mutex.Lock()
					defer mutex.Unlock()

					built++

					return new(int)
				},
			)
			if err != nil {
				t.Errorf(
					"shared: %v",
					err,
				)
			}

			seen[index] = value
		})
	}

	group.Wait()

	if built != 1 {
		t.Fatalf(
			"build ran %d times, want 1",
			built,
		)
	}

	for index, value := range seen {
		if value != seen[0] {
			t.Fatalf(
				"caller %d saw another value",
				index,
			)
		}
	}
}

// TestShared_AKeyBuildsWhileAnotherKeyBuilds proves the once is per key: a
// build of one key that waits for the build of another key, on another thread,
// does not deadlock.
//
// Revisions:
//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
func TestShared_AKeyBuildsWhileAnotherKeyBuilds(t *testing.T) {
	var (
		thread = _Begun(t)
		group  sync.WaitGroup
		inside = make(chan struct{})
		other  = make(chan struct{})
	)

	group.Go(func() {
		_, _ = scheduler.Shared(
			thread,
			"slow",
			func() int {
				close(inside)

				<-other

				return 1
			},
		)
	})

	<-inside

	_Within(
		t,
		"a build of one key waits for the build of another key",
		func() {
			_, err := scheduler.Shared(
				thread,
				"quick",
				func() int {
					return 2
				},
			)
			if err != nil {
				t.Errorf(
					"quick: %v",
					err,
				)
			}
		},
	)

	close(other)
	group.Wait()
}

// TestShared_APanicInBuildLeavesTheKeyUnbuilt proves a build that panics holds
// nothing, so the next call builds again, as it did when build ran under the
// mutex.
//
// Revisions:
//   - 2026-11-10 11:50: initial creation (C116, B238, Q296)
func TestShared_APanicInBuildLeavesTheKeyUnbuilt(t *testing.T) {
	thread := _Begun(t)

	_Within(
		t,
		"a build that panics",
		func() {
			defer func() {
				_ = recover()
			}()

			_, _ = scheduler.Shared(
				thread,
				"panics",
				func() int {
					panic("build failed")
				},
			)
		},
	)

	got, err := scheduler.Shared(
		thread,
		"panics",
		func() int {
			return 7
		},
	)
	if err != nil || got != 7 {
		t.Fatalf(
			"got %d, %v, want 7, nil",
			got,
			err,
		)
	}
}
