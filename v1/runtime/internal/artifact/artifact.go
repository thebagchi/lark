// Package artifact is internal, not public: it holds the compile of a script
// and its modules into one artifact.
//
// Package artifact compiles a Starlark script, together with every module it
// loads, into one thing that owes nothing to the loader or the sources it came
// from.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/merge"
	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// The errors this package raises itself. A cycle, a depth, a refused call, a missing
// entry and a missing source are spelling's and script's, returned as they are.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: one-word names for the 4 sentinels it names (C121, Q348)
//   - 2026-11-13 23:55: the doc says a refused call, formerly an arity (C136, Q475)
var (
	ErrUnit = errors.New("no such unit")

	// ErrArtifact is returned for an artifact lark did not compile: a nil
	// one, or a value of another type that has a Save method. Only a type of
	// this package carries the compiled code a run needs.
	//
	// Revisions:
	//   - 2026-11-07 23:04: initial creation (C91w6, Q118)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 17:26: renamed from ErrNotCompiled (C121, Q348)
	ErrArtifact = errors.New("the artifact was not compiled by lark")

	// ErrUTF8 is returned by Save for a script whose merged source is not
	// valid UTF-8. The source travels in a string field, and proto3 refuses
	// text that is not UTF-8. The script runs; only a bundle is refused.
	//
	// Revisions:
	//   - 2026-11-03 06:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 17:26: renamed from ErrNotUTF8 (C121, Q348)
	ErrUTF8 = errors.New("the source is not valid UTF-8, so no bundle can carry it")

	// ErrPlugin is returned by Load for a bundle that names a plugin
	// the load was not given. Save writes the name of every plugin whose names
	// the script uses, so the run would fail at the first use of one.
	//
	// Revisions:
	//   - 2026-11-05 18:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 17:26: renamed from ErrPluginMissing (C121, Q348)
	ErrPlugin = errors.New("the bundle needs a plugin that was not given")
)

// _Unit is one compiled file inside an artifact.
//
// Its name, its compiled bytes and its resolved loads are held as the generated
// artifact.Unit rather than re-declared here: that message already says what
// those three things are, and a Go struct beside it would be a second
// declaration to keep in step. Save marshals it as it stands.
//
// The tree and the compiled program sit outside it, because neither survives a
// wire format: a tree is not carried at all, and a program is carried as the
// bytes inside the message.
//
// cache is set only on a pure unit: one whose top level calls nothing and
// loads only pure units. It keeps what the first run built for every later
// run. A unit read back from a bundle has no tree to read, so it has no cache
// and every run builds it.
//
// kept is tree without what the merge shook out: the program's shape.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Unit struct {
	saved *artifactpb.Unit
	tree  *syntax.File
	kept  *syntax.File
	code  *starlark.Program
	cache *_Cache
}

// Artifact is a script and every module it loads, compiled together.
//
// Once built it owes nothing to the loader or the sources it came from: the
// code of every unit is in here, in an order that initialises dependencies
// first.
//
// Which unit is the entry, and the units themselves in that order, are the
// generated artifact.Artifact - the same reuse _Unit makes. units indexes those
// by name for lookup.
//
// env is the environment the units were compiled against, kept because every
// run initialises them again and must do so against the environment they
// resolved against. order is the order that initialises dependencies first.
//
// No globals. What initialising produces belongs to one run, since a run's
// arguments are part of it. A pure unit is the exception, and its unit keeps
// the globals, since a top level that calls nothing cannot read a run.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
type Artifact struct {
	saved  *artifactpb.Artifact
	units  map[string]*_Unit
	env    starlark.StringDict
	order  []string
	merged *merge.Merged
	text   *_Text
	names  *_Declaring
}

// _Text is the source of a merge, rendered the first time a bundle is saved
// and kept for every save after it. A run never renders it.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Text struct {
	once sync.Once
	body []byte
	err  error
}

// WithPlugins makes a compile give its script the names plugins supply, beside
// those of every plugin of the runtime's own. It takes every one at once: a
// second WithPlugins is refused, and the compile returns an error naming it.
// A call with no plugin, or only nil ones, gives nothing and is not a call.
//
// Beside rather than instead, because the runtime's plugins are what a script
// is written against, and a compile that dropped them would refuse scripts
// that run everywhere else. Two compiles in one process can still see
// different plugins of a host's.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 13:12: takes the plugins themselves, so a host builds no
//     registry
//   - 2026-10-03 00:10: sets a compile's plugins, there being no Compiler to
//     hold them
//   - 2026-10-03 08:27: adds the plugins to the runtime's own, as the facade's did,
//     so the option means one thing wherever it is named
//   - 2026-11-04 23:50: skips a nil plugin, which has no name to give (C86)
//   - 2026-11-04 23:50: skips a typed-nil plugin too, found once here by
//     reflect, so no later use calls Name on it (C86)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-08 10:35: refuses a second WithPlugins, which replaced the first (C75, Q235)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-11 00:39: finds a nil plugin with nilcheck.Absent, the one check (C122, B268)
func WithPlugins(plugins ...common.Plugin) common.CompileOption {
	var (
		given = slices.Concat(
			plugin.Registered(),
			slices.DeleteFunc(
				slices.Clone(plugins),
				nilcheck.Absent[common.Plugin],
			),
		)
		// No plugin, or only absent ones, is not a call (Q261): nothing is given.
		none = len(given) == len(plugin.Registered())
	)

	return func(settings *common.CompileSettings) {
		if none {
			return
		}

		if settings.Plugins != nil {
			settings.Refused = errors.Join(
				settings.Refused,
				_PluginsTwice(),
			)

			return
		}

		settings.Plugins = given
	}
}

// _PluginsTwice is the refusal of a second WithPlugins: a plain error naming the
// option, the hint in parentheses, and no sentinel (Q255, Q158, Q180).
//
// Revisions:
//   - 2026-11-08 11:35: initial creation (C75, Q255)
func _PluginsTwice() error {
	return errors.New(
		"WithPlugins is given twice (give all plugins in one call)",
	)
}

// Compile builds source, together with every module it loads, without running
// any of them. The modules are merged into the script first, so the artifact
// is one unit; see package merge for the rules.
//
// The whole graph is discovered from compiled code rather than by running it.
// A compiled program lists the modules it loads, so the source's loader is
// walked to exhaustion and a cycle is found before a single top-level statement
// executes. What comes back owes the loader nothing. Every run initialises the
// units itself, with its own arguments, and Run is where a script with no
// entry point is refused. A unit whose top level calls nothing, and that loads
// only units like it, is marked pure here, and only the first run builds it.
//
// A call that passes a function the script defines or loads arguments it does
// not take is refused here, once every unit is compiled, since a loaded
// function's parameters are in another unit.
//
// Returns spelling.ErrCycle carrying the ring, spelling.ErrArgs at the call that passes the
// wrong arguments, and a wrapped error if any source fails to parse or
// resolve, or if the loader could not reach a module. Never panics.
//
// Revisions:
//   - 2026-09-19 18:26: initial creation
//   - 2026-09-19 20:16: a method on Compiler, which holds the loader, so a host
//     configures reaching modules once rather than at every call
//   - 2026-09-21 09:46: reads the environment from the registry it holds
//   - 2026-09-21 17:19: describes what it built, so a bundle carries the graph
//     a user interface draws
//   - 2026-10-02 01:25: adds CALL to the environment, refuses a call passing
//     the wrong arguments, and no longer describes what it built
//   - 2026-10-03 00:10: a function of the source, which names its own modules,
//     rather than a method on a Compiler holding a loader
//   - 2026-10-28 23:34: marks a pure unit, which only the first run builds
//   - 2026-11-02 22:58: merges the script with every module it loads, then compiles the
//     merge as one unit (C44a)
//   - 2026-11-02 22:58: keeps the merge, so Save can carry its source (C44a)
//   - 2026-11-04 23:50: refuses a nil source with ErrNoSource, and skips a nil
//     option (C86)
//   - 2026-11-05 16:54: records the plugins whose names the script uses, for
//     Save to write (C79)
//   - 2026-11-05 18:55: the plugins are asked through _Required, which returns the panic of
//     a host plugin as an error (B123, C96 fix round 2)
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the entry name in the compile error to common.QUOTE_LIMIT bytes
//     (C117, B239, Q4)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource; uses _Args, formerly _Arity
//     (C121, Q348, Q354)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
//   - 2026-11-13 23:55: uses spelling.ErrArgs, formerly ErrArity (C136, Q475)
func Compile(
	source *common.Source,
	opts ...common.CompileOption,
) (*Artifact, error) {
	if source == nil {
		return nil, script.ErrSource
	}

	var (
		settings = _Settled(opts)
		// Built once, here, and used for every unit and for linking. Rebuilding it
		// per unit would let a plugin hand each unit a different value under one
		// name, and would resolve a script against one environment while
		// initialising it against another.
		env, err = _Environment(settings)
	)
	if err != nil {
		return nil, fmt.Errorf(
			"compile %s: %w",
			unpack.ShortText(source.Entry),
			err,
		)
	}

	merged, err := merge.Merge(
		source,
		env,
	)
	if err != nil {
		return nil, err
	}

	loaded, err := _Loaded(
		merged,
		source.Entry,
		settings,
		env,
	)
	if err != nil {
		return nil, err
	}

	// Not wrapped with the name: the refusal opens with the call's own file,
	// line and column.
	err = _Args(
		loaded.units,
		loaded.order,
	)
	if err != nil {
		return nil, err
	}

	built := _Link(
		source.Entry,
		env,
		loaded.units,
		loaded.order,
	)
	built.merged = _Spent(merged)

	built.saved.Plugins, err = _Required(
		merged.Written,
		settings.Plugins,
	)
	if err != nil {
		return nil, err
	}

	built.text = new(_Text)

	return built, nil
}

// _Spent is merged with its trees dropped: they are spent, and a merge that
// keeps them would hold every file as long as the artifact lives. The text of
// the merge is all Save needs.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation, from Compile, to keep it short (C96 fix round 2)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func _Spent(merged *merge.Merged) *merge.Merged {
	merged.Compiled = nil
	merged.Files = nil

	return merged
}

// Merge is source and every module it loads as one script that loads
// nothing, for the plugins opts give.
//
// The names a compile gives a script decide which names a module may not take,
// so the options are the compile's own. Compile merges the same way and
// compiles the tree of the merge, so this is the text of what a compile runs.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-02 22:58: renders the text of the merge Compile makes
//   - 2026-11-04 23:50: refuses a nil source with ErrNoSource, and skips a nil
//     option (C86)
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the entry name in the merge error to common.QUOTE_LIMIT bytes
//     (C117, B239, Q4)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource; renamed from Amalgamate and
//     Amalgamate (C121, Q348, Q350)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func Merge(
	source *common.Source,
	opts ...common.CompileOption,
) (*common.Source, error) {
	if source == nil {
		return nil, script.ErrSource
	}

	env, err := _Environment(_Settled(opts))
	if err != nil {
		return nil, fmt.Errorf(
			"merge %s: %w",
			unpack.ShortText(source.Entry),
			err,
		)
	}

	merged, err := merge.Merge(
		source,
		env,
	)
	if err != nil {
		return nil, err
	}

	return merged.Source()
}

// _Loaded compiles the merged tree of the entry and every module it loads.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 20:41: starts a shared Walk at the entry, in place of a loader
//     and a chain of its own (C43)
//   - 2026-11-02 22:58: compiles the tree of a merge, which walks no loads (C44a)
//   - 2026-11-07 23:04: takes common.CompileSettings (C91w6)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func _Loaded(
	merged *merge.Merged,
	entry string,
	settings *common.CompileSettings,
	env starlark.StringDict,
) (*_Graph, error) {
	var (
		loaded = &_Graph{
			plugins: settings.Plugins,
			env:     env,
			units:   map[string]*_Unit{},
		}
		err = loaded._Add(
			entry,
			merged,
		)
	)
	if err != nil {
		return nil, err
	}

	return loaded, nil
}

// Run calls the entry point as a run of its own, and returns what it produced,
// once everything it spawned has stopped.
//
// Every run initialises the units itself, with the arguments its options
// supply, logs what it prints where they say, and numbers its own spine 0 and
// its spawns from 1, so two runs of one artifact produce two independent
// numberings and can take different arguments. A pure unit is built by the
// first run alone: its top level calls nothing, so no run could tell.
//
// The run begins before the units initialise, and they initialise on its
// spine. So a module's top level is part of the run: a spawn there has a run
// to join, and a call there of a function the script defines is a line of the
// spine, before the entry point's own.
//
// Nothing else checks the entry point, so this is where a script that is not
// a workflow is refused - and it reads the syntax tree rather than the
// globals, because the tree carries a position and an entry point taking
// arguments should be refused at the line that defines it.
//
// What a run produced, and how a failure is reported, are the scheduler's
// rules and are applied by Eval. A host runs an artifact by starting it;
// this is the run a start performs.
//
// Returns ErrJSON for arguments holding a value JSON cannot, spelling.ErrArgs for
// an argument no arg() of the script declares, spelling.ErrMain
// when the entry unit defines no entry point a run could call, and otherwise
// whatever Eval returns.
//
// Revisions:
//   - 2026-09-19 22:41: initial creation
//   - 2026-10-02 16:08: takes the run's options, which it hands to Invoke
//   - 2026-10-03 00:20: a function rather than a method, so a host's one way to run is
//     Start, and the run itself, which Invoke performed for any global and
//     now performs for the entry point alone
//   - 2026-10-03 08:31: hands the scheduler the run's settings, rather than a
//     context carrying a logger
//   - 2026-10-03 16:30: refuses arguments its options could not supply, before
//     anything runs
//   - 2026-10-28 23:34: builds a pure unit on the first run alone
//   - 2026-10-29 01:24: applies opts and hands them to Perform, which holds
//     the body
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 17:15: uses ErrMain and ErrJSON, formerly ErrNoMain and ErrNotJSON; uses
//     Eval, formerly Evaluate (C121, Q348, Q350)
//   - 2026-11-13 23:55: uses spelling.ErrArgs, formerly ErrArity (C136, Q475)
func Run(
	ctx context.Context,
	art common.Artifact,
	opts ...common.RunOption,
) (starlark.Value, error) {
	return Perform(
		ctx,
		art,
		Apply(opts),
	)
}

// Perform is Run, with the run's options already applied into given.
//
// For a caller that reads what the options say before the run starts, so it
// applies them once rather than once for each question and once more for the
// run. given belongs to this run alone; Run tells the rest.
//
// Revisions:
//   - 2026-10-29 01:24: initial creation, from the body of Run
//   - 2026-11-01 13:00: hands the run no ceiling, the budget being gone
//   - 2026-11-01 14:00: checks the entry with _Entered, _Runnable being gone
//   - 2026-11-02 22:32: refuses a memory watch the system cannot support, and
//     hands the run the watch (C27)
//   - 2026-11-04 23:50: a nil artifact, or one with no entry unit, returns
//     ErrNoUnit; a nil given is a given of no options (C86 fix round 2)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:04: takes common.Artifact, asserts it to this package's type, and returns
//     ErrNotCompiled for a nil or foreign one; takes common.Given (C91w6, Q118)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit; uses Eval, formerly Evaluate (C121,
//     Q348, Q350)
//   - 2026-11-13 13:04: reads the arguments as a workflow.Map (C132, Q437)
func Perform(
	ctx context.Context,
	art common.Artifact,
	given *common.Given,
) (starlark.Value, error) {
	held, err := Concrete(art)
	if err != nil {
		return nil, err
	}

	if given == nil {
		given = new(common.Given)
	}

	entry := held._Entry()
	if entry == nil {
		return nil, ErrUnit
	}

	if given.Refused != nil {
		return nil, given.Refused
	}

	// A watch that was asked for and cannot read the system is refused here,
	// before anything runs, rather than doing nothing while the run proceeds.
	// An injected source is the caller's own reading: nothing to check.
	if given.Memory != nil && given.Memory.Source == nil {
		err := memwatch.Check()
		if err != nil {
			return nil, err
		}
	}

	// An artifact loaded from a bundle has no tree, and Save refused it
	// before the bundle was written.
	tree := entry.tree
	if tree != nil {
		err := held._Entered(tree)
		if err != nil {
			return nil, err
		}
	}

	var (
		settings = &common.Settings{
			Reporter: given.Into,
			Watch:    given.Memory,
		}
		body = func(thread *starlark.Thread) (starlark.Value, error) {
			return held._Main(
				given.Args.GetEntries(),
				thread,
			)
		}
	)

	settings.Logger.Store(given.Logger)

	return scheduler.Eval(
		ctx,
		common.ENTRY,
		body,
		settings,
	)
}

// Save returns this artifact as one bundle: which unit is the entry, and every
// unit in the order that initialises dependencies first, for Load to read
// back.
//
// What a container format must carry per unit is a name, the compiled code,
// and what each load spelling in that unit resolved to. The resolutions cannot
// be recomputed by whatever reads this - recomputing them is a loader's job and
// a reader has none - so they are carried.
//
// A bundle is a program a host runs elsewhere, so an artifact whose entry
// point a run could not call is refused here, where its syntax tree can say
// why, rather than wherever it is loaded, which has none. It carries no flow:
// a host that wants the picture derives one from the source.
//
// Returns spelling.ErrMain for an artifact with no entry point a run could call, and
// a wrapped error if the bundle cannot be encoded. Never panics.
//
// Revisions:
//   - 2026-09-19 21:32: initial creation, replacing Units, Code and Loads
//   - 2026-09-21 17:19: one bundle, carrying the graph, rather than one
//     message per unit
//   - 2026-10-02 01:25: carries no graph
//   - 2026-10-03 00:26: refuses an artifact a run could not start, a bundle now being
//     read back to be run
//   - 2026-10-24 16:05: does not skip the entry check when the syntax tree
//     is nil, so a loaded artifact whose entry has no main is refused
//   - 2026-11-01 14:00: checks the entry with _Entered, _Saveable being gone
//   - 2026-11-02 08:33: marshals deterministically, so one binary gives the same
//     bytes for the same artifact (plan Q6); the bytes are not canonical across
//     languages or library versions
//   - 2026-11-02 22:58: carries the merged source beside the code (plan Q38, C44a)
//   - 2026-11-04 23:50: a nil or zero Artifact returns ErrNoUnit (C86 fix round 2)
//   - 2026-11-05 16:54: carries the names of the plugins the script uses (C79)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 11:44: the entry name in a marshal error is cut to QUOTE_LIMIT
//     bytes (C117, B239, Q4)
//   - 2026-11-10 17:15: uses ErrMain and ErrUnit, formerly ErrNoMain and ErrNoUnit (C121, Q348)
func (a *Artifact) Save() ([]byte, error) {
	entry := a._Entry()
	if entry == nil {
		return nil, ErrUnit
	}

	err := a._Entered(entry.tree)
	if err != nil {
		return nil, err
	}

	bundle, err := a._Bundle()
	if err != nil {
		return nil, err
	}

	encoded, err := proto.MarshalOptions{
		Deterministic: true,
	}.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal %s: %w",
			unpack.ShortText(a.saved.GetEntry()),
			err,
		)
	}

	return encoded, nil
}

// _Entry is the unit the entry point is in, or nil for a nil or zero artifact,
// which has none.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 2)
func (a *Artifact) _Entry() *_Unit {
	if a == nil {
		return nil
	}

	return a.units[a.saved.GetEntry()]
}

// _Bundle is the message Save marshals: the units as compiled, each with the
// source it was compiled from.
//
// A compiled artifact renders the merged source once, the first time a bundle
// is saved, and shares the text between saves. A loaded artifact has no merge,
// so its units carry the source they were read with. A new unit message shares
// the code bytes and writes nothing into the artifact, which runs may be
// reading.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-02 22:58: the source is a string (plan Q47)
//   - 2026-11-02 22:58: refuses a source that is not valid UTF-8, naming the file
//     (plan Q51)
//   - 2026-11-05 16:54: carries the plugin names (C79)
//   - 2026-11-10 17:15: uses ErrUTF8, formerly ErrNotUTF8 (C121, Q348)
func (a *Artifact) _Bundle() (*artifactpb.Artifact, error) {
	if a.merged != nil {
		name, bad := a.merged.Invalid()
		if bad {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrUTF8,
			)
		}
	}

	bundle := &artifactpb.Artifact{
		Entry:   a.saved.GetEntry(),
		Plugins: a.saved.GetPlugins(),
		Units: make(
			[]*artifactpb.Unit,
			0,
			len(a.saved.GetUnits()),
		),
	}

	for _, unit := range a.saved.GetUnits() {
		source := unit.GetSource()

		if a.merged != nil && unit.GetName() == a.saved.GetEntry() {
			rendered, err := a._Source()
			if err != nil {
				return nil, err
			}

			source = string(rendered)
		}

		bundle.Units = append(
			bundle.Units,
			&artifactpb.Unit{
				Name:   unit.GetName(),
				Code:   unit.GetCode(),
				Loads:  unit.GetLoads(),
				Source: source,
			},
		)
	}

	return bundle, nil
}

// _Source is the text of the merge, rendered once.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (a *Artifact) _Source() ([]byte, error) {
	a.text.once.Do(func() {
		merged, err := a.merged.Source()
		if err != nil {
			a.text.err = err

			return
		}

		a.text.body = merged.Text
	})

	return a.text.body, a.text.err
}

// _Main initialises the units on thread with the arguments supplied, then calls
// the entry point on it.
//
// Revisions:
//   - 2026-10-02 01:25: initial creation, as _Called, from Invoke's body
//   - 2026-10-02 16:08: takes the arguments, rather than a context carrying
//     them
//   - 2026-10-03 00:20: calls the entry point, the one function a run calls, and
//     refuses one the units did not define with ErrNoMain
//   - 2026-11-05 19:10: refuses an argument no arg() declares once the units
//     have initialised, before the entry point is called (C71, Q170)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the entry name in the error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
//   - 2026-11-13 13:04: the arguments are workflow.Value (C132, Q437)
func (a *Artifact) _Main(
	supplied map[string]*workflowpb.Value,
	thread *starlark.Thread,
) (starlark.Value, error) {
	globals, err := a._Initialise(
		supplied,
		thread,
	)
	if err != nil {
		return nil, err
	}

	err = a._Unknown(supplied)
	if err != nil {
		return nil, err
	}

	entry, ok := globals[common.ENTRY].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(a.saved.GetEntry()),
			spelling.ErrMain,
		)
	}

	return starlark.Call(
		thread,
		entry,
		nil,
		nil,
	)
}

// Load reads back a bundle Save wrote, as an artifact a run can start.
//
// The units arrive compiled, so nothing is parsed or checked again, and the
// bundle owes nothing to the loader or the sources it came from. What it does
// need is the environment it was compiled against, so it takes the options a
// compile takes. A bundle names the plugins its script uses, and one that names
// a plugin the options do not give is refused here, not at the first use of one
// of its names. A plugin given and not named is fine. A bundle written before
// the names were saved names none, so it is loaded with no such check.
//
// Every module a unit loads must be one the bundle holds before it, which is
// the order Save writes them in, so a bundle that does not hold up is refused
// here rather than partway through a run.
//
// Returns ErrUnit for an entry or a load the bundle does not hold,
// ErrPlugin, with the plugin quoted and bounded, for a plugin the
// bundle needs and the options do not give, and a wrapped error for bytes
// that are not a bundle or code the interpreter cannot read. Never panics.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
//   - 2026-10-24 16:05: walks each compiled program's loads and verifies the
//     saved loads map covers them, so a cleared map is refused before any
//     module runs
//   - 2026-11-05 16:54: refuses a bundle that names a plugin not given, with
//     ErrPluginMissing (C79, B111)
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-10 16:05: cuts the entry name in the load and entry errors to common.QUOTE_LIMIT
//     bytes (C117, B239, Q4)
//   - 2026-11-10 17:15: uses ErrUnit and ErrPlugin, formerly ErrNoUnit and ErrPluginMissing (C121,
//     Q348)
func Load(
	bundle []byte,
	opts ...common.CompileOption,
) (*Artifact, error) {
	var saved = new(artifactpb.Artifact)

	err := proto.Unmarshal(
		bundle,
		saved,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"unmarshal bundle: %w",
			err,
		)
	}

	var settings = _Settled(opts)

	env, err := _Environment(settings)
	if err != nil {
		return nil, fmt.Errorf(
			"load %s: %w",
			unpack.ShortText(saved.GetEntry()),
			err,
		)
	}

	err = _Supplied(
		saved.GetPlugins(),
		settings.Plugins,
	)
	if err != nil {
		return nil, err
	}

	units, order, err := _Units(saved)
	if err != nil {
		return nil, err
	}

	_, found := units[saved.GetEntry()]
	if !found {
		return nil, fmt.Errorf(
			"entry %s: %w",
			unpack.ShortText(saved.GetEntry()),
			ErrUnit,
		)
	}

	err = _Covered(units)
	if err != nil {
		return nil, err
	}

	return &Artifact{
		saved: saved,
		units: units,
		env:   env,
		order: order,
		names: new(_Declaring),
	}, nil
}

// _Settled is the settings opts make. Plugins stay nil while no WithPlugins
// has run, which is how a second one is seen; then they are the runtime's own.
//
// Revisions:
//   - 2026-10-03 00:10: initial creation, from NewCompiler
//   - 2026-10-03 08:27: starts from the runtime's own plugins, a list
//   - 2026-11-04 23:50: skips a nil option (C86)
//   - 2026-11-07 23:04: uses common.CompileOption, common.RunOption and common.CompileSettings,
//     moved from here (C91w6)
//   - 2026-11-08 10:35: leaves Plugins nil until WithPlugins or the end, so a repeat
//     is seen (C75, Q235)
func _Settled(opts []common.CompileOption) *common.CompileSettings {
	settings := new(common.CompileSettings)

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(settings)
	}

	if settings.Plugins == nil {
		settings.Plugins = plugin.Registered()
	}

	return settings
}

// _Environment is the names a script is compiled against, and initialised
// against: whatever the plugins supply, and the dialect's hidden CALL,
// whichever plugins a host enables, because the dialect compiles a statement
// call of a script function through it.
//
// Revisions:
//   - 2026-10-03 00:10: initial creation, from Compile, so a loaded bundle is
//     given the environment a compiled one was
//   - 2026-10-03 08:27: merges the plugins, a list
//   - 2026-11-02 11:56: wraps the plugins' builtins so each polls the thread's context when
//     it returns, and leaves CALL as it is
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:04: a function of common.CompileSettings, in place of a method of _Settings
//     (C91w6, Q226)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: returns settings.Refused first (C75, Q235)
func _Environment(settings *common.CompileSettings) (starlark.StringDict, error) {
	if settings.Refused != nil {
		return nil, settings.Refused
	}

	merged, err := plugin.Environment(settings.Plugins)
	if err != nil {
		return nil, err
	}

	env := scheduler.Polled(merged)
	env[common.CALL] = scheduler.Call()

	return env, nil
}

// _Entered refuses an artifact whose entry unit has no entry point a run could
// call, read off tree, the entry unit's syntax tree. A nil tree has no entry
// point, so it is refused with spelling.ErrMain. Every error names the entry unit.
//
// Revisions:
//   - 2026-11-01 14:00: initial creation, replacing _Runnable and _Saveable,
//     which differed only in the nil tree
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the entry name in the error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func (a *Artifact) _Entered(tree *syntax.File) error {
	err := spelling.ErrMain
	if tree != nil {
		err = _RequireEntry(tree)
	}

	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			unpack.ShortText(a.saved.GetEntry()),
			err,
		)
	}

	return nil
}

// _Units is every unit of a saved bundle, decoded and filed by name, and the
// order Save wrote them in.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func _Units(saved *artifactpb.Artifact) (map[string]*_Unit, []string, error) {
	var (
		units = make(
			map[string]*_Unit,
			len(saved.GetUnits()),
		)
		order = make(
			[]string,
			0,
			len(saved.GetUnits()),
		)
	)

	for _, unit := range saved.GetUnits() {
		err := _Held(
			unit,
			units,
		)
		if err != nil {
			return nil, nil, err
		}

		decoded, err := _Decoded(unit)
		if err != nil {
			return nil, nil, err
		}

		units[unit.GetName()] = decoded
		order = append(
			order,
			unit.GetName(),
		)
	}

	return units, order, nil
}

// _Held refuses a unit that loads a module the bundle has not given before it.
// Sorted, so a unit missing two names the same one every time.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 16:05: cuts the unit name in the error to common.QUOTE_LIMIT bytes (C117, B239,
//     Q4)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func _Held(
	unit *artifactpb.Unit,
	units map[string]*_Unit,
) error {
	loads := unit.GetLoads()

	for _, spelling := range slices.Sorted(maps.Keys(loads)) {
		_, found := units[loads[spelling]]
		if !found {
			return fmt.Errorf(
				"%s loads %s, which the bundle does not hold before it: %w",
				unpack.ShortText(unit.GetName()),
				unpack.QuotePrefix(spelling),
				ErrUnit,
			)
		}
	}

	return nil
}

// _Decoded is one saved unit with its compiled code read back.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
//   - 2026-11-04 21:56: checks the counts of the encoded program before the
//     interpreter allocates from them (C94, B126)
//   - 2026-11-04 21:56: quotes and bounds the unit name in both errors (C94)
func _Decoded(unit *artifactpb.Unit) (*_Unit, error) {
	err := _Bounded(unit.GetCode())
	if err != nil {
		return nil, fmt.Errorf(
			"decode %s: %w",
			unpack.QuotePrefix(unit.GetName()),
			err,
		)
	}

	code, err := starlark.CompiledProgram(bytes.NewReader(unit.GetCode()))
	if err != nil {
		return nil, fmt.Errorf(
			"decode %s: %w",
			unpack.QuotePrefix(unit.GetName()),
			err,
		)
	}

	return &_Unit{
		saved: unit,
		code:  code,
	}, nil
}

// _Covered refuses a bundle where a compiled program lists a load that the
// saved loads map does not name. The saved map is what _Initialise looks up at
// run time, so a spelling absent from it would return ErrUnit after
// earlier modules already ran.
//
// _Held checks the other direction: every name in the saved map must be a unit
// the bundle holds before it. Together the two checks make sure the saved map
// and the compiled program agree, and that every target exists.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-02 00:31: the check of one unit moved to _Unmapped
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func _Covered(units map[string]*_Unit) error {
	for _, unit := range units {
		err := _Unmapped(unit)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Unmapped refuses unit when its compiled program lists a load that its saved
// loads map does not name. The text opens with the unit's name.
//
// Revisions:
//   - 2026-11-02 00:31: initial creation, extracted from _Covered, to take the nesting out
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 16:05: cuts the unit name in the error to common.QUOTE_LIMIT bytes (C117, B239,
//     Q4)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func _Unmapped(unit *_Unit) error {
	loads := unit.saved.GetLoads()

	for idx := range unit.code.NumLoads() {
		spelling, _ := unit.code.Load(idx)
		if _, found := loads[spelling]; !found {
			return fmt.Errorf(
				"%s loads %s, which the bundle does not map: %w",
				unpack.ShortText(unit.saved.GetName()),
				unpack.QuotePrefix(spelling),
				ErrUnit,
			)
		}
	}

	return nil
}

// Concrete is art as this package's own Artifact, or ErrArtifact when art
// is nil or a value of another type.
//
// For the runtime's calls that take a common.Artifact: Start, Eval and
// Perform. A nil *Artifact held in art passes, and is refused later with
// ErrUnit, as a nil *Artifact always was.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6, Q118)
//   - 2026-11-10 17:15: uses ErrUnit and ErrArtifact, formerly ErrNoUnit and ErrNotCompiled, and
//     names Eval, formerly Evaluate, in the text (C121, Q338, Q348)
func Concrete(art common.Artifact) (*Artifact, error) {
	held, ok := art.(*Artifact)
	if !ok {
		return nil, ErrArtifact
	}

	return held, nil
}
