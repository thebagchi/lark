package tests

import (
	"context"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

const (
	// SCRIPT_NAME is the file name the compiled test scripts carry.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constant in
	//     beside_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-12 18:58: names the script fork_test.star, formerly beside_test.star (C129,
	//     Q396)
	SCRIPT_NAME = "fork_test.star"

	// WORKER, SPINNER and ANON_FUNC name the globals of SOURCE, and ANSWER is
	// what WORKER returns.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constants in
	//     beside_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WORKER    = "worker"
	SPINNER   = "spin"
	ANON_FUNC = "ANON"
	ANSWER    = "7"

	// SOURCE is the script the Fork tests spawn from: WORKER returns at once and
	// SPINNER counts for far longer than any bound the tests allow.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constant in
	//     beside_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
	SOURCE = `
ANON = lambda: 1

def worker():
    return 7

def spin():
    total = 0
    for i in range(100000000):
        total += i
    return total
`
)

// _Open starts a run on a new thread through run.Begin and returns the thread
// and a function that ends the run. The function is safe to call more than once,
// and the test calls it when it ends, so a test that does not need the end
// earlier never calls it.
//
// Revisions:
//   - 2026-11-08 22:45: initial creation, as the external form of _Started and
//     _Thread in run_internal_test.go (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Open(
	ctx context.Context,
	t *testing.T,
	settings *common.Settings,
) (*starlark.Thread, func()) {
	t.Helper()

	var (
		thread = &starlark.Thread{
			Name: SCRIPT_NAME,
		}
		finish = run.Begin(
			ctx,
			thread,
			common.ENTRY,
			settings,
		)
		once sync.Once
		end  = func() {
			once.Do(func() {
				err := finish(nil)
				if err != nil {
					t.Errorf(
						"finish: %v",
						err,
					)
				}
			})
		}
	)

	t.Cleanup(end)

	return thread, end
}

// _Callable compiles SOURCE and returns the named global as something Fork
// takes.
//
// Revisions:
//   - 2026-11-08 22:45: initial creation, as the external form of _Callable in
//     beside_internal_test.go (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func _Callable(
	t *testing.T,
	name string,
) starlark.Callable {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT_NAME,
		},
		SCRIPT_NAME,
		[]byte(SOURCE),
		starlark.StringDict{},
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	globals.Freeze()

	target, ok := globals[name].(starlark.Callable)
	if !ok {
		t.Fatalf(
			"%s is not callable",
			name,
		)
	}

	return target
}
