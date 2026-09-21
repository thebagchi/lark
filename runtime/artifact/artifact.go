// Package artifact compiles a Starlark script, together with every module it
// loads, into one thing that owes nothing to the loader or the sources it came
// from.
package artifact

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

var (
	ErrCycle       = errors.New("cycle in the load graph")
	ErrNoGlobal    = errors.New("no such global")
	ErrNoLoader    = errors.New("no loader to reach a module with")
	ErrNoUnit      = errors.New("no such unit")
	ErrNotCallable = errors.New("global is not callable")
)

const CHAIN_ARROW = " -> "

// Option sets one thing on a Compiler.
//
// Options rather than parameters because a Compiler will grow knobs - a limit,
// a predeclared environment - and a parameter list cannot grow without breaking
// every caller. See CLAUDE.md, SOLID, open/closed.
type Option func(c *Compiler)

// Compiler builds artifacts. The loader it holds is how every compile it runs
// reaches a module; a Compiler without one resolves modules against the
// directory of the file that loaded them. The registry is where its scripts'
// names come from; without one it is the default, which every plugin's init
// registers into.
type Compiler struct {
	loader   Loader
	registry *plugin.Registry
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
// by name for lookup, and globals is what initialising produced, which no wire
// format carries.
type Artifact struct {
	saved   *artifactpb.Artifact
	units   map[string]*_Unit
	globals starlark.StringDict
}

// NewCompiler returns a Compiler configured by opts.
//
// Revisions:
//   - 2026-09-19 20:15: initial creation
//   - 2026-09-21 09:46: starts from the default registry
func NewCompiler(opts ...Option) *Compiler {
	compiler := &Compiler{
		loader:   &_Dir{},
		registry: plugin.DEFAULT,
	}

	for _, opt := range opts {
		opt(compiler)
	}

	return compiler
}

// WithLoader makes a Compiler reach modules through loader.
//
// Revisions:
//   - 2026-09-19 20:15: initial creation
func WithLoader(loader Loader) Option {
	return func(c *Compiler) {
		c.loader = loader
	}
}

// WithPlugins makes a Compiler give its scripts the names in registry rather
// than the default one's.
//
// Two compilers in one process can then see different plugins, and a test can
// build an environment without touching what every other test sees.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func WithPlugins(registry *plugin.Registry) Option {
	return func(c *Compiler) {
		c.registry = registry
	}
}

// Compile builds name from src, together with every module it loads, and
// initialises all of them.
//
// The answer this POC was written for: the whole graph is discovered from
// compiled code rather than by running it. A compiled program lists the modules
// it loads, so the loader is walked to exhaustion, the cycle is found and the
// entry point is checked before a single top-level statement executes. What
// comes back owes the loader nothing.
//
// Returns ErrNoMain if name defines no entry point a run could call, ErrCycle
// carrying the ring, and a wrapped error if any source fails to parse, resolve
// or initialise, or if the loader could not reach a module. Never panics.
//
// Revisions:
//   - 2026-09-19 18:26: initial creation
//   - 2026-09-19 20:16: a method on Compiler, which holds the loader, so a host
//     configures reaching modules once rather than at every call
//   - 2026-09-21 09:46: reads the environment from the registry it holds
func (c *Compiler) Compile(name string, src []byte) (*Artifact, error) {
	// Built once, here, and used for every unit and for linking. Rebuilding it
	// per unit would let a plugin hand each unit a different value under one
	// name, and would resolve a script against one environment while
	// initialising it against another.
	env, err := c.registry.Environment()
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", name, err)
	}

	graph := &_Graph{
		loader: c.loader,
		env:    env,
		units:  map[string]*_Unit{},
		chain:  []string{name},
	}

	_, err = graph._Add(name, src)
	if err != nil {
		return nil, err
	}

	return _Link(name, env, graph.units, graph.order)
}

// Invoke calls the global named fn as a run of its own and returns what it
// produced, once everything it spawned has stopped.
//
// Every invocation is a run of its own: it numbers its own spine 0 and its
// spawns from 1, so two invocations of one artifact produce two independent
// numberings. That is what makes a recorded graph comparable with a later run
// of the same function.
//
// What a run produced, and how a failure is reported, are the scheduler's
// rules and are applied by Evaluate; this only finds the function.
//
// Returns ErrNoGlobal if fn names nothing, ErrNotCallable if it names something
// that is not a function, and otherwise whatever Evaluate returns.
//
// Revisions:
//   - 2026-09-19 22:40: initial creation
//   - 2026-09-20 01:41: wraps the context's error when a failure arrives with
//     it already done, so a cancelled run is identifiable wherever the cancel
//     happened to land
//   - 2026-09-21 08:09: drops a branch nothing could reach, since both
//     sentinels it matched are returned before the call is made
//   - 2026-09-21 09:46: the run itself moved to scheduler.Evaluate, which owns
//     the rules it applies
func (a *Artifact) Invoke(ctx context.Context, fn string) (starlark.Value, error) {
	value, found := a.globals[fn]
	if !found {
		return nil, fmt.Errorf("invoke %s: %w", fn, ErrNoGlobal)
	}

	target, ok := value.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("invoke %s: %w", fn, ErrNotCallable)
	}

	return scheduler.Evaluate(ctx, fn, target)
}

// Run calls the entry point.
//
// Nothing else here checks the entry point, so this is where a script that is
// not a workflow is refused - and it reads the syntax tree rather than the
// globals, because the tree carries a position and an entry point taking
// arguments should be refused at the line that defines it.
//
// Returns ErrNoMain when the entry unit defines no entry point a run could
// call, and otherwise whatever Invoke returns.
//
// Revisions:
//   - 2026-09-19 22:41: initial creation
func (a *Artifact) Run(ctx context.Context) (starlark.Value, error) {
	err := _RequireEntry(a.units[a.saved.GetEntry()].tree)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.saved.GetEntry(), err)
	}

	return a.Invoke(ctx, ENTRY)
}

// Save returns every unit of this artifact, dependencies first, each one a
// marshalled artifact.Unit.
//
// The answer this POC was written for: what a container format must carry is a
// name, the compiled code, and what each load spelling in that unit resolved
// to. The resolutions cannot be recomputed by whatever reads this - recomputing
// them is a loader's job and a reader has none - so they are carried.
//
// The slice is the artifact's initialisation order, so a reader can replay it
// without sorting. The schema is declared in proto/artifact.proto; this returns
// one marshalled message per unit rather than a single Artifact message,
// because the entry's name is the only other thing needed and a caller already
// has it.
//
// Returns a wrapped error if a unit's code cannot be encoded. Never panics.
//
// Revisions:
//   - 2026-09-19 21:32: initial creation, replacing Units, Code and Loads
func (a *Artifact) Save() ([][]byte, error) {
	saved := make([][]byte, 0, len(a.saved.GetUnits()))

	for _, unit := range a.saved.GetUnits() {
		encoded, err := proto.Marshal(unit)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", unit.GetName(), err)
		}

		saved = append(saved, encoded)
	}

	return saved, nil
}
