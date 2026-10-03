package artifact

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// _Graph collects the compiled units of one artifact, in an order that puts a
// dependency before whatever loads it.
type _Graph struct {
	loader  script.Loader
	plugins []plugin.Plugin
	env     starlark.StringDict
	units   map[string]*_Unit
	order   []string
	chain   []string
}

// _Add compiles src as path, then walks whatever it loads, depth first.
//
// A compiled program lists its loads without having been run, which is what
// lets the whole graph be built, and a cycle be refused, before any top-level
// statement executes. The order is built on the way out of the recursion, so a
// dependency always precedes whatever loaded it.
//
// Revisions:
//   - 2026-09-19 18:30: initial creation
//   - 2026-09-21 17:19: keeps what it compiled, so describing reads nothing
//     twice
//   - 2026-09-29 23:30: compiles through the dialect, so a lambda written as
//     spawn's argument takes the variables it reads at the spawn
//   - 2026-10-02 01:26: keeps no source, since nothing describes the artifact
//     afterwards
//   - 2026-10-03 21:00: encodes into the bytes it keeps, rather than into text it
//     then copied into bytes
func (g *_Graph) _Add(path string, src []byte) (*_Unit, error) {
	tree, code, err := dialect.Compile(path, src, g.env.Has)
	if err != nil {
		// Not wrapped with the path: a parse or resolve error from the
		// interpreter already opens with file:line:column, and repeating the
		// file makes a reader scan past it twice to reach the position.
		return nil, err
	}

	var encoded bytes.Buffer

	err = code.Write(&encoded)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", path, err)
	}

	// Not wrapped with the path: a refusal carries the position it was
	// written at, which opens with the file, and repeating it makes a reader
	// scan past the same name twice.
	err = _Checked(g.plugins, tree)
	if err != nil {
		return nil, err
	}

	unit := &_Unit{
		saved: &artifactpb.Unit{
			Name:  path,
			Code:  encoded.Bytes(),
			Loads: map[string]string{},
		},
		tree: tree,
		code: code,
	}

	g.units[path] = unit

	for index := range code.NumLoads() {
		module, _ := code.Load(index)

		name, err := g._Reach(path, module)
		if err != nil {
			return nil, err
		}

		unit.saved.Loads[module] = name
	}

	g.order = append(g.order, path)

	return unit, nil
}

// _Reach brings the module target, as spelled by from, into the graph if it is
// not there already.
//
// The loader is asked to resolve first, because until it says what target is
// called there is nothing to compare: one spelling can reach two modules and
// two spellings can reach one, and only the loader knows which. Everything
// after that is keyed on the name it returned, and the fetch happens last, so
// a module already built and a module that closes a cycle both cost nothing to
// read.
//
// The chain is then checked before the map of units, and that order is the
// rule: _Add registers a unit as soon as it compiles and before walking its own
// loads, so everything on the chain is already in that map. Asking the map
// first answers "seen it" for a unit that is still being built, which is
// exactly the case a cycle is.
//
// Revisions:
//   - 2026-09-19 18:32: initial creation
//   - 2026-09-19 18:57: checks the chain before the map, so a cycle through the
//     entry is refused rather than reported as a missing unit at link time
//   - 2026-09-19 20:18: resolves through the loader before either check, so a
//     module is identified by what it is rather than by how it was spelled
//   - 2026-09-19 20:28: fetches only after both checks, restoring one fetch per
//     module and none at all for a cycle
//   - 2026-10-03 00:10: always has a loader, a source naming none reading
//     files beside the script
func (g *_Graph) _Reach(from string, target string) (string, error) {
	name, err := g.loader.Resolve(from, target)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q from %s: %w", target, from, err)
	}

	start := slices.Index(g.chain, name)
	if start >= 0 {
		ring := append(slices.Clone(g.chain[start:]), name)

		return "", fmt.Errorf("%w: %s", ERR_CYCLE, strings.Join(ring, CHAIN_ARROW))
	}

	_, done := g.units[name]
	if done {
		return name, nil
	}

	src, err := g.loader.Load(name)
	if err != nil {
		// The spelling and the file that wrote it, because the loader's own
		// error says what it tried to reach. Wrapping with the resolved name
		// would print one path twice.
		return "", fmt.Errorf("cannot load %q from %s: %w", target, from, err)
	}

	g.chain = append(g.chain, name)

	defer func() {
		g.chain = g.chain[:len(g.chain)-1]
	}()

	_, err = g._Add(name, src)

	return name, err
}

// _Link assembles the units into one artifact, in the order that initialises
// dependencies first.
//
// Nothing runs here. A script's module-level statements execute once per run,
// not once per compile, because what they produce includes the arguments the
// run supplies - and a value bound at compile time would be one compile's
// value shared by every run of that artifact.
//
// env is the one the source was compiled against, kept rather than rebuilt at
// each run. A script resolved against one environment and initialised against
// another is a script whose names exist at compile time and not at run time -
// and a plugin holding state would hand out a different store to each.
//
// Revisions:
//   - 2026-09-19 18:36: initial creation
//   - 2026-09-22 22:24: assembles without initialising, which moved to the run
func _Link(
	entry string,
	env starlark.StringDict,
	units map[string]*_Unit,
	order []string,
) *Artifact {
	message := &artifactpb.Artifact{
		Entry: entry,
		Units: make([]*artifactpb.Unit, 0, len(order)),
	}

	for _, name := range order {
		message.Units = append(message.Units, units[name].saved)
	}

	return &Artifact{
		saved: message,
		units: units,
		env:   env,
		order: order,
	}
}

// _Initialise runs every unit's module-level statements in order on thread,
// with this run's arguments bound on it, and freezes what each produced.
//
// Once per run, on the run's own spine, so a module's top level is part of the
// run it belongs to. Dependencies come first, so the load hook only ever has
// to hand back globals that are already built and frozen - there is nothing
// left to fetch by the time anything runs. The hook is the unit's own while it
// initialises, and none afterwards, since only a top level can load. The
// arguments are bound for as long, since only a top level can declare one.
//
// Unguarded here because the run is: a module's top level runs arbitrary
// script, and the evaluation this runs inside already recovers a panic as the
// run's failure.
//
// Returns what the entry unit produced, and a wrapped error if any unit fails
// to initialise.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation, holding what _Link used to do
//   - 2026-10-02 01:26: initialises every unit on the run's thread rather than
//     on a thread of its own, inside the run's guard
//   - 2026-10-02 01:41: takes the arguments off the thread once the units are
//     initialised, so the entry point is a body
//   - 2026-10-02 16:08: takes the arguments, rather than a context carrying
//     them
func (a *Artifact) _Initialise(
	supplied map[string]*structpb.Value,
	thread *starlark.Thread,
) (starlark.StringDict, error) {
	built := map[string]starlark.StringDict{}

	_Bind(thread, supplied)

	defer func() {
		_Unbind(thread)

		thread.Load = nil
	}()

	for _, path := range a.order {
		unit := a.units[path]

		thread.Load = func(
			_ *starlark.Thread,
			spelling string,
		) (starlark.StringDict, error) {
			globals, found := built[unit.saved.GetLoads()[spelling]]
			if !found {
				return nil, fmt.Errorf("%s: %w", spelling, ERR_NO_UNIT)
			}

			return globals, nil
		}

		globals, err := unit.code.Init(thread, a.env)
		if err != nil {
			return nil, fmt.Errorf("initialise %s: %w", path, err)
		}

		globals.Freeze()

		built[path] = globals
	}

	return built[a.saved.GetEntry()], nil
}

// _Checked lets every plugin that wants to refuse a use of its own names read
// the source before anything runs.
//
// The compiler asks and does not look: which names mean what is the plugin's
// business, and a compiler that knew would be one more thing to change every
// time somebody writes a plugin.
//
// Revisions:
//   - 2026-09-24 17:40: initial creation
//   - 2026-10-03 08:27: takes the plugins, a list
func _Checked(plugins []plugin.Plugin, tree *syntax.File) error {
	bound := _Bound(tree)

	for _, installed := range plugins {
		checker, ok := installed.(plugin.Checking)
		if !ok {
			continue
		}

		if _Shadowed(installed, bound) {
			continue
		}

		err := checker.Check(tree)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Bound is every name this file binds as a global.
//
// A global shadows a predeclared one, so a file that binds a name has taken
// it: what that name means in this file is what the file said, not what a
// plugin supplied. Missing one is not a near miss - it is the whole mistake,
// because the name then looks like the plugin's and a correct script is
// refused and told about a plugin it never reached.
//
// So every way a global is bound counts, and there are four: a def, an
// assignment, a load, and a loop variable. An assignment or a loop may
// destructure, which is why the targets are walked rather than read.
//
// Into control flow, because this dialect allows it at the top level - a name
// bound inside a top-level if is still a global. Never into a def: what that
// binds is a local, and a local cannot be what a call elsewhere resolves to.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
//   - 2026-09-24 21:00: counts a load, a loop variable, a destructuring
//     target and a binding under top-level control flow - the first two
//     predicted by a reader, the rest found looking for their neighbours
func _Bound(tree *syntax.File) map[string]bool {
	bound := map[string]bool{}

	dialect.Bound(tree.Stmts, bound)

	return bound
}

// _Shadowed reports whether this file has taken any name the plugin supplies.
//
// Asked here rather than left to each plugin, because forgetting to ask is a
// mistake that reads as a correct refusal: the check matches on the spelling a
// plugin owns, the file means something else by it, and a script that runs
// perfectly well is refused and told about a plugin it never reached. That was
// found twice in one day, in two plugins, by two different people - so it is
// the compiler's question now and a plugin cannot forget it.
//
// Conservative where a plugin supplies several names and the file has taken
// one: the whole check is skipped rather than risk that refusal. A check not
// run leaves an ordinary error at run time, which is recoverable; a wrong
// refusal at compile time stops a correct script and blames the wrong thing.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
func _Shadowed(installed plugin.Plugin, bound map[string]bool) bool {
	for name := range installed.Values() {
		if bound[name] {
			return true
		}
	}

	return false
}
