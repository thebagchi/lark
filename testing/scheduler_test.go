// Regression probes for running a script: how long a duration may be, and
// what a held name refuses.
package testing_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/scheduler"
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
