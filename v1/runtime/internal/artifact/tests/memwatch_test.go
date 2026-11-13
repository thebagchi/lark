// This file tests the memory watch through the exported API of the package:
// Compile, Apply, WithMemoryWatch and Perform, with the reading source
// injected through common.Given.Memory (exported since C91w6).
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: Given is in common, with exported fields (C91w6)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"bytes"
	"errors"
	"os"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

const (
	// ENDLESS is a script that runs until the run is cancelled, and BRIEF one
	// that finishes at once. STUCK calls a builtin that ignores the run's
	// context until the test lets it go.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:46: moved to the external package (C65a)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ENDLESS = "def main():\n    for i in range(1000000000):\n        pass\n"
	BRIEF   = "def main():\n    return 1\n"
	STUCK   = "def main():\n    stuck()\n"

	// SHORT is the grace the test hands the watch, and LIMIT how long a test
	// waits for what must happen.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:46: moved to the external package (C65a)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHORT = 50 * time.Millisecond

	// HOG_CHUNK is the size of one allocation of the hog builtin, and
	// HOG_CHUNKS how many it makes.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:46: moved to the external package (C65a)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HOG_CHUNK  = 8 << 20
	HOG_CHUNKS = 200
	LIMIT      = 10 * time.Second
)

// _Over is a source that always reads 99 percent in use, and _Under one that
// reads 1 percent.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Over() (*common.MemoryReading, error) {
	return &common.MemoryReading{
		Used:  99,
		Total: 100,
	}, nil
}

// _Under is a test helper.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Under() (*common.MemoryReading, error) {
	return &common.MemoryReading{
		Used:  1,
		Total: 100,
	}, nil
}

// _Stuck is a plugin whose one builtin waits on a channel, as a Go builtin
// that allocates in a loop does not look at its context either.
//
// entered, when not nil, is closed once the builtin is running, so a test can
// wait on the fact rather than on time.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Stuck struct {
	release chan struct{}
	entered chan struct{}
	once    sync.Once
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Stuck) Name() string {
	return "stuck"
}

// Values returns the builtin that waits.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Stuck) Values() starlark.StringDict {
	return starlark.StringDict{
		"stuck": starlark.NewBuiltin(
			"stuck",
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				if s.entered != nil {
					s.once.Do(func() {
						close(s.entered)
					})
				}

				<-s.release

				return starlark.None, nil
			},
		),
	}
}

// _Hog is a plugin whose one builtin allocates 8 MiB chunks, 200 of them, and
// holds them, never looking at the run's context: the shape of a Go builtin
// that allocates in a loop. It stops by itself so the experiment is bounded.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Hog struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (h *_Hog) Name() string {
	return "hog"
}

// Values returns the builtin that allocates.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (h *_Hog) Values() starlark.StringDict {
	return starlark.StringDict{
		"hog": starlark.NewBuiltin(
			"hog",
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				held := make(
					[][]byte,
					0,
					HOG_CHUNKS,
				)

				for range HOG_CHUNKS {
					held = append(
						held,
						bytes.Repeat(
							[]byte{1},
							HOG_CHUNK,
						),
					)

					time.Sleep(10 * time.Millisecond)
				}

				return starlark.MakeInt(len(held)), nil
			},
		),
	}
}

// _Hogging compiles text with the hog plugin.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Hogging(
	t *testing.T,
	text string,
) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(
		&common.Source{
			Entry: "w.star",
			Text:  []byte(text),
		},
		artifact.WithPlugins(&_Hog{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	return built
}

// _CompiledStuck compiles text with the stuck plugin.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _CompiledStuck(
	t *testing.T,
	text string,
	stuck *_Stuck,
) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(
		&common.Source{
			Entry: "w.star",
			Text:  []byte(text),
		},
		artifact.WithPlugins(stuck),
	)
	if err != nil {
		t.Fatal(err)
	}

	return built
}

// TestMemoryWatch_EndsTheRun proves, with injected readings, that a run with
// the watch on ends with memwatch.ErrLimit ("memory limit reached")
// when the reading is over the limit, finishes normally when it is under, and
// that a run without the option never reads memory (off by default).
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestMemoryWatch_EndsTheRun(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		options []common.RunOption
		source  func() (*common.MemoryReading, error)
		failure bool
	}{
		{
			"over the limit ends an endless script",
			ENDLESS,
			[]common.RunOption{artifact.WithMemoryWatch(nil)},
			_Over,
			true,
		},
		{
			"under the limit lets a short script finish",
			BRIEF,
			[]common.RunOption{artifact.WithMemoryWatch(nil)},
			_Under,
			false,
		},
		{
			"off by default never reads",
			BRIEF,
			nil,
			nil,
			false,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					reads atomic.Int64
					given = artifact.Apply(item.options)
				)

				if given.Memory != nil {
					given.Memory.Interval = time.Millisecond
					given.Memory.Source = func() (
						*common.MemoryReading,
						error,
					) {
						reads.Add(1)

						return item.source()
					}
				}

				var (
					finished = make(
						chan error,
						1,
					)
					built = _CompiledStuck(
						t,
						item.text,
						&_Stuck{},
					)
				)

				go func() {
					_, err := artifact.Perform(
						t.Context(),
						built,
						given,
					)
					finished <- err
				}()

				var err error

				select {
				case err = <-finished:
				case <-time.After(LIMIT):
					t.Fatal("the run did not end")
				}

				if !item.failure {
					if err != nil {
						t.Fatalf(
							"got %v, want success",
							err,
						)
					}

					if item.options == nil && reads.Load() != 0 {
						t.Fatalf(
							"no option, yet memory read %d times",
							reads.Load(),
						)
					}

					return
				}

				if !errors.Is(
					err,
					memwatch.ErrLimit,
				) ||
					!strings.Contains(
						err.Error(),
						"memory limit reached",
					) {
					t.Fatalf(
						"got %v, want memory limit reached",
						err,
					)
				}
			},
		)
	}
}

// TestMemoryWatch_AbandonsARunStuckInABuiltin proves the grace: a builtin that
// ignores the context keeps the run open, so after the grace the host's
// abandon function runs with memwatch.ErrLimit; then, once the builtin lets go,
// the run ends with the same error. The source reads low until the builtin has
// started, so the trip always lands on a run that is stuck.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-05 16:50: the source trips only after the builtin started (C97, B104)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestMemoryWatch_AbandonsARunStuckInABuiltin(t *testing.T) {
	var (
		abandoned = make(
			chan error,
			1,
		)
		stuck = &_Stuck{
			release: make(chan struct{}),
			entered: make(chan struct{}),
		}
		given = artifact.Apply([]common.RunOption{
			artifact.WithMemoryWatch(func(err error) {
				abandoned <- err
			}),
		})
		finished = make(
			chan error,
			1,
		)
		built = _CompiledStuck(
			t,
			STUCK,
			stuck,
		)
	)

	given.Memory.Interval = time.Millisecond
	given.Memory.Grace = SHORT
	// The source reads low until the builtin is running. A source that read
	// high from the first tick could end the run before the builtin starts, and
	// then nothing is stuck to abandon: under load the 1 ms tick beat the run's
	// goroutine to it (B104).
	given.Memory.Source = func() (*common.MemoryReading, error) {
		select {
		case <-stuck.entered:
			return _Over()

		default:
			return _Under()
		}
	}

	go func() {
		_, err := artifact.Perform(
			t.Context(),
			built,
			given,
		)
		finished <- err
	}()

	select {
	case err := <-abandoned:
		if !errors.Is(
			err,
			memwatch.ErrLimit,
		) {
			t.Fatalf(
				"abandoned with %v",
				err,
			)
		}

	case <-time.After(LIMIT):
		t.Fatal("the stuck run was not abandoned")
	}

	close(stuck.release)

	select {
	case err := <-finished:
		if !errors.Is(
			err,
			memwatch.ErrLimit,
		) {
			t.Fatalf(
				"got %v, want ErrLimit",
				err,
			)
		}

	case <-time.After(LIMIT):
		t.Fatal("the run did not end after the builtin returned")
	}
}

// TestMemoryWatch_RealSystem runs a short script with the watch on and no
// injected source, so Perform checks the real system. It passes where the
// system offers /proc/meminfo and cgroup v2, and expects memwatch.ErrUnavailable
// naming a file where it does not: never a silent run without the watch.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMemoryWatch_RealSystem(t *testing.T) {
	_, err := artifact.Perform(
		t.Context(),
		_CompiledStuck(
			t,
			BRIEF,
			&_Stuck{},
		),
		artifact.Apply([]common.RunOption{artifact.WithMemoryWatch(nil)}),
	)

	switch {
	case err == nil:
	case errors.Is(
		err,
		memwatch.ErrUnavailable,
	) && strings.Contains(
		err.Error(),
		"/",
	):
	default:
		t.Fatalf(
			"got %v, want success or ErrUnavailable naming a file",
			err,
		)
	}
}

// _Taken is the memory the Go runtime holds (total less released), read here
// with runtime/metrics so the test needs nothing from the watch's
// internals.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Taken() uint64 {
	samples := []metrics.Sample{
		{
			Name: "/memory/classes/total:bytes",
		},
		{
			Name: "/memory/classes/heap/released:bytes",
		},
	}

	metrics.Read(samples)

	return samples[0].Value.Uint64() - samples[1].Value.Uint64()
}

// TestMemoryWatch_SmallScaleStress is the bounded experiment behind the watch.
// It is gated, because it holds about 1.6 GB: LARK_SMALL_STRESS=1, and run it
// without -race (the race detector multiplies memory):
//
//	LARK_SMALL_STRESS=1 go test -count=1 -v -run SmallScaleStress ./v1/runtime/artifact/
//
// The watch reads a fake machine of 700 MiB in which this
// process's Go memory is all the use, so the trip is at 665 MiB of Go memory
// and no real memory pressure beyond 1.6 GB is possible. Two shapes of endless
// allocation must each end the run within the bound: a loop in the interpreter
// (the run ends with memwatch.ErrLimit; the script's own allocation per step is
// large, so the next poll of the context may come late and the watch may
// abandon it instead, which is also exit 5 in the command) and one Go builtin
// (hog) that allocates and never looks at its context (the watch must
// abandon the run, which the command turns into exit 5). No Go memory limit
// is set: the watch alone ends the run.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: adds the steady builtin calls shape (C27)
//   - 2026-11-02 23:47: the exit code in this text is 5 (C27)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-08 22:46: moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestMemoryWatch_SmallScaleStress(t *testing.T) {
	if os.Getenv("LARK_SMALL_STRESS") != "1" {
		t.Skip("set LARK_SMALL_STRESS=1; see the doc comment")
	}

	const (
		FAKE = 700 << 20
		FILL = "def main():\n    held = []\n    for i in range(100000):\n" +
			"        held.append([0] * 1000000)\n"
		HOG    = "def main():\n    hog()\n"
		STEADY = "def main():\n    chunk = [0] * 100000\n    held = []\n" +
			"    for i in range(100000000):\n" +
			"        held.append(patch_json({\"a\": chunk}," +
			" [{\"op\": \"copy\", \"from\": \"/a\", \"path\": \"/b\"}]))\n"
	)

	source := func() (*common.MemoryReading, error) {
		return &common.MemoryReading{
			Used:  _Taken(),
			Total: FAKE,
		}, nil
	}

	t.Logf(
		"fake machine of %d MiB",
		FAKE>>20,
	)

	cases := []struct {
		name      string
		text      string
		abandoned bool
	}{
		{
			"interpreter loop",
			FILL,
			false,
		},
		{
			"steady builtin calls",
			STEADY,
			false,
		},
		{
			"builtin loop",
			HOG,
			true,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				// The case before left its garbage: give it back, or the
				// fake machine starts this case full.
				debug.FreeOSMemory()
				t.Logf(
					"starting with %d MiB held",
					_Taken()>>20,
				)

				var (
					abandoned atomic.Bool
					began     = time.Now()
					given     = artifact.Apply([]common.RunOption{
						artifact.WithMemoryWatch(func(error) {
							t.Logf(
								"abandoned at %v",
								time.Since(began),
							)
							abandoned.Store(true)
						}),
					})
					finished = make(
						chan error,
						1,
					)
				)

				given.Memory.Source = source

				go func() {
					_, err := artifact.Perform(
						t.Context(),
						_Hogging(
							t,
							item.text,
						),
						given,
					)
					finished <- err
				}()

				select {
				case err := <-finished:
					if !errors.Is(
						err,
						memwatch.ErrLimit,
					) {
						t.Fatalf(
							"got %v, want ErrLimit",
							err,
						)
					}

				case <-time.After(LIMIT * 6):
					t.Fatal("the run did not end within the bound")
				}

				t.Logf(
					"%s: ended after %v, abandoned %v, held %d MiB",
					item.name,
					time.Since(began),
					abandoned.Load(),
					_Taken()>>20,
				)

				if item.abandoned && !abandoned.Load() {
					t.Fatal("a run stuck in a builtin was not abandoned")
				}
			},
		)
	}
}
