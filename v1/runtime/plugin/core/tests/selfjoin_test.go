package tests

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// SELF_JOIN is a script in which the thread f joins its own handle H, and
	// main joins H. Before the fix f waited for itself for ever. Main spawns f,
	// because a top level may not, and hands H to f through keep and take, which
	// _Ran supplies.
	//
	// Revisions:
	//   - 2026-11-10 11:55: initial creation, from the breaker case scheduler-F1 (C116, B237,
	//     Q295)
	//   - 2026-11-11 22:54: spawns in main and hands H over with keep and take (C127, B274,
	//     Q383)
	SELF_JOIN = `
def f():
    return join(take())

def main():
    H = spawn(f)
    keep(H)
    return join(H)
`

	// OTHER_JOIN is a script in which the thread f joins the handle of a
	// thread it spawned, which is not its own and must still work.
	//
	// Revisions:
	//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
	OTHER_JOIN = `
def g():
    return 5

def f():
    return join(spawn(g))

def main():
    return join(spawn(f))
`
)

// _Answer is what a script's main returned.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
type _Answer struct {
	value starlark.Value
	err   error
}

// _Handing adds keep and take to the builtins of core. keep closes a channel
// after it stores the value, so a take that sees the channel closed reads a
// value written before it.
//
// Revisions:
//   - 2026-11-11 22:54: initial creation (C127, B274, Q383)
func _Handing(builtins starlark.StringDict) starlark.StringDict {
	var (
		ready = make(chan struct{})
		once  sync.Once
		held  starlark.Value
	)

	builtins["keep"] = starlark.NewBuiltin(
		"keep",
		func(
			_ *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			_ []starlark.Tuple,
		) (starlark.Value, error) {
			once.Do(func() {
				held = args[0]

				close(ready)
			})

			return starlark.None, nil
		},
	)

	builtins["take"] = starlark.NewBuiltin(
		"take",
		func(
			_ *starlark.Thread,
			_ *starlark.Builtin,
			_ starlark.Tuple,
			_ []starlark.Tuple,
		) (starlark.Value, error) {
			select {
			case <-ready:
				return held, nil

			case <-time.After(LIMIT):
				return nil, errors.New("take: nothing was kept")
			}
		},
	)

	return builtins
}

// _Ran runs source on a new run, calls main, and returns what main returned.
// It fails the test when the run has not ended within LIMIT, which is how a
// thread that waits for itself shows. It supplies keep, which stores a value,
// and take, which waits for it, so that one thread can hand a handle to another.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
//   - 2026-11-11 22:54: supplies keep and take (C127, B274, Q383)
func _Ran(
	t *testing.T,
	source string,
) (starlark.Value, error) {
	t.Helper()

	var (
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		ctx, cancel = context.WithCancel(t.Context())
		finish      = scheduler.Begin(
			ctx,
			thread,
			ENTRY,
			nil,
		)
		done = make(
			chan _Answer,
			1,
		)
		predeclared = _Handing(core.Builtins())
	)

	defer cancel()

	go func() {
		globals, err := starlark.ExecFileOptions(
			common.OPTIONS,
			thread,
			SCRIPT_NAME,
			[]byte(source),
			predeclared,
		)
		if err != nil {
			done <- _Answer{
				nil,
				err,
			}

			return
		}

		value, err := starlark.Call(
			thread,
			globals[ENTRY],
			nil,
			nil,
		)

		done <- _Answer{
			value,
			err,
		}
	}()

	select {
	case got := <-done:
		_ = finish(got.err)

		return got.value, got.err
	case <-time.After(LIMIT):
		t.Fatal("hang: the script did not end")

		return nil, nil
	}
}

// TestJoin_RefusesTheHandleOfTheCallingThread is Q295: a thread that joins its
// own handle fails at once with an error, and the join of main, which waits
// for that thread, reports it.
//
// The check: go test -race -run TestJoin_RefusesTheHandleOfTheCallingThread
// ./v1/runtime/plugin/core/tests. Before the fix the script hangs for LIMIT.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
//   - 2026-11-10 17:15: uses ErrSelf, formerly ErrSelfJoin (C121, Q348)
func TestJoin_RefusesTheHandleOfTheCallingThread(t *testing.T) {
	_, err := _Ran(
		t,
		SELF_JOIN,
	)
	if err == nil {
		t.Fatal("joining its own handle succeeded")
	}

	if !strings.Contains(
		err.Error(),
		builtin.ErrSelf.Error(),
	) {
		t.Fatalf(
			"got %v, want it to say %q",
			err,
			builtin.ErrSelf,
		)
	}
}

// TestJoin_StillJoinsTheHandleOfAnotherThread proves the refusal is for the
// calling thread's own handle only: a thread joins one it spawned.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
func TestJoin_StillJoinsTheHandleOfAnotherThread(t *testing.T) {
	got, err := _Ran(
		t,
		OTHER_JOIN,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "[[5]]" {
		t.Fatalf(
			"got %v, want [[5]]",
			got,
		)
	}
}
