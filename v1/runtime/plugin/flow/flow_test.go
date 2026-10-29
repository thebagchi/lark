package flow_test

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	FIXTURE_DIR = "testdata"
	BUDGET      = 5 * time.Second

	// PROMPT is how long a run that should end at once may take on a loaded
	// machine before it is called hung.
	PROMPT = 1500 * time.Millisecond

	// PAUSE is the delay the delayed fixtures wait between their two calls:
	// long beside what a call costs, so one pause is told apart from none and
	// from two on a loaded machine.
	PAUSE = 300 * time.Millisecond

	// PANIC_TEXT is what the exploding builtin panics with.
	PANIC_TEXT = "a plugin blew up"

	// ZERO asks a wrapper for no calls at all, and LAMBDAS wraps an anonymous
	// function, which is what a compiler emits for a site that passes
	// arguments.
	ZERO    = "zero.star"
	LAMBDAS = "lambdas.star"

	// SPARE is how many threads' worth a run gets when the budget must not be
	// what fails it: an attempt, the thread it spawned, and one more.
	SPARE = 4
)

// _Disk is a Loader over testdata.
type _Disk struct{}

// _Exploding is a plugin whose one builtin panics, which is what a buggy
// plugin does.
type _Exploding struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func (e *_Exploding) Name() string {
	return "exploding"
}

// Values returns a builtin that panics when a script calls it.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func (e *_Exploding) Values() starlark.StringDict {
	return starlark.StringDict{
		"explode": starlark.NewBuiltin(
			"explode",
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				panic(PANIC_TEXT)
			},
		),
	}
}

// init installs the exploding plugin beside the real ones.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func init() {
	plugin.Register(&_Exploding{})
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (d *_Disk) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load reads the fixture at name.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (d *_Disk) Load(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(
		FIXTURE_DIR,
		name,
	))
}

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Built(
	t *testing.T,
	name string,
) *runtime.Artifact {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(
		FIXTURE_DIR,
		name,
	))
	if err != nil {
		t.Fatalf(
			"read fixture %s: %v",
			name,
			err,
		)
	}

	built, err := runtime.Compile(&runtime.Source{
		Entry:  name,
		Text:   src,
		Loader: &_Disk{},
	})
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	return built
}

// _Run compiles and runs the named fixture.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-21 08:09: compiles through _Built
func _Run(
	t *testing.T,
	name string,
) (starlark.Value, error) {
	t.Helper()

	return runtime.Start(
		t.Context(),
		_Built(
			t,
			name,
		),
	).Wait()
}

// _Value runs a fixture that is expected to succeed.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func _Value(
	t *testing.T,
	name string,
) string {
	t.Helper()

	value, err := _Run(
		t,
		name,
	)
	if err != nil {
		t.Fatalf(
			"run %s: %v",
			name,
			err,
		)
	}

	return value.String()
}

// TestRepeat_CallsExactlyThatManyTimes proves the count is exact and that n()
// reads the attempt.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestRepeat_CallsExactlyThatManyTimes(t *testing.T) {
	if got := _Value(
		t,
		"repeat.star",
	); got != "[3, 3]" {
		t.Fatalf(
			"got %s, want [3, 3]",
			got,
		)
	}
}

// TestRepeat_StopsAtTheFirstError proves it does not run on after a failure.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestRepeat_StopsAtTheFirstError(t *testing.T) {
	_, err := _Run(
		t,
		"repeat_stops.star",
	)
	if err == nil {
		t.Fatal("a failing repeat succeeded")
	}

	if !strings.Contains(
		err.Error(),
		"attempt 1",
	) {
		t.Fatalf(
			"stopped somewhere other than the first attempt: %v",
			err,
		)
	}
}

// TestRetry_StopsAtTheFirstSuccess proves an assertion fails one attempt rather
// than the run, which is the whole of what retry needs from assert.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func TestRetry_StopsAtTheFirstSuccess(t *testing.T) {
	if got := _Value(
		t,
		"retry.star",
	); got != "3" {
		t.Fatalf(
			"got %s, want 3",
			got,
		)
	}
}

// TestRetry_PropagatesTheLastAssertion proves a retry that never succeeds ends
// the run exactly as a bare assertion would.
//
// Revisions:
//   - 2026-09-20 01:40: initial creation
func TestRetry_PropagatesTheLastAssertion(t *testing.T) {
	_, err := _Run(
		t,
		"retry_gives_up.star",
	)
	if !errors.Is(
		err,
		core.ErrAssert,
	) {
		t.Fatalf(
			"got %v, want ErrAssert",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"3 attempts",
	) {
		t.Fatalf(
			"does not say how many attempts it made: %v",
			err,
		)
	}

	t.Logf(
		"gave up: %v",
		err,
	)
}

// TestRetry_DoesNotRetryAFail proves the distinction that makes assert and fail
// two different things: one says a check did not hold, the other that this
// cannot work.
//
// Revisions:
//   - 2026-09-20 01:41: initial creation
func TestRetry_DoesNotRetryAFail(t *testing.T) {
	_, err := _Run(
		t,
		"retry_ignores_fail.star",
	)
	if err == nil {
		t.Fatal("a fail was retried into a success")
	}

	if !strings.Contains(
		err.Error(),
		"attempt 1",
	) {
		t.Fatalf(
			"a fail was attempted more than once: %v",
			err,
		)
	}
}

// TestTimeout_StopsAWaitingCall proves a timeout reaches a target blocked in
// sleep, which is the rule every blocking builtin here owes.
//
// The target sleeps for thirty seconds. If the cancel did not reach it, this
// would take thirty seconds rather than fifty milliseconds.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestTimeout_StopsAWaitingCall(t *testing.T) {
	var (
		started = time.Now()
		_, err  = _Run(
			t,
			"timeout.star",
		)
	)
	if !strings.Contains(
		err.Error(),
		"timed out",
	) {
		t.Fatalf(
			"got %v, want a timeout",
			err,
		)
	}

	if time.Since(started) > BUDGET {
		t.Fatalf(
			"waited %s for a call it was bounding",
			time.Since(started),
		)
	}
}

// TestTimeout_LetsAQuickCallThrough proves the bound is a limit, not a wait.
//
// Revisions:
//   - 2026-09-20 01:43: initial creation
func TestTimeout_LetsAQuickCallThrough(t *testing.T) {
	if got := _Value(
		t,
		"timeout_ok.star",
	); got != `"finished in time"` {
		t.Fatalf(
			"got %s",
			got,
		)
	}
}

// TestFactories_RefuseACountBelowOne covers the refusal every wrapper shares: a
// count below one asks for nothing to happen.
//
// A lambda used to be refused here too. It is accepted now: a compiler emits
// one wherever a call passes arguments, since a wrapper takes none to pass on.
// What it costs is the name, and lambdas.star is inverted into a check that it
// runs rather than a check that it is turned away.
//
// Revisions:
//   - 2026-09-20 01:44: initial creation
//   - 2026-09-20 20:53: lambdas are accepted; only a bad count is refused
func TestFactories_RefuseACountBelowOne(t *testing.T) {
	for _, name := range []string{ZERO} {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := _Run(
					t,
					name,
				)
				if err == nil {
					t.Fatalf(
						"%s was accepted",
						name,
					)
				}

				t.Logf(
					"refused: %v",
					err,
				)
			},
		)
	}
}

// TestAttempt_RefusesOutsideAWrapper proves n() does not invent a number where
// there is no attempt to count.
//
// Revisions:
//   - 2026-09-20 01:45: initial creation
func TestAttempt_RefusesOutsideAWrapper(t *testing.T) {
	_, err := _Run(
		t,
		"bare_n.star",
	)
	if err == nil {
		t.Fatal("n() answered outside a wrapper")
	}

	if !strings.Contains(
		err.Error(),
		"only meaningful inside",
	) {
		t.Fatalf(
			"refused for some other reason: %v",
			err,
		)
	}
}

// TestFactories_TakeALambda is the other half of the refusal that went.
//
// A compiler emits a lambda wherever a call passes arguments, because a wrapper
// takes none to pass on: repeat(3, lambda: greet("alice")). What that costs is
// the name, and the name is not the wrapper's to supply.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
func TestFactories_TakeALambda(t *testing.T) {
	value, err := _Run(
		t,
		LAMBDAS,
	)
	if err != nil {
		t.Fatalf(
			"want a wrapped lambda to run, got %v",
			err,
		)
	}

	if value.String() != "1" {
		t.Fatalf(
			"want the lambda's own result, got %s",
			value,
		)
	}
}

// TestTimeout_APanicInsideItFailsTheRunNotTheProcess is review defect A.
//
// A wrapper used to call starlark.Call on an unguarded goroutine, where a
// panic cannot be recovered from outside - so this script killed the host.
// Without the guard this test does not fail; it takes the test binary down.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestTimeout_APanicInsideItFailsTheRunNotTheProcess(t *testing.T) {
	_, err := _Run(
		t,
		"explodes_inside_timeout.star",
	)
	if err == nil || !strings.Contains(
		err.Error(),
		PANIC_TEXT,
	) {
		t.Fatalf(
			"want the panic as the run's failure, got %v",
			err,
		)
	}
}

// TestTimeout_CutsAJoinShort is review defect C: a timeout used to wait a
// join out, because the spawn inside derived from the run rather than from
// the bounded evaluation and the join watched nothing but the handle.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestTimeout_CutsAJoinShort(t *testing.T) {
	var (
		started = time.Now()
		_, err  = _Run(
			t,
			"timeout_over_join.star",
		)
	)
	if !strings.Contains(
		err.Error(),
		"timed out",
	) {
		t.Fatalf(
			"got %v, want a timeout",
			err,
		)
	}

	if time.Since(started) > PROMPT {
		t.Fatalf(
			"a 0.2 second timeout took %s",
			time.Since(started),
		)
	}
}

// TestRetry_AnExhaustedInnerRetryIsOneFailedAttemptOfTheOuter is review
// defect D: giving up used to end the run outright, so an outer retry made one
// attempt where it should have made three.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestRetry_AnExhaustedInnerRetryIsOneFailedAttemptOfTheOuter(t *testing.T) {
	var (
		built = _Built(
			t,
			"nested_retry.star",
		)
		_, err = runtime.Start(
			t.Context(),
			built,
		).Wait()
	)
	if !errors.Is(
		err,
		core.ErrAssert,
	) {
		t.Fatalf(
			"want the last assertion to end the run, got %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"retry(3, inner) gave up after 3 attempts",
	) {
		t.Fatalf(
			"want three outer attempts, got %v",
			err,
		)
	}
}

// TestRetry_AThreadSpawnedInsideAnAttemptIsInsideIt is review defect I: n()
// used to fail in a spawned thread, and an assertion there ended the run.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestRetry_AThreadSpawnedInsideAnAttemptIsInsideIt(t *testing.T) {
	if got := _Value(
		t,
		"spawn_inside_retry.star",
	); got != "2" {
		t.Fatalf(
			"want the spawned check to succeed on attempt 2, got %s",
			got,
		)
	}
}

// TestSleep_RefusesWhatNoTimerHolds is review defect J.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestSleep_RefusesWhatNoTimerHolds(t *testing.T) {
	_, err := _Run(
		t,
		"huge_sleep.star",
	)
	if !errors.Is(
		err,
		scheduler.ErrDuration,
	) {
		t.Fatalf(
			"want ErrDuration, got %v",
			err,
		)
	}
}

// TestDelay_WaitsBetweenCallsOnly checks a delay is waited between two calls,
// third or by name, and neither before the first call nor after the last.
//
// Each fixture makes two calls, so it waits exactly once. Less than one pause
// means it did not wait; two means it waited before the first call or after
// the last.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func TestDelay_WaitsBetweenCallsOnly(t *testing.T) {
	for _, name := range []string{"delayed_repeat.star", "delayed_retry.star"} {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					started = time.Now()
					got     = _Value(
						t,
						name,
					)
					took = time.Since(started)
				)

				if got != "2" {
					t.Fatalf(
						"got %s, want the second call's 2",
						got,
					)
				}

				once := took >= PAUSE && took < 2*PAUSE
				if !once {
					t.Fatalf(
						"two calls took %s, want one pause of %s",
						took,
						PAUSE,
					)
				}
			},
		)
	}
}

// TestDelay_EndsWithACancel checks a cancel reaches a repeat waiting out its
// delay, so the next call never starts: a timeout of 200 milliseconds around
// a delay of thirty seconds.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func TestDelay_EndsWithACancel(t *testing.T) {
	var (
		started = time.Now()
		_, err  = _Run(
			t,
			"delay_over_timeout.star",
		)
	)
	if err == nil || !strings.Contains(
		err.Error(),
		"timed out",
	) {
		t.Fatalf(
			"got %v, want a timeout",
			err,
		)
	}

	if time.Since(started) > PROMPT {
		t.Fatalf(
			"a 200 millisecond timeout took %s",
			time.Since(started),
		)
	}
}

// TestDelay_RefusesOneGivenTwice checks a delay passed third and named as well
// is refused, rather than one of the two silently winning.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func TestDelay_RefusesOneGivenTwice(t *testing.T) {
	_, err := _Run(
		t,
		"delay_twice.star",
	)
	if err == nil || !strings.Contains(
		err.Error(),
		"multiple values",
	) {
		t.Fatalf(
			"got %v, want the delay refused as given twice",
			err,
		)
	}
}

// TestDelay_RefusesAFractionOfAMillisecond checks a delay is read as sleep and
// timeout are, in whole milliseconds the schema can hold.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func TestDelay_RefusesAFractionOfAMillisecond(t *testing.T) {
	_, err := _Run(
		t,
		"delay_fraction.star",
	)
	if !errors.Is(
		err,
		scheduler.ErrDuration,
	) {
		t.Fatalf(
			"got %v, want ErrDuration",
			err,
		)
	}
}

// TestWrappers_ReportOneLine checks each wrapper reports one line under the
// function it calls: opened by the first attempt, advanced by each later one
// with the count it makes, and closed once, however many attempts failed. A
// lambda that calls no function the script defines names none.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func TestWrappers_ReportOneLine(t *testing.T) {
	cases := map[string]struct {
		fails bool
		want  []string
	}{
		"repeat.star": {
			want: []string{
				`start "step" repeat 1/3`,
				`start "step" repeat 2/3`,
				`start "step" repeat 3/3`,
				`end "step" succeeded`,
			},
		},
		"retry.star": {
			want: []string{
				`start "flaky" retry 1/5`,
				`start "flaky" retry 2/5`,
				`start "flaky" retry 3/5`,
				`end "flaky" succeeded`,
			},
		},
		"retry_gives_up.star": {
			fails: true,
			want: []string{
				`start "never" retry 1/3`,
				`start "never" retry 2/3`,
				`start "never" retry 3/3`,
				`end "never" failed`,
			},
		},
		"timeout_ok.star": {
			want: []string{
				`start "quick" timeout 0/0`,
				`end "quick" succeeded`,
			},
		},
		LAMBDAS: {
			want: []string{
				`start "" repeat 1/2`,
				`start "" repeat 2/2`,
				`end "" succeeded`,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					into   = new(_Lines)
					_, err = artifact.Run(
						t.Context(),
						_Built(
							t,
							name,
						),
						artifact.Reporting(into),
					)
				)
				if (err != nil) != tc.fails {
					t.Fatalf(
						"run gave %v",
						err,
					)
				}

				got := into._All()
				if strings.Join(
					got,
					"\n",
				) != strings.Join(
					tc.want,
					"\n",
				) {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// _Lines is a reporter that keeps, one line of text each, every start a
// builtin reported and every end but the entry's.
type _Lines struct {
	guard sync.Mutex
	lines []string
}

// Started keeps a start some builtin reported.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func (l *_Lines) Started(
	thread string,
	line *scheduler.Line,
) {
	if line.Builtin == "" {
		return
	}

	l.guard.Lock()
	defer l.guard.Unlock()

	kept := fmt.Sprintf(
		"start %q %s %d/%d",
		line.Name,
		line.Builtin,
		line.Attempt,
		line.Count,
	)

	l.lines = append(
		l.lines,
		kept,
	)
}

// Ended keeps an end, unless it is the entry's.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func (l *_Lines) Ended(
	thread string,
	name string,
	err error,
) {
	if name == spelling.ENTRY {
		return
	}

	l.guard.Lock()
	defer l.guard.Unlock()

	outcome := "succeeded"
	if err != nil {
		outcome = "failed"
	}

	l.lines = append(
		l.lines,
		fmt.Sprintf(
			"end %q %s",
			name,
			outcome,
		),
	)
}

// _All is everything kept so far.
//
// Revisions:
//   - 2026-10-02 01:04: initial creation
func (l *_Lines) _All() []string {
	l.guard.Lock()
	defer l.guard.Unlock()

	return append(
		[]string(nil),
		l.lines...,
	)
}

// TestRegression_Spawn proves a spawn inside a successful wrapper lives until
// the run ends: the worker sleeps, the retry succeeds, and main waits long
// enough for the worker to write its result.
//
// Before the fix, the attempt's context was cancelled when the attempt
// returned, and the spawned worker was cancelled before it could finish.
//
// Revisions:
//   - 2026-10-24 19:30: initial creation
func TestRegression_Spawn(t *testing.T) {
	got := _Value(
		t,
		"spawn_survives_retry.star",
	)
	if got != `"yes"` {
		t.Fatalf(
			"got %s, want the worker to finish after the retry returned",
			got,
		)
	}
}

// TestWrappers_NameTheCallInAFailure pins the text a failed repeat or retry
// gives: the call as the script wrote it, then which attempt failed or how
// many it made. The call is built only when one fails, and this is what keeps
// that text the same.
//
// Revisions:
//   - 2026-10-29 00:23: initial creation
func TestWrappers_NameTheCallInAFailure(t *testing.T) {
	cases := map[string]string{
		"repeat_stops.star":       "repeat(5, step) attempt 1: ",
		"retry_ignores_fail.star": "retry(5, broken) attempt 1: ",
		"retry_gives_up.star":     "retry(3, never) gave up after 3 attempts: ",
	}

	for name, want := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := _Run(
					t,
					name,
				)
				if err == nil || !strings.Contains(
					err.Error(),
					want,
				) {
					t.Fatalf(
						"got %v, want it to say %q",
						err,
						want,
					)
				}
			},
		)
	}
}

// TestAttempt_ACancelStaysWithItsCaller pins where a cancel reaches an attempt.
// A timeout around a retry stops the attempt at once, and an assertion that
// ends the run stops an attempt that sleeps. Neither reaches a thread outside
// the timeout, which goes on to return.
//
// Each attempt sleeps three seconds, so a cancel that did not reach it would
// hold the run past PROMPT. The lines are compared without their order, since
// a cancel ends lines on more than one goroutine.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func TestAttempt_ACancelStaysWithItsCaller(t *testing.T) {
	cases := map[string]struct {
		fails bool
		want  string
		lines []string
	}{
		"timeout_over_retry.star": {
			fails: true,
			want:  "timeout(200, waits): timed out",
			lines: []string{
				`start "waits" timeout 0/0`,
				`start "slow" retry 1/3`,
				`start "" sleep 0/0`,
				`end "" failed`,
				`end "slow" failed`,
				`end "waits" failed`,
			},
		},
		"failure_stops_retry.star": {
			fails: true,
			want:  `"a sibling broke": assertion failed`,
			lines: []string{
				`start "broken" spawn 0/0`,
				`start "slow" retry 1/3`,
				`start "" sleep 0/0`,
				`start "" sleep 0/0`,
				`end "" succeeded`,
				`end "broken" failed`,
				`end "" failed`,
				`end "slow" failed`,
			},
		},
		"timeout_beside_main.star": {
			want: `"main lives"`,
			lines: []string{
				`start "bounded" spawn 0/0`,
				`start "" sleep 0/0`,
				`start "waits" timeout 0/0`,
				`start "slow" retry 1/3`,
				`start "" sleep 0/0`,
				`end "" failed`,
				`end "slow" failed`,
				`end "waits" failed`,
				`end "bounded" failed`,
				`end "" succeeded`,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					into    = new(_Lines)
					started = time.Now()
					got     = _Outcome(
						t,
						name,
						artifact.Reporting(into),
					)
					took = time.Since(started)
				)

				_Matches(
					t,
					got,
					&_Expected{
						fails: tc.fails,
						text:  tc.want,
					},
				)

				if took > PROMPT {
					t.Fatalf(
						"a cancel took %s to reach the attempt",
						took,
					)
				}

				lines := into._All()
				slices.Sort(lines)
				slices.Sort(tc.lines)

				if !slices.Equal(
					lines,
					tc.lines,
				) {
					t.Fatalf(
						"got %q, want %q",
						lines,
						tc.lines,
					)
				}
			},
		)
	}
}

// TestAttempt_APanicFailsTheRunOnce pins what a panic inside an attempt does:
// the run fails with the panic, the attempt is not made again, and the line
// opens once and ends failed.
//
// A panic is not an assertion, so a retry does not catch it.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func TestAttempt_APanicFailsTheRunOnce(t *testing.T) {
	cases := map[string][]string{
		"explodes_inside_retry.star": {
			`start "boom" retry 1/3`,
			`end "boom" failed`,
		},
		"explodes_inside_repeat.star": {
			`start "boom" repeat 1/2`,
			`end "boom" failed`,
		},
	}

	for name, want := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					into = new(_Lines)
					got  = _Outcome(
						t,
						name,
						artifact.Reporting(into),
					)
				)

				_Matches(
					t,
					got,
					&_Expected{
						fails: true,
						text:  PANIC_TEXT,
					},
				)

				lines := into._All()
				if !slices.Equal(
					lines,
					want,
				) {
					t.Fatalf(
						"got %q, want %q",
						lines,
						want,
					)
				}
			},
		)
	}
}

// TestAttempt_KeepsTheIdsOfASpawnInside pins every line a retry and a repeat
// report when each attempt spawns and joins, with the thread that reports it.
//
// An attempt reports on its caller's lane, and a spawn inside it is a child of
// that lane, numbered in turn across attempts.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func TestAttempt_KeepsTheIdsOfASpawnInside(t *testing.T) {
	want := []string{
		`thread_0 start "main"  0/0  []`,
		`thread_0 start "attempt" retry 1/3  []`,
		`thread_0 start "checks" spawn 0/0 thread_1 []`,
		`thread_0 start "" join 0/0  [thread_1]`,
		`thread_1 end "checks" failed`,
		`thread_0 end "" failed`,
		`thread_0 start "attempt" retry 2/3  []`,
		`thread_0 start "checks" spawn 0/0 thread_2 []`,
		`thread_0 start "" join 0/0  [thread_2]`,
		`thread_2 end "checks" succeeded`,
		`thread_0 end "" succeeded`,
		`thread_0 end "attempt" succeeded`,
		`thread_0 start "twice" repeat 1/2  []`,
		`thread_0 start "worker" spawn 0/0 thread_3 []`,
		`thread_0 start "" join 0/0  [thread_3]`,
		`thread_3 end "worker" succeeded`,
		`thread_0 end "" succeeded`,
		`thread_0 start "twice" repeat 2/2  []`,
		`thread_0 start "worker" spawn 0/0 thread_4 []`,
		`thread_0 start "" join 0/0  [thread_4]`,
		`thread_4 end "worker" succeeded`,
		`thread_0 end "" succeeded`,
		`thread_0 end "twice" succeeded`,
		`thread_0 end "main" succeeded`,
	}

	var (
		into = new(_Lanes)
		got  = _Outcome(
			t,
			"spawn_ids_retry.star",
			artifact.Reporting(into),
		)
	)

	_Matches(
		t,
		got,
		&_Expected{
			fails: false,
			text:  "[2, 2]",
		},
	)

	lines := into._All()
	if !slices.Equal(
		lines,
		want,
	) {
		t.Fatalf(
			"got %q, want %q",
			lines,
			want,
		)
	}
}

// TestAttempt_IsChargedAsAThread pins what an attempt costs the run's budget:
// one thread, held while the attempt runs and given back when it ends.
//
// One thread's worth is then enough for any number of attempts in turn, less
// than that refuses the first attempt, and a spawn inside an attempt needs
// more. A spawned thread gives its charge back after its join returns, so the
// run that succeeds has room for a thread the last attempt spawned as well.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func TestAttempt_IsChargedAsAThread(t *testing.T) {
	cases := map[string]struct {
		name   string
		memory int64
		fails  bool
		want   string
	}{
		"a retry under one thread": {
			name:   "retry.star",
			memory: scheduler.THREAD_COST - 1,
			fails:  true,
			want:   "retry(5, flaky) attempt 1: ",
		},
		"a repeat under one thread": {
			name:   "repeat.star",
			memory: scheduler.THREAD_COST - 1,
			fails:  true,
			want:   "repeat(3, step) attempt 1: ",
		},
		"a retry in one thread": {
			name:   "retry.star",
			memory: scheduler.THREAD_COST,
			want:   "3",
		},
		"a spawn inside in one thread": {
			name:   "spawn_ids_retry.star",
			memory: scheduler.THREAD_COST,
			fails:  true,
			want:   "retry(3, attempt) attempt 1: checks: ",
		},
		"a spawn inside with room to spare": {
			name:   "spawn_ids_retry.star",
			memory: SPARE * scheduler.THREAD_COST,
			want:   "[2, 2]",
		},
	}

	for label, tc := range cases {
		t.Run(
			label,
			func(t *testing.T) {
				got := _Outcome(
					t,
					tc.name,
					artifact.WithMemory(tc.memory),
				)

				_Matches(
					t,
					got,
					&_Expected{
						fails: tc.fails,
						text:  tc.want,
					},
				)

				refused := errors.Is(
					got.err,
					scheduler.ErrMemory,
				)
				if refused != tc.fails {
					t.Fatalf(
						"got %v, want ErrMemory only when it fails",
						got.err,
					)
				}
			},
		)
	}
}

// TestAttempt_SeesTheUpdateItIsInside pins what an attempt inside an update
// may do. It may not update or wait, since the update it is inside holds a
// name. An attempt outside one updates, and leaves its caller free to update
// after it.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func TestAttempt_SeesTheUpdateItIsInside(t *testing.T) {
	cases := map[string]struct {
		fails bool
		want  string
	}{
		"update_inside_attempt.star": {
			fails: true,
			want:  `state.update "y": started inside an update of "x"`,
		},
		"wait_inside_attempt.star": {
			fails: true,
			want:  `event.wait inside an update of "x"`,
		},
		"update_after_attempt.star": {
			want: "12",
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got := _Outcome(
					t,
					name,
				)

				_Matches(
					t,
					got,
					&_Expected{
						fails: tc.fails,
						text:  tc.want,
					},
				)

				nested := errors.Is(
					got.err,
					scheduler.ErrNested,
				)
				if nested != tc.fails {
					t.Fatalf(
						"got %v, want ErrNested only when it fails",
						got.err,
					)
				}
			},
		)
	}
}

// _Ran is what one run of a fixture gave: its value, or its error.
type _Ran struct {
	value starlark.Value
	err   error
}

// _Outcome runs the named fixture with opts and keeps what it gave.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func _Outcome(
	t *testing.T,
	name string,
	opts ...artifact.RunOption,
) *_Ran {
	t.Helper()

	value, err := artifact.Run(
		t.Context(),
		_Built(
			t,
			name,
		),
		opts...,
	)

	return &_Ran{
		value: value,
		err:   err,
	}
}

// _Expected is what a run of a fixture should give: whether it fails, and
// text its error or its value holds.
type _Expected struct {
	fails bool
	text  string
}

// _Matches fails the test unless got failed when want says so, with an error
// or a value whose text holds want's.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func _Matches(
	t *testing.T,
	got *_Ran,
	want *_Expected,
) {
	t.Helper()

	if (got.err != nil) != want.fails {
		t.Fatalf(
			"got %v and %v, want a failure: %t",
			got.value,
			got.err,
			want.fails,
		)
	}

	text := fmt.Sprint(got.err)
	if !want.fails {
		text = got.value.String()
	}

	if !strings.Contains(
		text,
		want.text,
	) {
		t.Fatalf(
			"got %s, want it to say %s",
			text,
			want.text,
		)
	}
}

// _Lanes is a reporter that keeps every start and every end, one line of
// text each, with the thread that reported it and the facts that tie lines to
// threads.
type _Lanes struct {
	guard sync.Mutex
	lines []string
}

// Started keeps a start, with its attempt, its child and what it waits on.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func (l *_Lanes) Started(
	thread string,
	line *scheduler.Line,
) {
	kept := fmt.Sprintf(
		"%s start %q %s %d/%d %s %v",
		thread,
		line.Name,
		line.Builtin,
		line.Attempt,
		line.Count,
		line.Child,
		line.Waits,
	)

	l._Keep(kept)
}

// Ended keeps an end, with whether it failed.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func (l *_Lanes) Ended(
	thread string,
	name string,
	err error,
) {
	outcome := "succeeded"
	if err != nil {
		outcome = "failed"
	}

	kept := fmt.Sprintf(
		"%s end %q %s",
		thread,
		name,
		outcome,
	)

	l._Keep(kept)
}

// _All is everything kept so far.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func (l *_Lanes) _All() []string {
	l.guard.Lock()
	defer l.guard.Unlock()

	return slices.Clone(l.lines)
}

// _Keep adds one line, under the lock, since lines arrive from every thread.
//
// Revisions:
//   - 2026-10-29 00:38: initial creation
func (l *_Lanes) _Keep(kept string) {
	l.guard.Lock()
	defer l.guard.Unlock()

	l.lines = append(
		l.lines,
		kept,
	)
}
