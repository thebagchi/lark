package plugin_test

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
	"github.com/thebagchi/lark/runtime/plugin"
	_ "github.com/thebagchi/lark/runtime/plugin/json"
)

const (
	JSON_NAME    = "json"
	CLASH_SOURCE = "clashing"
	SCRIPT_NAME  = "encode.star"
	ENCODED      = `{"a":1}`
	OUT          = "out"
)

// _Fixed is a plugin supplying names a test chooses.
type _Fixed struct {
	name   string
	values starlark.StringDict
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-09-19 22:34: initial creation
func (f *_Fixed) Name() string {
	return f.name
}

// Values returns what this plugin supplies.
//
// Revisions:
//   - 2026-09-19 22:34: initial creation
func (f *_Fixed) Values() starlark.StringDict {
	return f.values
}

// TestEnvironment_ImportingAPluginEnablesIt proves a blank import is the whole
// of installing one: nothing in this test names a type from that package.
//
// Revisions:
//   - 2026-09-19 22:35: initial creation
func TestEnvironment_ImportingAPluginEnablesIt(t *testing.T) {
	env, err := plugin.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	if _, found := env[JSON_NAME]; !found {
		t.Fatalf("environment has %v, want %s", env.Keys(), JSON_NAME)
	}
}

// TestEnvironment_APluginsNamesReachAScript proves a registered plugin is
// reachable from Starlark, not merely present in a map.
//
// Revisions:
//   - 2026-09-19 22:36: initial creation
func TestEnvironment_APluginsNamesReachAScript(t *testing.T) {
	env, err := plugin.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	src := "out = json.encode({\"a\": 1})\n"

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT_NAME},
		SCRIPT_NAME,
		[]byte(src),
		env,
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	got, ok := globals[OUT].(starlark.String)
	if !ok {
		t.Fatalf("%s is %T, want starlark.String", OUT, globals[OUT])
	}

	if string(got) != ENCODED {
		t.Fatalf("got %s, want %s", string(got), ENCODED)
	}
}

// TestEnvironment_RefusesAConflictNamingBoth proves a clash is refused before
// anything runs, and that the refusal says who supplied what.
//
// It captures the registry and puts back exactly what was there, because
// registration is package-level state. Restoring something that merely looks
// like what was registered is what makes a test like this order-dependent, and
// an earlier version of it did precisely that.
//
// Revisions:
//   - 2026-09-19 22:37: initial creation
func TestEnvironment_RefusesAConflictNamingBoth(t *testing.T) {
	before := plugin.Registered()

	defer func() {
		plugin.Reset()

		for _, installed := range before {
			plugin.Register(installed)
		}
	}()

	plugin.Register(&_Fixed{
		name: CLASH_SOURCE,
		values: starlark.StringDict{
			JSON_NAME: starlark.None,
		},
	})

	_, err := plugin.Environment()
	if !errors.Is(err, plugin.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}

	for _, want := range []string{CLASH_SOURCE, JSON_NAME} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal does not name %s: %v", want, err)
		}
	}

	t.Logf("refused: %v", err)
}

// TestEnvironment_ReportsOneConflictTheSameWayEveryRun proves the refusal does
// not depend on map iteration order: a plugin supplying two clashing names must
// name the same one each time.
//
// Revisions:
//   - 2026-09-19 22:38: initial creation
func TestEnvironment_ReportsOneConflictTheSameWayEveryRun(t *testing.T) {
	before := plugin.Registered()

	defer func() {
		plugin.Reset()

		for _, installed := range before {
			plugin.Register(installed)
		}
	}()

	plugin.Reset()

	plugin.Register(&_Fixed{
		name: JSON_NAME,
		values: starlark.StringDict{
			"alpha": starlark.None,
			"omega": starlark.None,
		},
	})

	plugin.Register(&_Fixed{
		name: CLASH_SOURCE,
		values: starlark.StringDict{
			"alpha": starlark.None,
			"omega": starlark.None,
		},
	})

	first := ""

	for attempt := range 20 {
		_, err := plugin.Environment()
		if !errors.Is(err, plugin.ErrConflict) {
			t.Fatalf("attempt %d gave %v, want ErrConflict", attempt, err)
		}

		if attempt == 0 {
			first = err.Error()

			continue
		}

		if err.Error() != first {
			t.Fatalf("attempt %d reported %q, first reported %q", attempt, err.Error(), first)
		}
	}

	t.Logf("stable across 20 runs: %s", first)
}
