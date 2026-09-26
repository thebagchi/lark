// Regression probes for running a script: how long a duration may be, and
// what a held name refuses.
package testing_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

func TestDuration_Boundary(t *testing.T) {
	seconds := float64(uint64(math.MaxInt64)+1) / float64(time.Second)
	got, err := scheduler.Duration(starlark.Float(seconds))
	t.Logf("seconds=%g duration=%d err=%v", seconds, got, err)
	if err == nil && got < 0 {
		t.Errorf("duration past int64 was accepted as %d", got)
	}
}

func TestDuration_PastInt64IsRefused(t *testing.T) {
	seconds := float64(uint64(math.MaxInt64)+1) / float64(time.Second)
	_, err := scheduler.Duration(starlark.Float(seconds))
	if !errors.Is(err, scheduler.ErrDuration) {
		t.Fatalf("got %v, want ErrDuration", err)
	}
}

func _Run(t *testing.T, src string) (starlark.Value, error) {
	t.Helper()

	built, err := runtime.NewCompiler().Compile("fixed.star", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	return built.Run(context.Background())
}

func TestState_SetOfAnotherNameInsideUpdateIsRefused(t *testing.T) {
	_, err := _Run(t, `
def main():
    def change(v):
        state.set("b", 1)
        return v
    state.update("a", change)
    return "reached"
`)
	if !errors.Is(err, scheduler.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}
}

func TestJoin_UnderTheLockIsRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := time.Now()
	_, err := _RunCtx(t, ctx, `
def rival():
    sleep(0.05)
    return state.update("k", lambda v: "rival")

def main():
    handle = spawn(rival)
    return state.update("k", lambda v: join(handle))
`)
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("join under the lock ran for %s: %v", time.Since(started), err)
	}
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}
	if !strings.Contains(err.Error(), "join") {
		t.Fatalf("want the refusal at join, got %v", err)
	}
}

func TestSpawn_InsideAnUpdateCarriesTheBan(t *testing.T) {
	_, err := _Run(t, `
def child():
    sleep(0.05)
    state.set("b", 1)

def main():
    found = []
    def change(v):
        found.append(spawn(child))
        return "done"
    state.update("a", change)
    return join(found[0])
`)
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("child set after the update: %v", err)
	}
	if !strings.Contains(err.Error(), "started inside an update") {
		t.Fatalf("got %v", err)
	}
}

func TestUpdate_ReturnIsFrozenAndGetIsACopy(t *testing.T) {
	_, err := _Run(t, `
def main():
    def change(v):
        return [1]
    made = state.update("k", change)
    made.append(2)
    return made
`)
	if err == nil {
		t.Fatal("append to the value update returned was accepted")
	}

	value, err := _Run(t, `
def main():
    def change(v):
        return [1]
    state.update("k", change)
    copied = state.get("k")
    copied.append(2)
    return [copied, state.get("k")]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "[[1, 2], [1]]" {
		t.Fatalf("get copy: %s", value.String())
	}
}

func _RunCtx(t *testing.T, ctx context.Context, src string) (starlark.Value, error) {
	t.Helper()
	built, err := runtime.NewCompiler().Compile("after.star", []byte(src))
	if err != nil {
		return nil, err
	}
	return built.Run(ctx)
}

const (
	// _SETTLE is how long main waits, in seconds as its script spells it, so an
	// unjoined child reaches its failure before the run ends. Without this the
	// exit would cancel the child first and the test would pass having proved
	// nothing.
	_SETTLE = 0.2

	// _ORPHAN_SLEEP is how long the abandoned child asks to sleep, in seconds
	// as its script spells it.
	_ORPHAN_SLEEP = 2

	// _KILLED_WITHIN is how long the run may take before its child is taken to
	// have been waited for rather than killed. Measured 2026-09-24: 0.205s
	// killed against 2.005s joined, so this sits between the two with room on
	// either side of it.
	_KILLED_WITHIN = time.Second
)

// TestSpawn_AnUnjoinedFailureDoesNotEndTheRun pins detached-thread semantics.
//
// Nobody asked the child for its value, so nothing is waiting to be told it
// failed - the same answer a goroutine, a Java thread with an uncaught
// exception or a Python daemon thread gives. The failure reaches the reporter;
// only the run's result is left alone.
//
// The child marks the store before it fails and main returns that mark, so a
// green run is evidence the child actually got there. A test that only checked
// the error would pass just as well if the child had never run at all.
//
// Revisions:
//   - 2026-09-24 21:26: initial creation
func TestSpawn_AnUnjoinedFailureDoesNotEndTheRun(t *testing.T) {
	src := fmt.Sprintf(`
def child():
    state.set("ran", True)

    return None + 1

def main():
    spawn(child)
    sleep(%v)

    return state.get("ran")
`, _SETTLE)

	got, err := _Run(t, src)
	if err != nil {
		t.Fatalf("an unjoined child's failure ended the run: %v", err)
	}

	if got != starlark.Bool(true) {
		t.Fatalf("main returned %v, so the child never reached its failure", got)
	}
}

// TestSpawn_AnUnjoinedChildIsKilledAtExit is the other half of the same rule.
//
// A run ends by cancelling its context and then waiting, so a child nobody
// joined is cut off rather than waited out. This is what keeps the rule above
// from being a way to abandon work that outlives the run - and it is why an
// unjoined failure could not become the run's result even if it were wanted:
// whether the child reaches its own failure first is a race with main
// returning.
//
// Revisions:
//   - 2026-09-24 21:26: initial creation
func TestSpawn_AnUnjoinedChildIsKilledAtExit(t *testing.T) {
	src := fmt.Sprintf(`
def child():
    sleep(%v)

    return "the exit waited"

def main():
    spawn(child)

    return "main finished"
`, _ORPHAN_SLEEP)

	started := time.Now()

	_, err := _Run(t, src)
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	taken := time.Since(started)
	if taken >= _KILLED_WITHIN {
		t.Fatalf("the run took %v, so the child was waited for rather than killed", taken)
	}
}
