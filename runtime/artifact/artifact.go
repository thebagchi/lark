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
	"github.com/thebagchi/lark/runtime/guard"
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
// directory of the file that loaded them.
type Compiler struct {
	loader Loader
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
func NewCompiler(opts ...Option) *Compiler {
	compiler := &Compiler{
		loader: &_Dir{},
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
func (c *Compiler) Compile(name string, src []byte) (*Artifact, error) {
	// Built once, here, and used for every unit and for linking. Rebuilding it
	// per unit would let a plugin hand each unit a different value under one
	// name, and would resolve a script against one environment while
	// initialising it against another.
	env, err := plugin.Environment()
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

// Invoke calls the global named fn on an interpreter thread of its own and
// returns what it produced, once everything it spawned has stopped.
//
// Every invocation is a run of its own: it numbers its own spine 0 and its
// spawns from 1, so two invocations of one artifact produce two independent
// numberings. That is what makes a recorded graph comparable with a later run
// of the same function.
//
// It does not return until every thread it started has stopped. A handle
// nobody joined is cancelled rather than waited for, so a forgotten spawn
// cannot hold a call open.
//
// Returns ErrNoGlobal if fn names nothing, ErrNotCallable if it names something
// that is not a function, and wraps whatever the script raised otherwise -
// including a failure re-raised from a join, and the interpreter's own message
// when ctx ended the call. A failure arriving with ctx already done also wraps
// ctx's own error, so a caller who stopped the run can say that is why.
//
// Revisions:
//   - 2026-09-19 22:40: initial creation
//   - 2026-09-20 01:41: wraps the context's error when a failure arrives with
//     it already done, so a cancelled run is identifiable wherever the cancel
//     happened to land
func (a *Artifact) Invoke(ctx context.Context, fn string) (starlark.Value, error) {
	value, found := a.globals[fn]
	if !found {
		return nil, fmt.Errorf("invoke %s: %w", fn, ErrNoGlobal)
	}

	target, ok := value.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("invoke %s: %w", fn, ErrNotCallable)
	}

	thread := &starlark.Thread{Name: fn}

	finish := scheduler.Begin(ctx, thread, fn)

	var (
		result starlark.Value
		err    error
	)

	guard.WithRecover(
		&result,
		&err,
		func() (starlark.Value, error) {
			return starlark.Call(thread, target, nil, nil)
		},
	)

	// Ended before the cause is read, not deferred. Ending a run waits for
	// every thread it started, and a thread still running is a thread that can
	// still assert. Reading the cause first let a run that had been stopped
	// report success, because the spine happened to return before the
	// assertion landed.
	finish()

	// The run's outcome is the answer when there is one, whatever this call
	// returned. A script that asserted in a thread nobody joined still failed;
	// a spine that returned a value while the run was being torn down did not
	// succeed.
	outcome := scheduler.Outcome(thread)
	if outcome != nil {
		return nil, outcome
	}

	if err != nil {
		// A caller who stopped this run should be able to say so, and could
		// not. The interpreter raises its own cancellation as text - it is
		// handed a reason string, not an error - so a cancel landing on the
		// spine produced a failure nothing could match against, while the same
		// cancel landing on a spawned thread produced one that could. Measured
		// at 7 runs in 40 taking the unmatchable path.
		if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			return nil, fmt.Errorf("%w: %w", err, ctx.Err())
		}

		// Wrapped with the function only when something above would otherwise
		// not say which one ran. A Starlark error carries its own backtrace,
		// and a re-raised join failure already names the thread that failed,
		// so "invoke main:" in front of either is a clause that adds nothing.
		if errors.Is(err, ErrNoGlobal) || errors.Is(err, ErrNotCallable) {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}

		return nil, err
	}

	return result, nil
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
