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

	"github.com/thebagchi/lark/runtime"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/state"
	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	FIXTURE_DIR = "testdata"
	BUDGET      = 5 * time.Second
)

// _Disk is a Loader over testdata.
type _Disk struct{}

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

// _Run compiles and runs the named fixture.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func _Run(t *testing.T, name string) (starlark.Value, error) {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	built, err := runtime.NewCompiler(runtime.WithLoader(&_Disk{})).Compile(name, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built.Run(t.Context())
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
	if !errors.Is(err, scheduler.ErrAssert) {
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

// TestFactories_RefuseWhatTheyCannotName covers the two refusals every wrapper
// shares: a lambda has no name to report, and a count below one asks for
// nothing to happen.
//
// Revisions:
//   - 2026-09-20 01:44: initial creation
func TestFactories_RefuseWhatTheyCannotName(t *testing.T) {
	for _, name := range []string{"lambdas.star", "zero.star"} {
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
