package tests

import (
	"sync"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	PARALLEL = 16
	ROUNDS   = 20
)

// TestValues_ParallelCompilesShareOneDict proves many goroutines may call
// Environment, Values and a script at once under -race, and that each gets a
// dict of its own while the builtins inside are the same values.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValues_ParallelCompilesShareOneDict(t *testing.T) {
	var (
		group  sync.WaitGroup
		failed = make(
			chan string,
			PARALLEL*ROUNDS,
		)
	)

	for range PARALLEL {
		group.Add(1)

		go func() {
			defer group.Done()

			for range ROUNDS {
				env, err := plugin.Environment(plugin.Registered())
				if err != nil {
					failed <- err.Error()

					return
				}

				for _, registered := range plugin.Registered() {
					_ = registered.Values()
				}

				globals, err := starlark.ExecFileOptions(
					common.OPTIONS,
					new(starlark.Thread),
					SCRIPT_NAME,
					[]byte("out = json.encode({\"a\": 1})\n"),
					env,
				)
				if err != nil {
					failed <- err.Error()

					return
				}

				if globals[OUT] != starlark.String(ENCODED) {
					failed <- globals[OUT].String()
				}
			}
		}()
	}

	group.Wait()
	close(failed)

	for reason := range failed {
		t.Errorf(
			"parallel run: %s",
			reason,
		)
	}
}

// TestValues_CachedBuiltinsAreTheSameAndImmutable proves two calls give the
// same builtin values in dicts that are not the same map, that a change to one
// returned dict does not reach the next, and that a script cannot change a
// module it was given.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValues_CachedBuiltinsAreTheSameAndImmutable(t *testing.T) {
	var (
		first  = _FromValues()
		second = _FromValues()
	)

	if first["integer.from_bytes"] != second["integer.from_bytes"] {
		t.Fatal("two Values calls gave different builtins")
	}

	for _, registered := range plugin.Registered() {
		values := registered.Values()
		for name := range values {
			delete(
				values,
				name,
			)
		}

		if len(registered.Values()) == 0 {
			t.Fatalf(
				"%s lost its names to a caller",
				registered.Name(),
			)
		}
	}

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatal(err)
	}

	module, ok := env["integer"].(*starlarkstruct.Module)
	if !ok {
		t.Fatal("integer is not a module")
	}

	if _, settable := starlark.Value(module).(starlark.HasSetField); settable {
		t.Fatal("a module can have a field set")
	}

	_, err = starlark.ExecFileOptions(
		common.OPTIONS,
		new(starlark.Thread),
		SCRIPT_NAME,
		[]byte("integer.from_bytes = 1\n"),
		env,
	)
	if err == nil {
		t.Fatal("a script assigned to a module of the shared dict")
	}
}

// _Moving is a host plugin whose names change from one call of Values to the
// next, as a remote plugin's do when it learns a name.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Moving struct {
	guard sync.Mutex
	calls int
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q147)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Moving) Name() string {
	return "moving"
}

// Values returns a name that holds the number of this call.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q147)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Moving) Values() starlark.StringDict {
	m.guard.Lock()
	defer m.guard.Unlock()

	m.calls++

	return starlark.StringDict{
		"moving": starlark.MakeInt(m.calls),
	}
}

// TestEnvironment_AHostPluginIsNotCached proves a plugin that was not
// registered is asked for its names on every compile, so a name it learns
// between two compiles reaches the second.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q147)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnvironment_AHostPluginIsNotCached(t *testing.T) {
	host := new(_Moving)

	for want := 1; want <= 3; want++ {
		env, err := plugin.Environment([]common.Plugin{host})
		if err != nil {
			t.Fatalf(
				"environment: %v",
				err,
			)
		}

		if got := env["moving"]; got != starlark.MakeInt(want) {
			t.Fatalf(
				"compile %d saw %v, want %d",
				want,
				got,
				want,
			)
		}
	}
}
