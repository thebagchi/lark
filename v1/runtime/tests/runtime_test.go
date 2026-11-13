package tests

// This file imports only the facade. That is the point of it: if the facade
// stopped re-exporting something, or stopped registering the scheduler's
// builtins, nothing here would compile or pass - which is the whole of what
// phase 11 has to prove.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	EXPECTED_SUM = 10

	// HOST_FIXTURE is the script of host.star. It loads LIBRARY_FIXTURE, spawns
	// two functions and returns their sum, EXPECTED_SUM. The text opens with a
	// newline, which _Text drops.
	//
	// Revisions:
	//   - 2026-11-09 11:36: holds the script, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	HOST_FIXTURE = `
load("lib.star", "double")

def left():
    return double(2)

def right():
    return double(3)

def main():
    assert(1 + 1 == 2, "arithmetic still works")

    got = join(spawn(left), spawn(right))

    return got[0] + got[1]
`

	// BROKEN_FIXTURE is the script of broken.star, whose main fails an assert
	// with WHY.
	//
	// Revisions:
	//   - 2026-11-09 11:36: holds the script, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BROKEN_FIXTURE = `
def main():
    assert(False, "the host should see this")
    return 1
`

	// LIBRARY_FIXTURE is the module host.star loads, which defines no entry
	// point.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:36: holds the script, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LIBRARY_FIXTURE = `
def double(x):
    return x * 2
`

	// EXPECTED_TOTAL is EXPECTED_SUM as a script reports it.
	//
	// Revisions:
	//   - 2026-09-20 19:57: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	EXPECTED_TOTAL = "10"
	WHY            = "the host should see this"

	// SPEAKING_FIXTURE spawns two functions that print a line each, so its
	// graph has SPEAKERS functions, main among them, and its transcript SPOKEN
	// lines.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:36: holds the script, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPEAKING_FIXTURE = `
def left():
    print("from left")

def right():
    print("from right")

def main():
    first = spawn(left)
    second = spawn(right)
    join(first, second)
`
	SPEAKERS = 3
	SPOKEN   = 2

	// PLUGINS uses a name of the host's beside three of the runtime's own -
	// spawn and join from core, and math - and GREETED is what it returns.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PLUGINS = `
def work():
    return math.gcd(12, 18)

def main():
    worker = spawn(work)
    got = join(worker)
    return "%s %d" % (greeting, got[0])
`
	GREETED = `"hello 6"`

	// DISK reads a file, which only a script given file can name.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DISK = `
def main():
    return file.read("notes.txt")
`

	// FLOW is a flow as a user interface sends one, and DOUBLED what the
	// script it describes returns.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FLOW = `{
		"functions": [{"name": "double", "params": ["x"], "body": "return x * 2"}],
		"text": "return double(21)"
	}`
	DOUBLED = "42"

	// SAID prints one line and returns, and SAID_LINE is how WithLog writes that
	// line once its time and level are dropped.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SAID = `
def main():
    print("hello")
    return "done"
`
	SAID_LINE = "thread=thread_0 function=main msg=hello"

	// MISCALLED is a flow whose main passes double two arguments.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-13 10:16: the args are tagged values (C132, Q422)
	MISCALLED = `{
		"functions": [{"name": "double", "params": ["x"], "body": "return x * 2"}],
		"main": {"statement": [{"call": {"function": "double",
			"args": [{"int": "21"}, {"int": "1"}]}}]}
	}`
)

// MODULES maps each module name to its script, the way a host keeps scripts
// in memory. The texts open with a newline, which _Text drops.
//
// Revisions:
//   - 2026-11-09 11:36: initial creation (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	MODULES = map[string]string{
		"host.star":     HOST_FIXTURE,
		"broken.star":   BROKEN_FIXTURE,
		"lib.star":      LIBRARY_FIXTURE,
		"speaking.star": SPEAKING_FIXTURE,
	}
)

// _Text is the script of a module, less the newline it opens with.
//
// Revisions:
//   - 2026-11-09 11:36: initial creation (C114, Q281, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Text(name string) []byte {
	return []byte(strings.TrimPrefix(
		MODULES[name],
		"\n",
	))
}

// _Memory is a Loader written the way a host would write one, against the
// interface this package re-exports. It serves MODULES.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-09 11:36: serves MODULES, not files (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Memory struct{}

// Resolve reads target as a name beside from.
//
// Revisions:
//   - 2026-09-19 23:32: initial creation
//   - 2026-11-09 17:10: a method of _Memory, not _Disk (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (d *_Memory) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the script of the module name.
//
// Revisions:
//   - 2026-09-19 23:32: initial creation
//   - 2026-11-09 11:36: reads MODULES, not a file (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (d *_Memory) Load(name string) ([]byte, error) {
	_, found := MODULES[name]
	if !found {
		return nil, fmt.Errorf(
			"read %s: no such module",
			name,
		)
	}

	return _Text(name), nil
}

// _Compiled builds the named fixture through the facade or ends the test.
//
// Revisions:
//   - 2026-09-19 23:33: initial creation
//   - 2026-11-07 23:04: returns common.Artifact, an interface (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Compiled(
	t *testing.T,
	name string,
) common.Artifact {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   _Text(name),
		Loader: &_Memory{},
	})
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	return built
}

// _Flow is raw read as a flow, the way a host reads one a user interface sent.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Flow(
	t *testing.T,
	raw string,
) *workflowpb.Flow {
	t.Helper()

	var (
		flow = new(workflowpb.Flow)
		err  = protojson.Unmarshal(
			[]byte(raw),
			flow,
		)
	)
	if err != nil {
		t.Fatalf(
			"flow: %v",
			err,
		)
	}

	return flow
}

// TestHost_OneImportIsEnough proves what the facade exists for: a host that
// names no subpackage compiles a script which loads a module, asserts, spawns
// two functions and joins them, and runs it.
//
// If the init that registers the scheduler's builtins were missing, this would
// fail at compile with "undefined: spawn" rather than pass quietly. That is the
// only check standing behind the facade's reason to exist.
//
// Revisions:
//   - 2026-09-19 23:34: initial creation
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_OneImportIsEnough(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Compiled(
			t,
			"host.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != fmt.Sprint(EXPECTED_SUM) {
		t.Fatalf(
			"got %s, want %d",
			value.String(),
			EXPECTED_SUM,
		)
	}
}

// TestHost_CanMatchAFailure proves a host can act on what went wrong without
// importing the package that raised it: the sentinel this package re-exports is
// the same value, so errors.Is reaches through.
//
// Revisions:
//   - 2026-09-19 23:35: initial creation
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_CanMatchAFailure(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Compiled(
			t,
			"broken.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrAssert,
	) {
		t.Fatalf(
			"got %v, want ErrAssert",
			err,
		)
	}

	t.Logf(
		"the host matched it: %v",
		err,
	)
}

// TestHost_RunsABundleItSaved proves a host can compile once and run elsewhere
// through this package alone: the bundle Save writes is read back by Load, with
// no loader and no source, and runs to the same answer.
//
// Revisions:
//   - 2026-09-19 23:37: initial creation, as TestHost_CanSaveAnArtifact
//   - 2026-09-21 17:19: one bundle, carrying a graph
//   - 2026-10-02 01:39: a bundle carries no graph
//   - 2026-10-03 00:27: reads the bundle back and runs it, since Load now does
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_RunsABundleItSaved(t *testing.T) {
	saved, err := _Compiled(
		t,
		"host.star",
	).Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	loaded, err := runtime.Load(saved)
	if err != nil {
		t.Fatalf(
			"load: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		loaded,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"got %s, want %s",
			value,
			EXPECTED_TOTAL,
		)
	}
}

// TestHost_SeesTheGraphAndTheTranscript is what a host is told about a run,
// through this package alone: the graph Status draws, and the transcript of
// what the script printed.
//
// Revisions:
//   - 2026-09-23 23:02: initial creation, as TestHost_CanWatchARunAsItGoes
//   - 2026-10-02 13:12: the graph and the transcript, there being no watcher
//   - 2026-10-02 15:34: counts the graph's functions and calls
//   - 2026-10-02 16:21: logs what the script prints, the logger an option of
//     the run
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: gives the run WithLog, WithLogger being dropped (C121, Q340)
func TestHost_SeesTheGraphAndTheTranscript(t *testing.T) {
	var (
		out   bytes.Buffer
		built = _Compiled(
			t,
			"speaking.star",
		)
	)

	run, err := runtime.Start(
		t.Context(),
		built,
		runtime.WithLog(&out),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	var (
		graph     = run.Status()
		functions = len(graph.GetFunctions())
	)
	if functions != SPEAKERS || len(graph.GetCalls()) != SPOKEN {
		t.Fatalf(
			"want main calling two functions, got %v",
			graph,
		)
	}

	lines := strings.Count(
		out.String(),
		"\n",
	)
	if lines != SPOKEN {
		t.Fatalf(
			"the transcript has %d lines, want %d: %q",
			lines,
			SPOKEN,
			out.String(),
		)
	}
}

// _Named is a plugin of a host's own, written the way a host would write one
// against the interface this package re-exports: called plugin, it gives a
// script names.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Named struct {
	plugin string
	names  starlark.StringDict
}

// Name is what a clash calls this plugin.
//
// Revisions:
//   - 2026-10-02 15:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (n *_Named) Name() string {
	return n.plugin
}

// Values is the names this plugin gives a script.
//
// Revisions:
//   - 2026-10-02 15:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (n *_Named) Values() starlark.StringDict {
	return n.names
}

// TestHost_KeepsTheRuntimesPluginsBesideItsOwn proves a compiler given a plugin
// of the host's still gives a script every plugin of the runtime's own: the
// one import brings them, and WithPlugins adds to them rather than replacing
// them.
//
// Revisions:
//   - 2026-10-02 15:56: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_KeepsTheRuntimesPluginsBesideItsOwn(t *testing.T) {
	var (
		greetings = &_Named{
			plugin: "greetings",
			names:  starlark.StringDict{"greeting": starlark.String("hello")},
		}
		plugins = runtime.WithPlugins(greetings)
	)

	built, err := runtime.Compile(
		&common.Source{
			Entry: "plugins.star",
			Text:  []byte(PLUGINS),
		},
		plugins,
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != GREETED {
		t.Fatalf(
			"got %s, want %s",
			value,
			GREETED,
		)
	}
}

// TestHost_GivesNoFilesystemByItself proves file is not one of the runtime's own
// plugins: a script compiled with nothing handed over cannot name it, so a host
// that runs scripts it did not write gives them no disk by leaving file out.
//
// Revisions:
//   - 2026-10-02 17:12: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_GivesNoFilesystemByItself(t *testing.T) {
	_, err := runtime.Compile(&common.Source{
		Entry: "disk.star",
		Text:  []byte(DISK),
	})
	if err == nil || !strings.Contains(
		err.Error(),
		"undefined: file",
	) {
		t.Fatalf(
			"got %v, want file undefined",
			err,
		)
	}
}

// TestHost_DerivesAFlowItCanEmitBack proves a host can read a script into a
// flow and back through this package alone: the module it loads is read
// through the host's own Loader and inlined, and the script the flow emits
// runs to the same answer.
//
// Revisions:
//   - 2026-10-02 23:51: initial creation
//   - 2026-10-03 00:17: takes the flow Derive returns, nothing being reported beside it
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_DerivesAFlowItCanEmitBack(t *testing.T) {
	flow, err := runtime.Derive(&common.Source{
		Entry:  "host.star",
		Text:   _Text("host.star"),
		Loader: &_Memory{},
	})
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	emitted, err := runtime.Emit(flow)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	built, err := runtime.Compile(&common.Source{
		Entry: "emitted.star",
		Text:  emitted,
	})
	if err != nil {
		t.Fatalf(
			"compile:\n%s\n%v",
			emitted,
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"got %s, want %s",
			value,
			EXPECTED_TOTAL,
		)
	}
}

// TestHost_MatchesOneNoMainInADeriveAndARun proves a module, which defines no
// entry point, is refused with the one ErrMain whether it is read into a
// flow or run.
//
// Revisions:
//   - 2026-10-02 23:51: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func TestHost_MatchesOneNoMainInADeriveAndARun(t *testing.T) {
	_, err := runtime.Derive(&common.Source{
		Entry: "lib.star",
		Text:  _Text("lib.star"),
	})
	if !errors.Is(
		err,
		runtime.ErrMain,
	) {
		t.Fatalf(
			"deriving a module got %v, want ErrMain",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		_Compiled(
			t,
			"lib.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrMain,
	) {
		t.Fatalf(
			"running a module got %v, want ErrMain",
			err,
		)
	}
}

// TestHost_TurnsAFlowIntoAScript proves a host can take a flow from a user
// interface to a run through this package alone: check it, write its script,
// compile that and run it.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_TurnsAFlowIntoAScript(t *testing.T) {
	var (
		flow = _Flow(
			t,
			FLOW,
		)
		err = runtime.Check(flow)
	)
	if err != nil {
		t.Fatalf(
			"check: %v",
			err,
		)
	}

	src, err := runtime.Emit(flow)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	built, err := runtime.Compile(&common.Source{
		Entry: "flow.star",
		Text:  src,
	})
	if err != nil {
		t.Fatalf(
			"compile:\n%s\n%v",
			src,
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != DOUBLED {
		t.Fatalf(
			"got %s, want %s",
			value,
			DOUBLED,
		)
	}
}

// TestHost_MatchesOneArityInAFlowAndAScript proves a call passing the wrong
// number of arguments is refused with the one ErrArity, whether a check finds
// it in a flow or a compile finds it in a script.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_MatchesOneArityInAFlowAndAScript(t *testing.T) {
	err := runtime.Check(_Flow(
		t,
		MISCALLED,
	))
	if !errors.Is(
		err,
		runtime.ErrArity,
	) {
		t.Fatalf(
			"checking a flow got %v, want ErrArity",
			err,
		)
	}

	script := "def double(x):\n    return x * 2\n\ndef main():\n    double(21, 1)\n"

	_, err = runtime.Compile(&common.Source{
		Entry: "miscalled.star",
		Text:  []byte(script),
	})
	if !errors.Is(
		err,
		runtime.ErrArity,
	) {
		t.Fatalf(
			"compiling a script got %v, want ErrArity",
			err,
		)
	}
}

// TestHost_RunsAScriptInOneCall proves the shortest path a host has: one call
// compiles a script, the modules it loads included, runs it and hands back
// what main returned.
//
// Revisions:
//   - 2026-10-03 16:32: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_RunsAScriptInOneCall(t *testing.T) {
	value, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry:  "host.star",
			Text:   _Text("host.star"),
			Loader: &_Memory{},
		},
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"got %s, want %s",
			value,
			EXPECTED_TOTAL,
		)
	}
}

// TestHost_EmitRefusesWhatCheckRefuses proves Emit checks a flow before writing
// it, so a host writing a flow a user interface sent need not check it first.
//
// Revisions:
//   - 2026-10-03 16:32: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_EmitRefusesWhatCheckRefuses(t *testing.T) {
	_, err := runtime.Emit(_Flow(
		t,
		MISCALLED,
	))
	if !errors.Is(
		err,
		runtime.ErrArity,
	) {
		t.Fatalf(
			"got %v, want ErrArity",
			err,
		)
	}
}

// TestHost_RefusesArgumentsJSONCannotHold proves an argument that is not a value
// fails the run with ErrJSON, before the script runs, since the option
// that took it had nowhere to say so.
//
// Revisions:
//   - 2026-10-03 16:32: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrJSON, formerly ErrNotJSON (C121, Q348)
func TestHost_RefusesArgumentsJSONCannotHold(t *testing.T) {
	var args = map[string]any{
		"callback": func() {
			// Empty
		},
	}

	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "said.star",
			Text:  []byte(SAID),
		},
		runtime.WithArgs(args),
	)
	if !errors.Is(
		err,
		runtime.ErrJSON,
	) {
		t.Fatalf(
			"got %v, want ErrJSON",
			err,
		)
	}
}

// TestHost_LogsToAWriterWithTheMessageLast proves WithLog writes each printed
// line to a plain writer, saying where it came from before what was said.
//
// Revisions:
//   - 2026-10-03 16:32: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_LogsToAWriterWithTheMessageLast(t *testing.T) {
	var out bytes.Buffer

	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "said.star",
			Text:  []byte(SAID),
		},
		runtime.WithLog(&out),
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	line := strings.TrimSpace(out.String())
	if !strings.HasSuffix(
		line,
		SAID_LINE,
	) {
		t.Fatalf(
			"logged %q, want it to end %q",
			line,
			SAID_LINE,
		)
	}
}

// TestHost_CannotTakeARuntimeName proves a plugin of the host's cannot quietly
// replace one of the runtime's names: a compile refuses the clash, naming both
// plugins.
//
// Revisions:
//   - 2026-10-02 15:56: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHost_CannotTakeARuntimeName(t *testing.T) {
	var (
		mine = &_Named{
			plugin: "mine",
			names:  starlark.StringDict{"sleep": starlark.None},
		}
		plugins = runtime.WithPlugins(mine)
	)

	_, err := runtime.Compile(
		&common.Source{
			Entry: "plugins.star",
			Text:  []byte(PLUGINS),
		},
		plugins,
	)
	if !errors.Is(
		err,
		runtime.ErrConflict,
	) {
		t.Fatalf(
			"got %v, want ErrConflict",
			err,
		)
	}

	t.Logf(
		"refused: %v",
		err,
	)
}
