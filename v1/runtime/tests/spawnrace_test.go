package tests

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// TOP_SPAWN is a script whose top level spawns a thread that reads the
	// global H, then keeps the top level busy while that thread would read it.
	// Before the fix the two raced on the slot of H.
	//
	// Revisions:
	//   - 2026-11-11 22:39: initial creation (C127, B274, Q383)
	TOP_SPAWN = `
def f():
    for i in range(200):
        pass
    return H

H = spawn(f)

for i in range(200000):
    pass

def main():
    return join(H)
`

	// CALLED_SPAWN is a script whose top level calls a function that spawns:
	// that function runs while the top level does.
	//
	// Revisions:
	//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
	CALLED_SPAWN = `
def f():
    return 1

def start():
    return spawn(f)

H = start()

def main():
    return join(H)
`

	// LOADED_SPAWN is a script that loads a module whose top level spawns.
	//
	// Revisions:
	//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
	LOADED_SPAWN = `
load("module.star", "f")

def main():
    return 1
`

	// MODULE_SPAWN is the module LOADED_SPAWN loads.
	//
	// Revisions:
	//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
	MODULE_SPAWN = `
def f():
    return 1

spawn(f)
`

	// TIMEOUT_TEXT is the text of the refusal of a timeout while a module loads.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	TIMEOUT_TEXT = "timeout: cannot start a thread while the module loads " +
		"(call timeout from main or a function main calls)"

	// HINT ends the text of a refusal of a spawn in a flow.
	//
	// Revisions:
	//   - 2026-11-11 22:58: initial creation (C127, B274, Q383)
	HINT = "(spawn cannot start a thread while the module loads; " +
		"call spawn from main or a function main calls)"

	// TIMEOUT_HINT ends the text of a refusal of a timeout in a flow.
	//
	// Revisions:
	//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
	TIMEOUT_HINT = "(timeout cannot start a thread while the module loads; " +
		"call timeout from main or a function main calls)"

	// DERIVE_TIMEOUT_CALL is a script whose top level calls timeout.
	//
	// Revisions:
	//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
	DERIVE_TIMEOUT_CALL = `
def f():
    return 1

timeout(1000, f)

def main():
    return 1
`

	// DERIVE_TIMEOUT_BRANCH is a script whose top level calls timeout in a branch.
	//
	// Revisions:
	//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
	DERIVE_TIMEOUT_BRANCH = `
def f():
    return 1

if True:
    timeout(1000, f)

def main():
    return 1
`

	// DERIVE_TIMEOUT_MAIN is a script whose main calls timeout, which is fine.
	//
	// Revisions:
	//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
	DERIVE_TIMEOUT_MAIN = `
def f():
    return 1

def bounded():
    return timeout(1000, f)

def main():
    return bounded()
`

	// DERIVE_CALL is a script whose top level calls spawn.
	//
	// Revisions:
	//   - 2026-11-11 22:58: initial creation (C127, B274, Q383)
	DERIVE_CALL = `
def f():
    return 1

spawn(f)

def main():
    return 1
`

	// DERIVE_BRANCH is a script whose top level spawns in a branch.
	//
	// Revisions:
	//   - 2026-11-11 22:58: initial creation (C127, B274, Q383)
	DERIVE_BRANCH = `
def f():
    return 1

if True:
    spawn(f)

def main():
    return 1
`

	// DERIVE_LOOP is a script whose top level spawns in a loop.
	//
	// Revisions:
	//   - 2026-11-11 22:58: initial creation (C127, B274, Q383)
	DERIVE_LOOP = `
def f():
    return 1

for i in range(2):
    h = spawn(f)

def main():
    return 1
`

	// CHECK_SCRIPT is a script Derive reads into a flow that Check accepts, to
	// which the test then gives an init.
	//
	// Revisions:
	//   - 2026-11-11 22:58: initial creation (C127, B274, Q383)
	CHECK_SCRIPT = `
def f():
    return 1

def main():
    return join(spawn(f))
`

	// TOP_TIMEOUT is a script whose top level calls timeout, which runs its
	// function on a thread of its own while the top level runs.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	TOP_TIMEOUT = `
def work():
    return 1

R = timeout(1000, work)

def main():
    return R
`

	// CALLED_TIMEOUT is a script whose top level calls a function that calls
	// timeout.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	CALLED_TIMEOUT = `
def work():
    return 1

def bounded():
    return timeout(1000, work)

R = bounded()

def main():
    return R
`

	// LOADED_TIMEOUT is a script that loads a module whose top level calls
	// timeout.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	LOADED_TIMEOUT = `
load("timeout_module.star", "work")

def main():
    return 1
`

	// MODULE_TIMEOUT is the module LOADED_TIMEOUT loads.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	MODULE_TIMEOUT = `
def work():
    return 1

timeout(1000, work)
`

	// MAIN_TIMEOUT is a script that calls timeout from main, which is allowed.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
	MAIN_TIMEOUT = `
def work():
    return 1

def main():
    return timeout(1000, work)
`

	// MAIN_SPAWN is a script that spawns from main, which is allowed.
	//
	// Revisions:
	//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
	MAIN_SPAWN = `
def f():
    return 1

def main():
    return join(spawn(f))
`
)

// _Spawning is a Loader that serves two modules: MODULE_SPAWN as module.star and
// MODULE_TIMEOUT as timeout_module.star.
//
// Revisions:
//   - 2026-11-11 22:50: initial creation (C127, B274, Q383, Q389)
type _Spawning struct{}

// Resolve reads target as a name beside from.
//
// Revisions:
//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
func (m *_Spawning) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text of module.star or timeout_module.star.
//
// Revisions:
//   - 2026-11-11 22:50: initial creation (C127, B274, Q383, Q389)
func (m *_Spawning) Load(name string) ([]byte, error) {
	switch path.Base(name) {
	case "module.star":
		return []byte(MODULE_SPAWN), nil

	case "timeout_module.star":
		return []byte(MODULE_TIMEOUT), nil
	}

	return nil, fmt.Errorf(
		"read %s: no such module",
		name,
	)
}

// TestRun_ASpawnWhileTheModuleLoadsIsRefused proves spawn is refused while a
// module's top level runs - on the top level, in a function it calls, and in a
// module it loads - with runtime.ErrInit and a hint, before any thread starts,
// and that main may spawn. Each script runs 20 times, so under -race a thread
// that started would show the data race of B274 (Q383).
//
// The check: go test -race -count=40 -run TestRun_ASpawnWhileTheModuleLoadsIsRefused
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-11 22:39: initial creation, as TestRun_ASpawnAtTheTopLevelReadsAGlobalInitWrites;
//     refuses the spawn in place of racing (C127, B274, Q383)
func TestRun_ASpawnWhileTheModuleLoadsIsRefused(t *testing.T) {
	for _, item := range []struct {
		name string
		text string
		want error
	}{
		{
			"the top level",
			TOP_SPAWN,
			runtime.ErrInit,
		},
		{
			"a function the top level calls",
			CALLED_SPAWN,
			runtime.ErrInit,
		},
		{
			"a loaded module",
			LOADED_SPAWN,
			runtime.ErrInit,
		},
		{
			"main",
			MAIN_SPAWN,
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				built, err := runtime.Compile(&common.Source{
					Entry:  "spawning.star",
					Text:   []byte(item.text),
					Loader: &_Spawning{},
				})
				if err != nil {
					t.Fatal(err)
				}

				for range 20 {
					_, err = _Waited(
						t,
						built,
					)

					if !errors.Is(
						err,
						item.want,
					) {
						t.Fatalf(
							"err = %v, want %v",
							err,
							item.want,
						)
					}

					if item.want != nil && !strings.Contains(
						err.Error(),
						"(call spawn from main or a function main calls)",
					) {
						t.Fatalf(
							"err = %v, want the hint",
							err,
						)
					}
				}
			},
		)
	}
}

// _Waited starts a run of built and waits for it, for at most 20 seconds.
//
// Revisions:
//   - 2026-11-11 22:50: initial creation (C127, B274, Q383)
func _Waited(
	t *testing.T,
	built common.Artifact,
) (starlark.Value, error) {
	t.Helper()

	var (
		ctx, cancel = context.WithTimeout(
			t.Context(),
			20*time.Second,
		)
		run, err = runtime.Start(
			ctx,
			built,
		)
	)

	defer cancel()

	if err != nil {
		return nil, err
	}

	return run.Wait()
}

// TestRun_ATimeoutWhileTheModuleLoadsIsRefused proves timeout, which runs its
// function on a thread of its own, is refused while a module's top level runs -
// on the top level, in a function it calls, and in a module it loads - with
// runtime.ErrInit and a hint that names timeout, and that main may call it
// (Q389). Each script runs 20 times, under -race.
//
// The check: go test -race -count=40 -run TestRun_ATimeoutWhileTheModuleLoadsIsRefused
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
func TestRun_ATimeoutWhileTheModuleLoadsIsRefused(t *testing.T) {
	for _, item := range []struct {
		name string
		text string
		want error
	}{
		{
			"the top level",
			TOP_TIMEOUT,
			runtime.ErrInit,
		},
		{
			"a function the top level calls",
			CALLED_TIMEOUT,
			runtime.ErrInit,
		},
		{
			"a loaded module",
			LOADED_TIMEOUT,
			runtime.ErrInit,
		},
		{
			"main",
			MAIN_TIMEOUT,
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				built, err := runtime.Compile(&common.Source{
					Entry:  "timing.star",
					Text:   []byte(item.text),
					Loader: &_Spawning{},
				})
				if err != nil {
					t.Fatal(err)
				}

				for range 20 {
					_, err = _Waited(
						t,
						built,
					)

					if !errors.Is(
						err,
						item.want,
					) {
						t.Fatalf(
							"err = %v, want %v",
							err,
							item.want,
						)
					}

					if item.want != nil && !strings.Contains(
						err.Error(),
						TIMEOUT_TEXT,
					) {
						t.Fatalf(
							"err = %v, want the text and the hint",
							err,
						)
					}
				}
			},
		)
	}
}

// TestDerive_RefusesASpawnInTheTopLevel proves Derive refuses a top-level
// statement of the entry that spawns, directly, in a loop or in a branch, with
// runtime.ErrCarry and runtime.ErrInit, and the hint of Q383, since the flow
// could not run it.
//
// Revisions:
//   - 2026-11-11 22:58: initial creation (C127, B274, Q383, Q388)
func TestDerive_RefusesASpawnInTheTopLevel(t *testing.T) {
	for _, item := range []struct {
		name string
		text string
	}{
		{
			"a call",
			DERIVE_CALL,
		},
		{
			"a branch",
			DERIVE_BRANCH,
		},
		{
			"a loop",
			DERIVE_LOOP,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := runtime.Derive(&common.Source{
					Entry: "spawning.star",
					Text:  []byte(item.text),
				})
				if !errors.Is(
					err,
					runtime.ErrCarry,
				) || !errors.Is(
					err,
					runtime.ErrInit,
				) {
					t.Fatalf(
						"err = %v, want ErrCarry and ErrInit",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					HINT,
				) {
					t.Fatalf(
						"err = %v, want the hint",
						err,
					)
				}
			},
		)
	}
}

// TestCheck_RefusesASpawnInInit proves Check, and so Emit, refuses a flow whose
// init holds a spawn - as a call, or inside text - with runtime.ErrForm and
// runtime.ErrInit, and the hint of Q383, and accepts an init that does not
// spawn.
//
// Revisions:
//   - 2026-11-11 22:58: initial creation (C127, B274, Q383, Q388)
func TestCheck_RefusesASpawnInInit(t *testing.T) {
	for _, item := range []struct {
		name string
		init *workflowpb.Init
		want error
	}{
		{
			"a print call",
			&workflowpb.Init{
				Form: &workflowpb.Init_Call{
					Call: &workflowpb.Call{
						Function: "print",
						Builtin:  true,
					},
				},
			},
			nil,
		},
		{
			"a spawn call",
			&workflowpb.Init{
				Form: &workflowpb.Init_Call{
					Call: &workflowpb.Call{
						Function: "spawn",
						Builtin:  true,
					},
				},
			},
			runtime.ErrForm,
		},
		{
			"text that spawns in a branch",
			&workflowpb.Init{
				Form: &workflowpb.Init_Text{
					Text: "if True:\n    spawn(f)",
				},
			},
			runtime.ErrForm,
		},
		{
			"text that does not spawn",
			&workflowpb.Init{
				Form: &workflowpb.Init_Text{
					Text: "if True:\n    print(\"x\")",
				},
			},
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				flow, err := runtime.Derive(&common.Source{
					Entry: "spawning.star",
					Text:  []byte(CHECK_SCRIPT),
				})
				if err != nil {
					t.Fatal(err)
				}

				flow.Init = []*workflowpb.Init{item.init}

				err = runtime.Check(flow)
				if !errors.Is(
					err,
					item.want,
				) || errors.Is(
					err,
					runtime.ErrInit,
				) != (item.want != nil) {
					t.Fatalf(
						"err = %v, want %v, and ErrInit if refused",
						err,
						item.want,
					)
				}

				if item.want != nil && !strings.Contains(
					err.Error(),
					HINT,
				) {
					t.Fatalf(
						"err = %v, want the hint",
						err,
					)
				}
			},
		)
	}
}

// TestDerive_RefusesATimeoutInTheTopLevel proves Derive refuses a top-level
// statement of the entry that calls timeout, as it does spawn, with
// runtime.ErrCarry and runtime.ErrInit and a hint that names timeout, and that
// a timeout in a function main calls is carried and passes Check (Q389).
//
// Revisions:
//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
func TestDerive_RefusesATimeoutInTheTopLevel(t *testing.T) {
	for _, item := range []struct {
		name string
		text string
		want bool
	}{
		{
			"a call",
			DERIVE_TIMEOUT_CALL,
			true,
		},
		{
			"a branch",
			DERIVE_TIMEOUT_BRANCH,
			true,
		},
		{
			"a function main calls",
			DERIVE_TIMEOUT_MAIN,
			false,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				flow, err := runtime.Derive(&common.Source{
					Entry: "timing.star",
					Text:  []byte(item.text),
				})
				if !item.want {
					if err != nil {
						t.Fatal(err)
					}

					err = runtime.Check(flow)
					if err != nil {
						t.Fatal(err)
					}

					return
				}

				if !errors.Is(
					err,
					runtime.ErrCarry,
				) || !errors.Is(
					err,
					runtime.ErrInit,
				) {
					t.Fatalf(
						"err = %v, want ErrCarry and ErrInit",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					TIMEOUT_HINT,
				) {
					t.Fatalf(
						"err = %v, want the hint",
						err,
					)
				}
			},
		)
	}
}

// TestCheck_RefusesATimeoutInInit proves Check refuses a flow whose init holds
// a timeout - as a call, or inside text - with runtime.ErrForm and
// runtime.ErrInit and a hint that names timeout (Q389).
//
// Revisions:
//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
func TestCheck_RefusesATimeoutInInit(t *testing.T) {
	for _, item := range []struct {
		name string
		init *workflowpb.Init
	}{
		{
			"a timeout call",
			&workflowpb.Init{
				Form: &workflowpb.Init_Call{
					Call: &workflowpb.Call{
						Function: "timeout",
						Builtin:  true,
					},
				},
			},
		},
		{
			"text that calls timeout in a branch",
			&workflowpb.Init{
				Form: &workflowpb.Init_Text{
					Text: "if True:\n    timeout(10, f)",
				},
			},
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				flow, err := runtime.Derive(&common.Source{
					Entry: "timing.star",
					Text:  []byte(CHECK_SCRIPT),
				})
				if err != nil {
					t.Fatal(err)
				}

				flow.Init = []*workflowpb.Init{item.init}

				err = runtime.Check(flow)
				if !errors.Is(
					err,
					runtime.ErrForm,
				) || !errors.Is(
					err,
					runtime.ErrInit,
				) {
					t.Fatalf(
						"err = %v, want ErrForm and ErrInit",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					TIMEOUT_HINT,
				) {
					t.Fatalf(
						"err = %v, want the hint",
						err,
					)
				}
			},
		)
	}
}
