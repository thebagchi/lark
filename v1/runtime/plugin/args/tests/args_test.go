package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/args"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// SUPPLIED is what a caller passes, as a caller writes it.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SUPPLIED = `{
		"host": "db.internal",
		"port": 5432,
		"ratio": 0.25,
		"tls": true,
		"spare": null,
		"tags": ["a", "b"],
		"labels": {"tier": "gold"},
		"keyword": "supplied"
	}`

	// RUNS is how many runs of one artifact the concurrent test makes.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RUNS = 8
)

const (
	// DECLARE is the text of the module declare.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DECLARE = `
# Every shape an argument can take, and the two names disagreeing on purpose:
# what a caller supplies is "host", what the script calls it is "db".

db = arg("host", "db.internal")
port = arg("port", 5432)
ratio = arg("ratio", 0.25)
tls = arg("tls", True)
spare = arg("spare", None)
tags = arg("tags", ["a", "b"])
labels = arg("labels", {"tier": "gold"})
keyword = arg("keyword", default = "set either way round")

def main():
    return [db, port, ratio, tls, spare, tags, labels, keyword]
`

	// INATTEMPT is the text of the module inattempt.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	INATTEMPT = `
# A declaration inside an attempt. The attempt runs on the module's own
# thread, but it is a body all the same, as it was when it had a thread of its
# own.

def declare():
    return arg("host", "localhost")

host = repeat(1, declare)

def main():
    return host
`

	// INBODY is the text of the module inbody.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	INBODY = `
# A declaration where one cannot be. The thread running a body is not the
# thread a run initialises a module on.

def main():
    return arg("host", "localhost")
`

	// MODULE is the text of the module module.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MODULE = `
# A loaded module declaring an argument of its own, so that a run binds every
# unit it initialises and not only the entry.

load("shared.star", "label")

host = arg("host", "localhost")

def main():
    return label(host)
`

	// REQUIRED is the text of the module required.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	REQUIRED = `
# An argument a run cannot do without: declared with no default.

token = arg("token")

def main():
    return token
`

	// SHARED is the text of the module shared.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SHARED = `
# The module the entry point loads.

prefix = arg("prefix", "svc")

def label(host):
    return "%s://%s" % (prefix, host)
`

	// VALIDATES is the text of the module validates.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	VALIDATES = `
# A script checking what its run supplied, at module level.
#
# The check belongs here rather than inside main: it is about the run's
# arguments, which are bound before anything else executes, so a run given a
# port nobody could listen on stops before it does any work.

port = arg("port", 8080)

assert(port > 0, "port must be positive")

def main():
    return port
`
)

var (
	// MODULES maps each module name to its text. The in-memory loader reads it
	// where the tests once read testdata.
	//
	// Revisions:
	//   - 2026-11-09 11:16: initial creation (C114, Q281)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MODULES = map[string]string{
		"declare.star":   _Script(DECLARE),
		"inattempt.star": _Script(INATTEMPT),
		"inbody.star":    _Script(INBODY),
		"module.star":    _Script(MODULE),
		"required.star":  _Script(REQUIRED),
		"shared.star":    _Script(SHARED),
		"validates.star": _Script(VALIDATES),
	}
)

// _Memory is a Loader over MODULES.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation, in place of _Disk (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Memory struct{}

// Resolve reads target as a module beside from.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Memory) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text of the module at name.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Memory) Load(name string) ([]byte, error) {
	text, ok := MODULES[name]
	if !ok {
		return nil, fmt.Errorf(
			"module %s: %w",
			name,
			fs.ErrNotExist,
		)
	}

	return []byte(text), nil
}

// TestArg_SuppliedWins checks that every shape a run can supply reaches the
// script as the Starlark value it should be, and not as the default.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_SuppliedWins(t *testing.T) {
	var (
		got = _Ran(
			t,
			"declare.star",
			SUPPLIED,
		)
		want = `["db.internal",` +
			` 5432, 0.25, True, None, ["a", "b"], {"tier": "gold"}, ` +
			`"supplied"]`
	)
	if got.String() != want {
		t.Fatalf(
			"got %s, want %s",
			got.String(),
			want,
		)
	}
}

// TestArg_DefaultWhenNothingSupplied checks that a run supplying nothing gives
// every declaration the value its script stated, including the keyword form.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_DefaultWhenNothingSupplied(t *testing.T) {
	var (
		got = _Ran(
			t,
			"declare.star",
			"",
		)
		want = `["db.internal", 5432, 0.25, True, None, ["a", "b"], ` +
			`{"tier": "gold"}, "set either way round"]`
	)
	if got.String() != want {
		t.Fatalf(
			"got %s, want %s",
			got.String(),
			want,
		)
	}
}

// TestArg_NumbersArriveWhole checks that a JSON number with no fraction
// reaches a script as an int.
//
// JSON has one number type and Starlark has two, so the conversion has to
// choose. A script indexing a list or counting a repeat with a float gets an
// error from Starlark rather than the count its caller passed.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_NumbersArriveWhole(t *testing.T) {
	var (
		got = _Ran(
			t,
			"declare.star",
			`{"port": 7}`,
		)
		held, ok = got.(*starlark.List)
	)
	if !ok {
		t.Fatalf(
			"returned a %T, want a list",
			got,
		)
	}

	if held.Index(1).String() != "7" {
		t.Fatalf(
			"port arrived as %s, want 7",
			held.Index(1).String(),
		)
	}
}

// TestArg_RequiredAndMissingFailsTheRun checks that an argument declared with
// no default, and not supplied, stops the run at the declaration.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRequired, formerly ErrNotSupplied (C121, Q349)
func TestArg_RequiredAndMissingFailsTheRun(t *testing.T) {
	_, err := _Run(
		t,
		"required.star",
		"{}",
	)
	if !errors.Is(
		err,
		args.ErrRequired,
	) {
		t.Fatalf(
			"got %v, want ErrRequired",
			err,
		)
	}

	got, err := _Run(
		t,
		"required.star",
		`{"token": "t-123"}`,
	)
	if err != nil {
		t.Fatalf(
			"supplied: %v",
			err,
		)
	}

	if got.String() != `"t-123"` {
		t.Fatalf(
			"got %s, want \"t-123\"",
			got.String(),
		)
	}
}

// TestArg_InABodyIsRefused checks the rule that closes the trap this feature
// would otherwise open.
//
// A body runs on a thread the scheduler made, not the thread a run
// initialises a module on, so no value could reach such a call. It would take
// its default in silence, and deriving a graph would not carry it either.
//
// It supplies no arguments: a key no module-level arg() declares is refused
// before main runs (C71), and this call is in a body, so it declares none.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-05 19:15: supplies no arguments, since the key would be refused
//     first (C71)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrDeclare, formerly ErrNotDeclaring (C121, Q349)
func TestArg_InABodyIsRefused(t *testing.T) {
	_, err := _Run(
		t,
		"inbody.star",
		"",
	)
	if !errors.Is(
		err,
		args.ErrDeclare,
	) {
		t.Fatalf(
			"got %v, want ErrDeclare",
			err,
		)
	}
}

// TestRegression_QWithin checks that an attempt of a repeat or a retry
// refuses a declaration, as any body does.
//
// An attempt now runs on its caller's thread. At module level, that thread is
// the one a run binds its arguments on, so without the check the call reads
// them.
//
// Revisions:
//   - 2026-10-29 06:26: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrDeclare, formerly ErrNotDeclaring (C121, Q349)
func TestRegression_QWithin(t *testing.T) {
	_, err := _Run(
		t,
		"inattempt.star",
		`{"host": "db.internal"}`,
	)
	if !errors.Is(
		err,
		args.ErrDeclare,
	) {
		t.Fatalf(
			"got %v, want ErrDeclare",
			err,
		)
	}
}

// TestArg_ALoadedModuleDeclaresItsOwn checks that every unit a run
// initialises is bound, and not only the entry.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_ALoadedModuleDeclaresItsOwn(t *testing.T) {
	got := _Ran(
		t,
		"module.star",
		`{"prefix": "redis", "host": "cache.internal"}`,
	)

	if got.String() != `"redis://cache.internal"` {
		t.Fatalf(
			"got %s, want \"redis://cache.internal\"",
			got.String(),
		)
	}
}

// TestArg_TwoRunsOfOneArtifactDiffer is why initialising moved out of the
// compile. One artifact, compiled once, run twice with different arguments.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_TwoRunsOfOneArtifactDiffer(t *testing.T) {
	var (
		art = _Compiled(
			t,
			"module.star",
		)
		first = _Result(
			t,
			art,
			`{"host": "db.internal"}`,
		)
		second = _Result(
			t,
			art,
			`{"prefix": "redis", "host": "cache.internal"}`,
		)
	)

	if first.String() != `"svc://db.internal"` {
		t.Fatalf(
			"first run got %s, want \"svc://db.internal\"",
			first.String(),
		)
	}

	if second.String() != `"redis://cache.internal"` {
		t.Fatalf(
			"second run got %s, want \"redis://cache.internal\"",
			second.String(),
		)
	}
}

// TestArg_ConcurrentRunsKeepTheirOwn checks that runs of one artifact at once
// keep their own arguments and their own globals.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_ConcurrentRunsKeepTheirOwn(t *testing.T) {
	art := _Compiled(
		t,
		"module.star",
	)

	var group sync.WaitGroup

	for run := range RUNS {
		group.Add(1)

		go func() {
			defer group.Done()

			var (
				host = fmt.Sprintf(
					"host-%d",
					run,
				)
				got = _Result(
					t,
					art,
					fmt.Sprintf(
						`{"host": %q}`,
						host,
					),
				)
				want = fmt.Sprintf(
					`"svc://%s"`,
					host,
				)
			)
			if got.String() != want {
				t.Errorf(
					"run %d got %s, want %s",
					run,
					got.String(),
					want,
				)
			}
		}()
	}

	group.Wait()
}

// _Ran is what a script returned, failing the test if it did not run.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ran(
	t *testing.T,
	name string,
	supplied string,
) starlark.Value {
	t.Helper()

	got, err := _Run(
		t,
		name,
		supplied,
	)
	if err != nil {
		t.Fatalf(
			"run %s: %v",
			name,
			err,
		)
	}

	return got
}

// _Result is what one run of an artifact returned, failing the test if the
// run did not finish.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Result(
	t *testing.T,
	art common.Artifact,
	supplied string,
) starlark.Value {
	t.Helper()

	got, err := _Invoked(
		t,
		art,
		supplied,
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	return got
}

// _Run compiles a fixture and runs it with supplied bound.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Run(
	t *testing.T,
	name string,
	supplied string,
) (starlark.Value, error) {
	t.Helper()

	return _Invoked(
		t,
		_Compiled(
			t,
			name,
		),
		supplied,
	)
}

// _Invoked runs an artifact with supplied bound.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-10-02 13:12: supplies the JSON in one call
//   - 2026-10-02 16:08: hands the run the arguments, as an option of it
//   - 2026-10-02 16:18: reads the JSON into the Struct a run is handed
//   - 2026-10-03 16:31: reads the JSON into the map a run is handed
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-13 12:57: reads each number as the text it was written in, as the command does
//     (C132, Q437)
func _Invoked(
	t *testing.T,
	art common.Artifact,
	supplied string,
) (starlark.Value, error) {
	t.Helper()

	if supplied == "" {
		run, err := runtime.Start(
			t.Context(),
			art,
		)
		if err != nil {
			t.Fatal(err)
		}

		return run.Wait()
	}

	var (
		held    map[string]any
		reading = json.NewDecoder(strings.NewReader(supplied))
	)

	reading.UseNumber()

	err := reading.Decode(&held)
	if err != nil {
		t.Fatalf(
			"arguments: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		art,
		runtime.WithArgs(held),
	)
	if err != nil {
		t.Fatal(err)
	}

	return run.Wait()
}

// _Compiled is a fixture compiled, with nothing run.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
//   - 2026-11-07 23:04: returns common.Artifact, an interface (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads MODULES, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Compiled(
	t *testing.T,
	name string,
) common.Artifact {
	t.Helper()

	src, ok := MODULES[name]
	if !ok {
		t.Fatalf(
			"no fixture %s",
			name,
		)
	}

	art, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   []byte(src),
		Loader: &_Memory{},
	})
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	return art
}

// TestArg_AModuleLevelCheckValidatesWhatWasSupplied is what per-run
// initialising is for, rather than something it costs.
//
// A module-level assert is a script checking its own arguments, and that check
// can only happen once per run because the values do. One artifact takes one
// set of arguments and refuses another, and the refusal is the script's own
// sentence rather than anything this plugin invented.
//
// Revisions:
//   - 2026-09-22 23:34: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestArg_AModuleLevelCheckValidatesWhatWasSupplied(t *testing.T) {
	var (
		art = _Compiled(
			t,
			"validates.star",
		)
		got = _Result(
			t,
			art,
			`{"port": 5432}`,
		)
	)
	if got.String() != "5432" {
		t.Fatalf(
			"got %s, want 5432",
			got.String(),
		)
	}

	_, err := _Invoked(
		t,
		art,
		`{"port": -1}`,
	)
	if !errors.Is(
		err,
		runtime.ErrAssert,
	) {
		t.Fatalf(
			"got %v, want ErrAssert",
			err,
		)
	}

	// And the same artifact still runs, so the refusal belonged to that run
	// and not to the artifact.
	got = _Result(
		t,
		art,
		`{"port": 80}`,
	)
	if got.String() != "80" {
		t.Fatalf(
			"got %s after the refused run, want 80",
			got.String(),
		)
	}
}

// _Alien is a value kind the converter does not know. It embeds a known kind to
// satisfy the unexported kind interface of workflow.Value, and the type switch of
// unpack.FromValue does not match it. A run cannot supply one, because
// runtime.WithArgs builds only known kinds, so the test binds it on a thread.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 23:11: moved to the external package (C65b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-13 12:57: the kind is a workflow.Value kind (C132, Q437)
type _Alien struct {
	*workflowpb.Value_Bool
}

// TestArg_RefusedValueMatchesBothSentinels checks that a refused value matches
// the args sentinel and the shared one, so a caller can ask either question.
// The test calls the registered arg builtin on a thread that holds a value of
// an unknown kind.
//
// Revisions:
//   - 2026-11-02 11:34: initial creation
//   - 2026-11-04 21:49: calls _Starlark, _Refused being gone (C59)
//   - 2026-11-08 23:11: calls the arg builtin, and moved to the external package (C65b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert and ErrValue, formerly ErrNotProtoValue and ErrNotValue
//     (C121, Q349)
//   - 2026-11-13 12:57: binds a workflow.Value of an unknown kind (C132, Q437)
func TestArg_RefusedValueMatchesBothSentinels(t *testing.T) {
	var declare *starlark.Builtin

	for _, registered := range plugin.Registered() {
		for _, value := range registered.Values() {
			builtin, ok := value.(*starlark.Builtin)
			if ok && builtin.Name() == "arg" {
				declare = builtin
			}
		}
	}

	if declare == nil {
		t.Fatal("no arg builtin is registered")
	}

	thread := new(starlark.Thread)
	thread.SetLocal(
		artifact.ARGS_LOCAL,
		map[string]*workflowpb.Value{
			"x": {
				Kind: _Alien{},
			},
		},
	)

	_, err := starlark.Call(
		thread,
		declare,
		starlark.Tuple{
			starlark.String("x"),
		},
		nil,
	)

	for name, want := range map[string]error{
		"args sentinel":   args.ErrValue,
		"shared sentinel": unpack.ErrConvert,
	} {
		t.Run(
			name,
			func(t *testing.T) {
				if !errors.Is(
					err,
					want,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						want,
					)
				}
			},
		)
	}
}

// _Script returns text without the newline that opens its raw string.
//
// Revisions:
//   - 2026-11-09 11:36: initial creation (C114, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Script(text string) string {
	return strings.TrimPrefix(
		text,
		"\n",
	)
}
