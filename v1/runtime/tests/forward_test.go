package tests

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// TestForwards_AreTheSourceValues proves a name that runtime forwards is the
// value of the package that declares it, with no internal alias between them:
// the nine internal aliases of graph and artifact were deleted (C98, Q252).
//
// Revisions:
//   - 2026-11-08 13:25: initial creation (C98, Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:00: ErrSelfJoin (C116, B237, Q327)
//   - 2026-11-10 17:15: uses ErrMain and ErrSource and ErrSelf, formerly ErrNoMain and ErrNoSource
//     and ErrSelfJoin (C121, Q348)
func TestForwards_AreTheSourceValues(t *testing.T) {
	cases := []struct {
		name string
		got  error
		want error
	}{
		{
			"ErrArity",
			runtime.ErrArity,
			spelling.ErrArity,
		},
		{
			"ErrCycle",
			runtime.ErrCycle,
			spelling.ErrCycle,
		},
		{
			"ErrMain",
			runtime.ErrMain,
			spelling.ErrMain,
		},
		{
			"ErrSource",
			runtime.ErrSource,
			script.ErrSource,
		},
		{
			"ErrSelf",
			runtime.ErrSelf,
			core.ErrSelf,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if !errors.Is(
					tc.got,
					tc.want,
				) {
					t.Fatalf(
						"runtime.%s = %v, want %v",
						tc.name,
						tc.got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestLoad_MissingPluginMatchesErrPlugin proves a host can match the
// refusal of Load for a plugin it did not give, through runtime.ErrPlugin
// (Q275), and that the text names the plugin.
//
// Revisions:
//   - 2026-11-08 22:36: initial creation (C78, Q275)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: renamed from TestLoad_MissingPluginMatchesErrPluginMissing, with
//     ErrPluginMissing now ErrPlugin (C121, Q348)
func TestLoad_MissingPluginMatchesErrPlugin(t *testing.T) {
	built, err := runtime.Compile(
		&common.Source{
			Entry: "note.star",
			Text:  []byte("def main():\n    return file.read(\"x\")\n"),
		},
		runtime.WithPlugins(file.New()),
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	t.Run(
		"without the plugin",
		func(t *testing.T) {
			_, err := runtime.Load(saved)
			if !errors.Is(
				err,
				runtime.ErrPlugin,
			) {
				t.Fatalf(
					"got %v, want runtime.ErrPlugin",
					err,
				)
			}

			if !strings.Contains(
				err.Error(),
				`"file"`,
			) {
				t.Fatalf(
					"the text %q does not name the plugin",
					err,
				)
			}
		},
	)

	t.Run(
		"with the plugin",
		func(t *testing.T) {
			_, err := runtime.Load(
				saved,
				runtime.WithPlugins(file.New()),
			)
			if err != nil {
				t.Fatalf(
					"got %v, want nil",
					err,
				)
			}
		},
	)
}

const (
	// SELF_JOIN is a script in which the thread f joins its own handle H. Main
	// spawns f, because a top level may not, and hands H to f through the
	// holder plugin of the test, as the only other way for f to see H is a
	// global.
	//
	// Revisions:
	//   - 2026-11-10 16:00: initial creation (C116, B237, Q327)
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
)

// _Holder is a plugin that hands a value from one thread to another: keep
// stores it, and take waits for it.
//
// Revisions:
//   - 2026-11-11 22:54: initial creation (C127, B274, Q383)
type _Holder struct {
	ready chan struct{}
	once  sync.Once
	held  starlark.Value
}

// Name is holder.
//
// Revisions:
//   - 2026-11-11 22:54: initial creation (C127, B274, Q383)
func (h *_Holder) Name() string {
	return "holder"
}

// Values are keep and take. keep closes ready after it stores the value, so a
// take that sees ready closed reads a value written before it.
//
// Revisions:
//   - 2026-11-11 22:54: initial creation (C127, B274, Q383)
func (h *_Holder) Values() starlark.StringDict {
	return starlark.StringDict{
		"keep": starlark.NewBuiltin(
			"keep",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				args starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				h.once.Do(func() {
					h.held = args[0]

					close(h.ready)
				})

				return starlark.None, nil
			},
		),
		"take": starlark.NewBuiltin(
			"take",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				select {
				case <-h.ready:
					return h.held, nil

				case <-time.After(5 * time.Second):
					return nil, errors.New("take: nothing was kept")
				}
			},
		),
	}
}

// TestRun_SelfJoinMatchesErrSelf proves a host can match the refusal of a
// join of the calling thread's own handle through runtime.ErrSelf, however
// many errors wrapped it on its way out of the run (Q327).
//
// Revisions:
//   - 2026-11-10 16:00: initial creation (C116, B237, Q327)
//   - 2026-11-10 17:15: renamed from TestRun_SelfJoinMatchesErrSelfJoin, with ErrSelfJoin now
//     ErrSelf (C121, Q348)
//   - 2026-11-11 22:54: gives the script the holder plugin, as its spawn moved into main (C127,
//     B274, Q383)
func TestRun_SelfJoinMatchesErrSelf(t *testing.T) {
	built, err := runtime.Compile(
		&common.Source{
			Entry: "selfjoin.star",
			Text:  []byte(SELF_JOIN),
		},
		runtime.WithPlugins(&_Holder{
			ready: make(chan struct{}),
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		t.Context(),
		5*time.Second,
	)

	defer cancel()

	run, err := runtime.Start(
		ctx,
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrSelf,
	) {
		t.Fatalf(
			"err = %v, want runtime.ErrSelf",
			err,
		)
	}
}
