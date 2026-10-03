package plugin_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
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
// of installing one: nothing in this test names a type from that package, and
// what it installed into is DEFAULT.
//
// Revisions:
//   - 2026-09-19 22:35: initial creation
func TestEnvironment_ImportingAPluginEnablesIt(t *testing.T) {
	env, err := plugin.Environment(plugin.DEFAULT)
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
	env, err := plugin.Environment(plugin.DEFAULT)
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
// A list of its own, copied from DEFAULT, so nothing here touches what every
// other test sees.
//
// Revisions:
//   - 2026-09-19 22:37: initial creation
//   - 2026-09-21 09:46: builds its own registry rather than restoring the
//     package's
//   - 2026-10-03 08:29: builds its own list, there being no registry
func TestEnvironment_RefusesAConflictNamingBoth(t *testing.T) {
	clashing := &_Fixed{
		name: CLASH_SOURCE,
		values: starlark.StringDict{
			JSON_NAME: starlark.None,
		},
	}

	_, err := plugin.Environment(slices.Concat(plugin.DEFAULT, []plugin.Plugin{clashing}))
	if !errors.Is(err, plugin.ERR_CONFLICT) {
		t.Fatalf("got %v, want ERR_CONFLICT", err)
	}

	for _, want := range []string{CLASH_SOURCE, JSON_NAME} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal does not name %s: %v", want, err)
		}
	}
}

// TestEnvironment_ReportsOneConflictTheSameWayEveryRun proves the refusal does
// not depend on map iteration order: a plugin supplying two clashing names must
// name the same one each time.
//
// Revisions:
//   - 2026-09-19 22:38: initial creation
//   - 2026-09-21 09:46: builds its own registry
//   - 2026-10-03 08:29: builds its own list, there being no registry
func TestEnvironment_ReportsOneConflictTheSameWayEveryRun(t *testing.T) {
	plugins := []plugin.Plugin{
		&_Fixed{
			name: JSON_NAME,
			values: starlark.StringDict{
				"alpha": starlark.None,
				"omega": starlark.None,
			},
		},
		&_Fixed{
			name: CLASH_SOURCE,
			values: starlark.StringDict{
				"alpha": starlark.None,
				"omega": starlark.None,
			},
		},
	}

	first := ""

	for attempt := range 20 {
		_, err := plugin.Environment(plugins)
		if !errors.Is(err, plugin.ERR_CONFLICT) {
			t.Fatalf("attempt %d gave %v, want ERR_CONFLICT", attempt, err)
		}

		if attempt == 0 {
			first = err.Error()

			continue
		}

		if err.Error() != first {
			t.Fatalf("attempt %d reported %q, first reported %q",
				attempt, err.Error(), first)
		}
	}
}

// TestEnvironment_OfNoPluginsSuppliesNothing proves an environment is the
// plugins it is given and nothing else, so two compiles in one process can be
// given different names.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation, as
//     TestRegistry_TwoRegistriesSeeDifferentPlugins
//   - 2026-10-03 08:29: merges no plugins, there being no registry to make empty
func TestEnvironment_OfNoPluginsSuppliesNothing(t *testing.T) {
	env, err := plugin.Environment(nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(env) != 0 {
		t.Fatalf("want no plugins to supply nothing, got %v", env.Keys())
	}

	if len(plugin.DEFAULT) == 0 {
		t.Fatal("want DEFAULT to hold the plugins this test imports")
	}
}
