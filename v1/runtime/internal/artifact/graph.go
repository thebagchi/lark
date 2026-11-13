package artifact

import (
	"bytes"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/internal/merge"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Graph collects the compiled units of one artifact, in an order that puts a
// dependency before whatever loads it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Graph struct {
	plugins []common.Plugin
	env     starlark.StringDict
	units   map[string]*_Unit
	order   []string
}

// _Add compiles tree as path, the one unit of the artifact.
//
// A compiled program lists its loads without having been run, but the tree is
// a merge: it loads nothing, so there is one unit and no order to build. A
// plugin's check reads each file as its author wrote it, not the merge, whose
// names a rename may have changed.
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
//   - 2026-11-02 00:55: returns only an error, since the unit is kept in the
//     graph and no caller read the copy it returned
//   - 2026-11-02 22:58: compiles the merged tree it is given, and walks no loads
//     (C44a)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func (g *_Graph) _Add(
	path string,
	merged *merge.Merged,
) error {
	code, err := dialect.Compile(
		merged.Written,
		merged.Compiled,
		g.env.Has,
	)
	if err != nil {
		// Not wrapped with the path: a parse or resolve error from the
		// interpreter already opens with file:line:column, and repeating the
		// file makes a reader scan past it twice to reach the position.
		return err
	}

	saved, err := _Encoded(
		path,
		code,
	)
	if err != nil {
		return err
	}

	// Not wrapped with the path: a refusal carries the position it was
	// written at, which opens with the file, and repeating it makes a reader
	// scan past the same name twice.
	for _, file := range merged.Files {
		err = _Checked(
			g.plugins,
			file,
		)
		if err != nil {
			return err
		}
	}

	return g._Stored(
		path,
		merged,
		code,
		saved,
	)
}

// _Stored keeps the unit and the modules it loads, and gives the unit a cache
// when it is pure.
//
// Pure is read here, after the loads, because a unit is pure only when every
// unit it loads is pure, and each of those is stored by then.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-28 23:34: gives a pure unit a cache, so its top level runs once
//     for every run of the artifact
//   - 2026-11-02 00:55: returns only an error, since the unit is kept in the
//     graph and no caller read the copy it returned
//   - 2026-11-05 17:15: keeps the tree without the shaken definitions too (C44b)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func (g *_Graph) _Stored(
	path string,
	merged *merge.Merged,
	code *starlark.Program,
	saved *artifactpb.Unit,
) error {
	unit := &_Unit{
		saved: saved,
		tree:  merged.Written,
		kept:  merged.Kept,
		code:  code,
	}

	g.units[path] = unit

	if g._Pure(unit) {
		unit.cache = _NewCache()
	}

	g.order = append(
		g.order,
		path,
	)

	return nil
}

// _Pure reports whether unit's top level calls nothing, and every unit it
// loads is pure.
//
// A load is a call of a kind: it hands over what another top level produced.
// So a unit that loads an impure unit sees what that run built, and must be
// built by that run too.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func (g *_Graph) _Pure(unit *_Unit) bool {
	if _Calls(unit.tree) {
		return false
	}

	for _, name := range unit.saved.GetLoads() {
		if g.units[name].cache == nil {
			return false
		}
	}

	return true
}

// _Encoded writes the compiled program into the unit the artifact keeps.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 16:05: cuts the path in the encode error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
func _Encoded(
	path string,
	code *starlark.Program,
) (*artifactpb.Unit, error) {
	var encoded bytes.Buffer

	err := code.Write(&encoded)
	if err != nil {
		return nil, fmt.Errorf(
			"encode %s: %w",
			unpack.ShortText(path),
			err,
		)
	}

	return &artifactpb.Unit{
		Name:  path,
		Code:  encoded.Bytes(),
		Loads: map[string]string{},
	}, nil
}

// _Link assembles the units into one artifact, in the order that initialises
// dependencies first.
//
// Nothing runs here. A script's module-level statements execute at a run,
// not at a compile, because what they produce includes the arguments the
// run supplies - and a value bound at compile time would be one compile's
// value shared by every run of that artifact. A pure unit runs once, at the
// first run that needs it, and a failure there is still that run's.
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
		Units: make(
			[]*artifactpb.Unit,
			0,
			len(order),
		),
	}

	for _, name := range order {
		message.Units = append(
			message.Units,
			units[name].saved,
		)
	}

	return &Artifact{
		saved: message,
		units: units,
		env:   env,
		order: order,
		names: new(_Declaring),
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
// A pure unit is the exception: its top level runs once, on the first run that
// needs it, and every later run gets the same frozen globals. Nothing in those
// globals belongs to a run, since a top level with no call never reads the
// thread, the arguments or the run.
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
//   - 2026-10-28 23:34: gets the globals of a pure unit from its cache, built
//     by the first run
//   - 2026-11-13 13:04: the arguments are workflow.Value (C132, Q437)
func (a *Artifact) _Initialise(
	supplied map[string]*workflowpb.Value,
	thread *starlark.Thread,
) (starlark.StringDict, error) {
	built := map[string]starlark.StringDict{}

	_Bind(
		thread,
		supplied,
	)

	defer func() {
		_Unbind(thread)

		thread.Load = nil
	}()

	for _, path := range a.order {
		globals, err := a._InitUnit(
			thread,
			path,
			a.units[path],
			built,
		)
		if err != nil {
			return nil, err
		}

		built[path] = globals
	}

	return built[a.saved.GetEntry()], nil
}

// _InitUnit is the globals of one unit for this run: those its cache keeps
// when the unit is pure, and otherwise those its top level produces now.
//
// A pure unit loads only pure units, which come before it in the order, so
// built already holds what it loads when its cache builds it on this thread.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-28 23:34: reads a pure unit from its cache, which runs the top
//     level only on the first run
func (a *Artifact) _InitUnit(
	thread *starlark.Thread,
	path string,
	unit *_Unit,
	built map[string]starlark.StringDict,
) (starlark.StringDict, error) {
	if unit.cache == nil {
		return a._Execute(
			thread,
			path,
			unit,
			built,
		)
	}

	return unit.cache._Globals(
		thread,
		func() (starlark.StringDict, error) {
			return a._Execute(
				thread,
				path,
				unit,
				built,
			)
		},
	)
}

// _Execute runs one unit's top level on thread, loading the units it names
// from built, and freezes what it produced.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation, from _InitUnit
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 16:05: cuts the path in the initialise error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func (a *Artifact) _Execute(
	thread *starlark.Thread,
	path string,
	unit *_Unit,
	built map[string]starlark.StringDict,
) (starlark.StringDict, error) {
	thread.Load = func(
		_ *starlark.Thread,
		spelling string,
	) (starlark.StringDict, error) {
		globals, found := built[unit.saved.GetLoads()[spelling]]
		if !found {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.ShortText(spelling),
				ErrUnit,
			)
		}

		return globals, nil
	}

	globals, err := unit.code.Init(
		thread,
		a.env,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialise %s: %w",
			unpack.ShortText(path),
			err,
		)
	}

	globals.Freeze()

	return globals, nil
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
//   - 2026-11-05 17:40: asks a plugin through plugin.Supplied and plugin.Examined, which
//     return a panic as an error (B123, C96 fix round 1)
//   - 2026-11-05 18:20: asks through _Hosted, in this package, with no new exported name in
//     plugin (Q183)
//   - 2026-11-05 19:07: collects the bound names with dialect.Bound, with no one-use
//     helper (C58)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func _Checked(
	plugins []common.Plugin,
	tree *syntax.File,
) error {
	bound := map[string]bool{}

	dialect.Bound(
		tree.Stmts,
		bound,
	)

	for _, installed := range plugins {
		_, ok := installed.(common.Checking)
		if !ok {
			continue
		}

		shadowed, err := _Shadowed(
			installed,
			bound,
		)
		if err != nil {
			return err
		}

		if shadowed {
			continue
		}

		var refusal error

		err = _Hosted(
			installed,
			"Check",
			func() {
				refusal = installed.(common.Checking).Check(tree)
			},
		)
		if err != nil {
			return err
		}

		if refusal != nil {
			return refusal
		}
	}

	return nil
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
//   - 2026-11-05 17:40: reads the names through plugin.Supplied and returns its error
//     (B123, C96 fix round 1)
//   - 2026-11-05 18:20: reads them through _Hosted (Q183)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func _Shadowed(
	installed common.Plugin,
	bound map[string]bool,
) (bool, error) {
	var names starlark.StringDict

	err := _Hosted(
		installed,
		"Values",
		func() {
			names = installed.Values()
		},
	)
	if err != nil {
		return false, err
	}

	for name := range names {
		if bound[name] {
			return true, nil
		}
	}

	return false, nil
}

// _Hosted runs call, which is one call into a host plugin's code made by this
// package, and returns a panic of that call as an error that names the plugin
// and the method and wraps the recovered panic (guard.Contained, Q113). Every
// call from this package into a plugin goes through it, so a host plugin that
// panics on its second Values or in its Check gives Compile an error. No
// sentinel wraps the error (Q179, Q180); guard keeps only an error panic value
// in the chain, and formats any other with %v.
//
// Revisions:
//   - 2026-11-05 18:20: initial creation, in place of plugin.Supplied and
//     plugin.Examined, which are not public (Q183)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
//   - 2026-11-10 16:05: cuts the plugin name in the error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
func _Hosted(
	given common.Plugin,
	method string,
	call func(),
) error {
	raised := guard.Contained(call)
	if raised == nil {
		return nil
	}

	var (
		label = "unnamed"
		named = guard.Contained(func() {
			label = given.Name()
		})
	)
	if named != nil {
		label = "unnamed"
	}

	return fmt.Errorf(
		"%s: plugin %s panicked: %w",
		unpack.ShortText(label),
		method,
		raised,
	)
}
