package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// FORK_TOP is a script whose top level calls forks, a builtin of the
	// host that calls scheduler.Fork.
	//
	// Revisions:
	//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
	//   - 2026-11-12 18:58: renamed from BESIDE_TOP (C129, Q396)
	FORK_TOP = `
def f():
    return 1

H = forks(f)

def main():
    return H
`

	// FORK_CALLED is a script whose top level calls a function that calls
	// forks: that function runs while the top level does.
	//
	// Revisions:
	//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
	//   - 2026-11-12 18:58: renamed from BESIDE_CALLED (C129, Q396)
	FORK_CALLED = `
def f():
    return 1

def start():
    return forks(f)

H = start()

def main():
    return H
`

	// FORK_MAIN is a script whose main calls forks, which is fine.
	//
	// Revisions:
	//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
	//   - 2026-11-12 18:58: renamed from BESIDE_MAIN (C129, Q396)
	FORK_MAIN = `
def f():
    return 1

def main():
    return forks(f)
`

	// FORK_TEXT is the text of the refusal of forks while a module loads.
	// The name is the builtin that called scheduler.Fork.
	//
	// Revisions:
	//   - 2026-11-12 17:20: initial creation (C128, B278, Q389)
	//   - 2026-11-12 18:58: renamed from BESIDE_TEXT (C129, Q396)
	FORK_TEXT = "forks: cannot start a thread while the module loads " +
		"(call forks from main or a function main calls)"
)

// _Forks is a plugin of the host with one builtin, forks, which starts the
// function it is given with scheduler.Fork and waits for the result, as a
// host builtin that starts script code does.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
//   - 2026-11-12 18:58: renamed from _Besides (C129, Q396)
type _Forks struct{}

// Name is forks.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278)
//   - 2026-11-12 18:58: renamed from _Besides.Name (C129, Q396)
func (b *_Forks) Name() string {
	return "forks"
}

// Values is the builtin forks.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
//   - 2026-11-12 18:58: renamed from _Besides.Values (C129, Q396)
func (b *_Forks) Values() starlark.StringDict {
	return starlark.StringDict{
		"forks": starlark.NewBuiltin(
			"forks",
			func(
				thread *starlark.Thread,
				_ *starlark.Builtin,
				args starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				target, ok := args[0].(starlark.Callable)
				if !ok {
					return nil, errors.New("forks wants a function")
				}

				args[0].Freeze()

				handle, err := scheduler.Fork(
					thread,
					target,
				)
				if err != nil {
					return nil, err
				}

				return scheduler.Wait(
					thread,
					handle,
				)
			},
		),
	}
}

// TestRun_ABuiltinOfTheHostThatCallsForkIsRefusedWhileTheModuleLoads proves
// scheduler.Fork is refused with runtime.ErrInit when a builtin of the host
// calls it while a module's top level runs - on the top level, or in a function
// it calls - and works when main calls the builtin. spawn and timeout are not
// the only builtins that start script code on a thread of their own (Q389).
//
// The check: go test -race -count=1 -run TestRun_ABuiltinOfTheHostThatCallsFork
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
//   - 2026-11-12 18:58: renamed from
//     TestRun_ABuiltinOfTheHostThatCallsBesideIsRefusedWhileTheModuleLoads (C129, Q396)
func TestRun_ABuiltinOfTheHostThatCallsForkIsRefusedWhileTheModuleLoads(t *testing.T) {
	for _, item := range []struct {
		name string
		text string
		want error
	}{
		{
			"the top level",
			FORK_TOP,
			runtime.ErrInit,
		},
		{
			"a function the top level calls",
			FORK_CALLED,
			runtime.ErrInit,
		},
		{
			"main",
			FORK_MAIN,
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				built, err := runtime.Compile(
					&common.Source{
						Entry: "forks.star",
						Text:  []byte(item.text),
					},
					runtime.WithPlugins(&_Forks{}),
				)
				if err != nil {
					t.Fatal(err)
				}

				_, err = _Waited(
					t,
					built,
				)

				if !errors.Is(
					err,
					item.want,
				) || (item.want == nil && err != nil) {
					t.Fatalf(
						"err = %v, want %v",
						err,
						item.want,
					)
				}

				if item.want != nil && !strings.Contains(
					err.Error(),
					FORK_TEXT,
				) {
					t.Fatalf(
						"err = %v, want the text %q",
						err,
						FORK_TEXT,
					)
				}
			},
		)
	}
}
