// Package artifact compiles a Starlark script, together with every module it
// loads, into one thing that owes nothing to the loader or the sources it came
// from.
package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/script"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ERR_CYCLE is spelling's, so it is the value reading a script into a flow
	// refuses the same ring with.
	ERR_CYCLE   = spelling.ERR_CYCLE
	ERR_NO_UNIT = errors.New("no such unit")
)

const CHAIN_ARROW = " -> "

// Option sets one thing on a compile.
//
// Options rather than parameters because a compile will grow knobs - a limit,
// a predeclared environment - and a parameter list cannot grow without breaking
// every caller. See CLAUDE.md, SOLID, open/closed.
type Option func(settings *_Settings)

// _Settings is what options set on a compile. plugins are where its script's
// names come from: the runtime's own, and whatever WithPlugins adds. Where its
// modules come from is the source's own, not a setting.
type _Settings struct {
	plugins []plugin.Plugin
}

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
type _Unit struct {
	saved *artifactpb.Unit
	tree  *syntax.File
	code  *starlark.Program
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
// arguments are part of it.
type Artifact struct {
	saved *artifactpb.Artifact
	units map[string]*_Unit
	env   starlark.StringDict
	order []string
}

// WithPlugins makes a compile give its script the names plugins supply, beside
// those of every plugin of the runtime's own. It takes every one at once: a
// second WithPlugins replaces the first.
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
func WithPlugins(plugins ...plugin.Plugin) Option {
	given := slices.Concat(plugin.DEFAULT, plugins)

	return func(settings *_Settings) {
		settings.plugins = given
	}
}

// Compile builds source, together with every module it loads, without running
// any of them.
//
// The whole graph is discovered from compiled code rather than by running it.
// A compiled program lists the modules it loads, so the source's loader is
// walked to exhaustion and a cycle is found before a single top-level statement
// executes. What comes back owes the loader nothing. Every run initialises the
// units itself, with its own arguments, and Run is where a script with no
// entry point is refused.
//
// A call that passes a function the script defines or loads arguments it does
// not take is refused here, once every unit is compiled, since a loaded
// function's parameters are in another unit.
//
// Returns ERR_CYCLE carrying the ring, ERR_ARITY at the call that passes the
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
func Compile(source *script.Source, opts ...Option) (*Artifact, error) {
	settings := _Settled(opts)

	// Built once, here, and used for every unit and for linking. Rebuilding it
	// per unit would let a plugin hand each unit a different value under one
	// name, and would resolve a script against one environment while
	// initialising it against another.
	env, err := settings._Environment()
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", source.Entry, err)
	}

	loaded := &_Graph{
		loader:  script.LoaderFor(source),
		plugins: settings.plugins,
		env:     env,
		units:   map[string]*_Unit{},
		chain:   []string{source.Entry},
	}

	_, err = loaded._Add(source.Entry, source.Text)
	if err != nil {
		return nil, err
	}

	// Not wrapped with the name: the refusal opens with the call's own file,
	// line and column.
	err = _Arity(loaded.units, loaded.order)
	if err != nil {
		return nil, err
	}

	return _Link(source.Entry, env, loaded.units, loaded.order), nil
}

// Run calls the entry point as a run of its own, and returns what it produced,
// once everything it spawned has stopped.
//
// Every run initialises the units itself, with the arguments its options
// supply, logs what it prints where they say, and numbers its own spine 0 and
// its spawns from 1, so two runs of one artifact produce two independent
// numberings and can take different arguments.
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
// rules and are applied by Evaluate. A host runs an artifact by starting it;
// this is the run a start performs.
//
// Returns ERR_NOT_JSON for arguments holding a value JSON cannot, ERR_NO_MAIN
// when the entry unit defines no entry point a run could call, and otherwise
// whatever Evaluate returns.
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
func Run(ctx context.Context, art *Artifact, opts ...RunOption) (starlark.Value, error) {
	given := _Applied(opts)
	if given.refused != nil {
		return nil, given.refused
	}

	err := art._Runnable()
	if err != nil {
		return nil, err
	}

	settings := &scheduler.Settings{
		Reporter: given.into,
		Logger:   given.logger,
		Ceiling:  given.ceiling,
	}

	body := func(thread *starlark.Thread) (starlark.Value, error) {
		return art._Main(given.args.GetFields(), thread)
	}

	return scheduler.Evaluate(ctx, ENTRY, body, settings)
}

// _Main initialises the units on thread with the arguments supplied, then calls
// the entry point on it.
//
// Revisions:
//   - 2026-10-02 01:25: initial creation, as _Called, from Invoke's body
//   - 2026-10-02 16:08: takes the arguments, rather than a context carrying
//     them
//   - 2026-10-03 00:20: calls the entry point, the one function a run calls, and
//     refuses one the units did not define with ERR_NO_MAIN
func (a *Artifact) _Main(
	supplied map[string]*structpb.Value,
	thread *starlark.Thread,
) (starlark.Value, error) {
	globals, err := a._Initialise(supplied, thread)
	if err != nil {
		return nil, err
	}

	entry, ok := globals[ENTRY].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%s: %w", a.saved.GetEntry(), ERR_NO_MAIN)
	}

	return starlark.Call(thread, entry, nil, nil)
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
// Returns ERR_NO_MAIN for an artifact with no entry point a run could call, and
// a wrapped error if the bundle cannot be encoded. Never panics.
//
// Revisions:
//   - 2026-09-19 21:32: initial creation, replacing Units, Code and Loads
//   - 2026-09-21 17:19: one bundle, carrying the graph, rather than one
//     message per unit
//   - 2026-10-02 01:25: carries no graph
//   - 2026-10-03 00:26: refuses an artifact a run could not start, a bundle now being
//     read back to be run
func (a *Artifact) Save() ([]byte, error) {
	err := a._Runnable()
	if err != nil {
		return nil, err
	}

	encoded, err := proto.Marshal(a.saved)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", a.saved.GetEntry(), err)
	}

	return encoded, nil
}

// Load reads back a bundle Save wrote, as an artifact a run can start.
//
// The units arrive compiled, so nothing is parsed or checked again, and the
// bundle owes nothing to the loader or the sources it came from. What it does
// need is the environment it was compiled against, so it takes the options a
// compile takes: a script that calls a plugin the bundle is not given fails at
// that name when it runs.
//
// Every module a unit loads must be one the bundle holds before it, which is
// the order Save writes them in, so a bundle that does not hold up is refused
// here rather than partway through a run.
//
// Returns ERR_NO_UNIT for an entry or a load the bundle does not hold, and a
// wrapped error for bytes that are not a bundle or code the interpreter cannot
// read. Never panics.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func Load(bundle []byte, opts ...Option) (*Artifact, error) {
	saved := new(artifactpb.Artifact)

	err := proto.Unmarshal(bundle, saved)
	if err != nil {
		return nil, fmt.Errorf("unmarshal bundle: %w", err)
	}

	env, err := _Settled(opts)._Environment()
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", saved.GetEntry(), err)
	}

	units, order, err := _Units(saved)
	if err != nil {
		return nil, err
	}

	_, found := units[saved.GetEntry()]
	if !found {
		return nil, fmt.Errorf("entry %s: %w", saved.GetEntry(), ERR_NO_UNIT)
	}

	return &Artifact{saved: saved, units: units, env: env, order: order}, nil
}

// _Settled is the settings opts make, starting from the runtime's own plugins.
//
// Revisions:
//   - 2026-10-03 00:10: initial creation, from NewCompiler
//   - 2026-10-03 08:27: starts from the runtime's own plugins, a list
func _Settled(opts []Option) *_Settings {
	settings := &_Settings{plugins: plugin.DEFAULT}

	for _, opt := range opts {
		opt(settings)
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
func (s *_Settings) _Environment() (starlark.StringDict, error) {
	env, err := plugin.Environment(s.plugins)
	if err != nil {
		return nil, err
	}

	env[spelling.CALL] = scheduler.CALL

	return env, nil
}

// _Runnable refuses an artifact whose entry point a run could not call, read
// off the entry unit's syntax tree. An artifact loaded from a bundle has none,
// and would have been refused when it was saved.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation, from Run, so Save refuses what Run would
func (a *Artifact) _Runnable() error {
	tree := a.units[a.saved.GetEntry()].tree
	if tree == nil {
		return nil
	}

	err := _RequireEntry(tree)
	if err != nil {
		return fmt.Errorf("%s: %w", a.saved.GetEntry(), err)
	}

	return nil
}

// _Units is every unit of a saved bundle, decoded and filed by name, and the
// order Save wrote them in.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func _Units(saved *artifactpb.Artifact) (map[string]*_Unit, []string, error) {
	units := make(map[string]*_Unit, len(saved.GetUnits()))
	order := make([]string, 0, len(saved.GetUnits()))

	for _, unit := range saved.GetUnits() {
		err := _Held(unit, units)
		if err != nil {
			return nil, nil, err
		}

		decoded, err := _Decoded(unit)
		if err != nil {
			return nil, nil, err
		}

		units[unit.GetName()] = decoded
		order = append(order, unit.GetName())
	}

	return units, order, nil
}

// _Held refuses a unit that loads a module the bundle has not given before it.
// Sorted, so a unit missing two names the same one every time.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func _Held(unit *artifactpb.Unit, units map[string]*_Unit) error {
	loads := unit.GetLoads()

	for _, spelling := range slices.Sorted(maps.Keys(loads)) {
		_, found := units[loads[spelling]]
		if !found {
			return fmt.Errorf(
				"%s loads %q, which the bundle does not hold before it: %w",
				unit.GetName(),
				spelling,
				ERR_NO_UNIT,
			)
		}
	}

	return nil
}

// _Decoded is one saved unit with its compiled code read back.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func _Decoded(unit *artifactpb.Unit) (*_Unit, error) {
	code, err := starlark.CompiledProgram(bytes.NewReader(unit.GetCode()))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", unit.GetName(), err)
	}

	return &_Unit{saved: unit, code: code}, nil
}
