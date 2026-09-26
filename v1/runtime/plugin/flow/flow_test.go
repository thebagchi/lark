package flow_test

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	FIXTURE_DIR = "testdata"
	BUDGET      = 5 * time.Second

	// PROMPT is how long a run that should end at once may take on a loaded
	// machine before it is called hung.
	PROMPT = 1500 * time.Millisecond

	// PANIC_TEXT is what the exploding builtin panics with.
	PANIC_TEXT = "a plugin blew up"

	// ZERO asks a wrapper for no calls at all, and LAMBDAS wraps an anonymous
	// function, which is what a compiler emits for a site that passes
	// arguments.
	ZERO    = "zero.star"
	LAMBDAS = "lambdas.star"
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
		"explode": starlark.NewBuiltin("explode", func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			panic(PANIC_TEXT)
		}),
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
func (d *_Disk) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the fixture at name.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (d *_Disk) Load(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(FIXTURE_DIR, name))
}

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Built(t *testing.T, name string) *runtime.Artifact {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	built, err := runtime.NewCompiler(runtime.WithLoader(&_Disk{})).Compile(name, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built
}

// _Run compiles and runs the named fixture.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-21 08:09: compiles through _Built
func _Run(t *testing.T, name string) (starlark.Value, error) {
	t.Helper()

	return _Built(t, name).Run(t.Context())
}

// _Value runs a fixture that is expected to succeed.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func _Value(t *testing.T, name string) string {
	t.Helper()

	value, err := _Run(t, name)
	if err != nil {
		t.Fatalf("run %s: %v", name, err)
	}

	return value.String()
}

// TestRepeat_CallsExactlyThatManyTimes proves the count is exact and that n()
// reads the attempt.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestRepeat_CallsExactlyThatManyTimes(t *testing.T) {
	if got := _Value(t, "repeat.star"); got != "[3, 3]" {
		t.Fatalf("got %s, want [3, 3]", got)
	}
}

// TestRepeat_StopsAtTheFirstError proves it does not run on after a failure.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestRepeat_StopsAtTheFirstError(t *testing.T) {
	_, err := _Run(t, "repeat_stops.star")
	if err == nil {
		t.Fatal("a failing repeat succeeded")
	}

	if !strings.Contains(err.Error(), "attempt 1") {
		t.Fatalf("stopped somewhere other than the first attempt: %v", err)
	}
}

// TestRetry_StopsAtTheFirstSuccess proves an assertion fails one attempt rather
// than the run, which is the whole of what retry needs from assert.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func TestRetry_StopsAtTheFirstSuccess(t *testing.T) {
	if got := _Value(t, "retry.star"); got != "3" {
		t.Fatalf("got %s, want 3", got)
	}
}

// TestRetry_PropagatesTheLastAssertion proves a retry that never succeeds ends
// the run exactly as a bare assertion would.
//
// Revisions:
//   - 2026-09-20 01:40: initial creation
func TestRetry_PropagatesTheLastAssertion(t *testing.T) {
	_, err := _Run(t, "retry_gives_up.star")
	if !errors.Is(err, core.ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}

	if !strings.Contains(err.Error(), "3 attempts") {
		t.Fatalf("does not say how many attempts it made: %v", err)
	}

	t.Logf("gave up: %v", err)
}

// TestRetry_DoesNotRetryAFail proves the distinction that makes assert and fail
// two different things: one says a check did not hold, the other that this
// cannot work.
//
// Revisions:
//   - 2026-09-20 01:41: initial creation
func TestRetry_DoesNotRetryAFail(t *testing.T) {
	_, err := _Run(t, "retry_ignores_fail.star")
	if err == nil {
		t.Fatal("a fail was retried into a success")
	}

	if !strings.Contains(err.Error(), "attempt 1") {
		t.Fatalf("a fail was attempted more than once: %v", err)
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
	started := time.Now()

	_, err := _Run(t, "timeout.star")
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("got %v, want a timeout", err)
	}

	if time.Since(started) > BUDGET {
		t.Fatalf("waited %s for a call it was bounding", time.Since(started))
	}
}

// TestTimeout_LetsAQuickCallThrough proves the bound is a limit, not a wait.
//
// Revisions:
//   - 2026-09-20 01:43: initial creation
func TestTimeout_LetsAQuickCallThrough(t *testing.T) {
	if got := _Value(t, "timeout_ok.star"); got != `"finished in time"` {
		t.Fatalf("got %s", got)
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
		t.Run(name, func(t *testing.T) {
			_, err := _Run(t, name)
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}

			t.Logf("refused: %v", err)
		})
	}
}

// TestAttempt_RefusesOutsideAWrapper proves n() does not invent a number where
// there is no attempt to count.
//
// Revisions:
//   - 2026-09-20 01:45: initial creation
func TestAttempt_RefusesOutsideAWrapper(t *testing.T) {
	_, err := _Run(t, "bare_n.star")
	if err == nil {
		t.Fatal("n() answered outside a wrapper")
	}

	if !strings.Contains(err.Error(), "only meaningful inside") {
		t.Fatalf("refused for some other reason: %v", err)
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
	value, err := _Run(t, LAMBDAS)
	if err != nil {
		t.Fatalf("want a wrapped lambda to run, got %v", err)
	}

	if value.String() != "1" {
		t.Fatalf("want the lambda's own result, got %s", value)
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
	_, err := _Run(t, "explodes_inside_timeout.star")
	if err == nil || !strings.Contains(err.Error(), PANIC_TEXT) {
		t.Fatalf("want the panic as the run's failure, got %v", err)
	}
}

// TestTimeout_CutsAJoinShort is review defect C: a timeout used to wait a
// join out, because the spawn inside derived from the run rather than from
// the bounded evaluation and the join watched nothing but the handle.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestTimeout_CutsAJoinShort(t *testing.T) {
	started := time.Now()

	_, err := _Run(t, "timeout_over_join.star")
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("got %v, want a timeout", err)
	}

	if time.Since(started) > PROMPT {
		t.Fatalf("a 0.2 second timeout took %s", time.Since(started))
	}
}

// TestRetry_AnExhaustedInnerRetryIsOneFailedAttemptOfTheOuter is review
// defect D: giving up used to end the run outright, so an outer retry made one
// attempt where it should have made three.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestRetry_AnExhaustedInnerRetryIsOneFailedAttemptOfTheOuter(t *testing.T) {
	built := _Built(t, "nested_retry.star")

	_, err := built.Run(t.Context())
	if !errors.Is(err, core.ErrAssert) {
		t.Fatalf("want the last assertion to end the run, got %v", err)
	}

	if !strings.Contains(err.Error(), "retry(3, inner) gave up after 3 attempts") {
		t.Fatalf("want three outer attempts, got %v", err)
	}
}

// TestRetry_AThreadSpawnedInsideAnAttemptIsInsideIt is review defect I: n()
// used to fail in a spawned thread, and an assertion there ended the run.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestRetry_AThreadSpawnedInsideAnAttemptIsInsideIt(t *testing.T) {
	if got := _Value(t, "spawn_inside_retry.star"); got != "2" {
		t.Fatalf("want the spawned check to succeed on attempt 2, got %s", got)
	}
}

// TestSleep_RefusesWhatNoTimerHolds is review defect J.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestSleep_RefusesWhatNoTimerHolds(t *testing.T) {
	_, err := _Run(t, "huge_sleep.star")
	if !errors.Is(err, scheduler.ErrDuration) {
		t.Fatalf("want ErrDuration, got %v", err)
	}
}
