// Package graph derives the flow a script describes, checks one a host
// authored, and generates the script a flow describes.
package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/script"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ERR_NO_MAIN is returned for a script defining no entry point. Such a file
	// is a module - something else loads it - and a module is not a workflow.
	// spelling's, so it is the value a run refuses one with.
	ERR_NO_MAIN = spelling.ERR_NO_MAIN

	// ERR_CYCLE is returned for modules that load each other: one reached while
	// it is still being read. spelling's, so it is the value a compile refuses
	// the same ring with.
	ERR_CYCLE = spelling.ERR_CYCLE

	// ERR_CONSTANT is returned for a module-level value a flow cannot carry,
	// and by Check for constants that name each other in a cycle.
	//
	// A giving-up rather than a classification, unlike an argument that states
	// no value. There is nowhere for a constant to ride along: dropping it
	// leaves every function that reads it failing with undefined, which is the
	// bug Flow.constants exists to fix.
	ERR_CONSTANT = errors.New("constant cannot be carried")

	// ERR_SIGNATURE is returned for a def whose signature a flow cannot carry:
	// a default, a *args or a **kwargs. Function.params is a list of names,
	// and emitting the bare names would generate a program the script was
	// not - a call relying on the default would fail with an undefined name.
	ERR_SIGNATURE = errors.New("signature cannot be carried")
)

const (
	// The vocabulary a script and a flow share, from the one package that
	// holds it. SUBJECT is the local a match evaluates its expression into,
	// the one name the walk reads as a match; the flow does not store it.
	ENTRY   = spelling.ENTRY
	SPAWN   = spelling.SPAWN
	JOIN    = spelling.JOIN
	CANCEL  = spelling.CANCEL
	SLEEP   = spelling.SLEEP
	REPEAT  = spelling.REPEAT
	RETRY   = spelling.RETRY
	TIMEOUT = spelling.TIMEOUT
	SUBJECT = spelling.SUBJECT

	// RANGE is the builtin a loop counts with.
	RANGE = "range"

	// DELAY is the keyword a repeat or a retry may name its pause with.
	DELAY = "delay"

	// TRUE and FALSE are how Starlark spells a boolean, and NONE its absence.
	TRUE  = "True"
	FALSE = "False"
	NONE  = "None"
)

// _Reading is one derivation's own state.
//
// Separate from the Flow it writes because a struct that is both the walk and
// its result is one a caller can corrupt by reading it.
type _Reading struct {
	modules   script.Loader
	defs      map[string]*syntax.DefStmt
	text      map[string]*_Lines
	owner     map[string]string
	order     []string
	constants map[string]*workflowpb.Constant
	args      map[string]*workflowpb.Arg
	loaded    map[string]bool
	loading   []string
}

// Of reads a script and returns the flow it describes.
//
// A function the walk cannot model is not a giving-up: it travels as body text,
// which the flow carries. So a flow is the whole script, and what cannot be
// carried at all is refused rather than left out.
//
// Each function, and main, is either statements or body text: one line the
// walk cannot model makes the whole of it text. Parsing goes through
// dialect.OPTIONS, so this uses the dialect decision rather than owning one.
//
// Returns ERR_NO_MAIN for a module or a nil source, ERR_CONSTANT, ERR_NOT_CARRIED
// and ERR_SIGNATURE for what the flow cannot carry, ERR_COLLISION and ERR_ALIAS
// for loads it cannot inline, and ERR_CYCLE for modules that load each other.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 08:09: names its source for what it is
//   - 2026-09-22 22:41: reads the arguments a script declares
//   - 2026-10-01 23:57: takes a Source and writes a Flow, a function's
//     statements or its body text rather than a list of threads
//   - 2026-10-03 00:10: returns the flow alone, refusing a cycle of loads
//     rather than reporting it beside the flow, and takes script's Source
func Of(source *script.Source) (*workflowpb.Flow, error) {
	if source == nil {
		return nil, ERR_NO_MAIN
	}

	tree, err := dialect.OPTIONS.Parse(source.Entry, source.Text, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.Entry, err)
	}

	reading := &_Reading{
		modules:   script.LoaderFor(source),
		defs:      make(map[string]*syntax.DefStmt),
		text:      make(map[string]*_Lines),
		owner:     make(map[string]string),
		constants: make(map[string]*workflowpb.Constant),
		args:      make(map[string]*workflowpb.Arg),
		loaded:    map[string]bool{source.Entry: true},
		loading:   []string{source.Entry},
	}

	err = reading._Read(tree, source.Entry, string(source.Text))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source.Entry, err)
	}

	if reading.defs[ENTRY] == nil {
		return nil, fmt.Errorf("%s: %w", source.Entry, ERR_NO_MAIN)
	}

	return reading._Flow(), nil
}

// _Constants is every module-level name a body may read: the arguments a run
// supplies, and the values that are fixed.
//
// A constant is a literal, or a call of a function this flow declares. Some
// constants are computed, and a Call naming a declared function stays
// inspectable and editable where a string of Starlark would not - arbitrary
// code before the entry point is still refused, and a named call of a declared
// function is not that.
//
// The two share one namespace, and one owner map enforces it: both bind a
// module-level name, so a name in both is the same collision as a name in two
// modules.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-22 22:41: sorts an argument declaration into args, leaving the
//     rest to be carried as constants
func (r *_Reading) _Constants(tree *syntax.File, module string) error {
	for _, stmt := range tree.Stmts {
		assign, ok := stmt.(*syntax.AssignStmt)
		if !ok {
			continue
		}

		name, ok := assign.LHS.(*syntax.Ident)
		if !ok || assign.Op != syntax.EQ {
			return fmt.Errorf("%s: %w", module, ERR_CONSTANT)
		}

		owner, known := r.owner[name.Name]
		if known && owner != module {
			return fmt.Errorf("%s in %s and %s: %w",
				name.Name, owner, module, ERR_COLLISION)
		}

		declared, err := r._Declared(assign.RHS)
		if err != nil {
			return fmt.Errorf("%s: %w", name.Name, err)
		}

		if declared != nil {
			r.args[name.Name] = declared
			r.owner[name.Name] = module

			continue
		}

		held, err := r._Held(assign.RHS)
		if err != nil {
			return fmt.Errorf("%s: %w", name.Name, err)
		}

		r.constants[name.Name] = held
		r.owner[name.Name] = module
	}

	return nil
}

// _Held is the constant an expression states.
//
// A call may pass literals, and names of the functions and constants declared
// before it: a constant is computed at the top of a file, where an argument or
// a later constant is not bound yet.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-01 23:57: a call may pass the name of a constant declared
//     before it, or of a function
func (r *_Reading) _Held(expr syntax.Expr) (*workflowpb.Constant, error) {
	value, ok := _Arg(expr)
	if ok {
		return &workflowpb.Constant{
			Kind: &workflowpb.Constant_Value{Value: value},
		}, nil
	}

	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, ERR_CONSTANT
	}

	held, ok := r._Call(call, r._Top())
	if !ok {
		return nil, ERR_CONSTANT
	}

	return &workflowpb.Constant{Kind: &workflowpb.Constant_Call{Call: held}}, nil
}

// _Prior reports whether a constant's call may pass name: a constant declared
// before it, or a function.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
func (r *_Reading) _Prior(name string) bool {
	_, constant := r.constants[name]

	return constant || r._Listed(name)
}

// _Flow is the flow this reading saw.
//
// main is not one of the functions: it is the spine, written from Flow.main or
// Flow.text, and the list does not begin with a call of it.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 23:53: the spine names the entry point in its first step, as
//     every thread names what it runs
//   - 2026-09-22 22:41: carries the arguments the script declared
//   - 2026-10-01 23:57: writes a Flow: the functions, then main as
//     statements or text
//   - 2026-10-03 00:10: returns the flow alone, nothing being left out of it
func (r *_Reading) _Flow() *workflowpb.Flow {
	flow := new(workflowpb.Flow)

	for _, name := range r.order {
		if name == ENTRY {
			continue
		}

		flow.Functions = append(flow.Functions, r._Function(r.defs[name]))
	}

	if len(r.constants) > 0 {
		flow.Constants = r.constants
	}

	if len(r.args) > 0 {
		flow.Args = r.args
	}

	r._Spine(flow)

	return flow
}

// _Function is one top-level def as the flow carries it: its name, its
// parameters, and either its statements or its body text.
//
// Statements when every line models, text when one does not. Never both: a
// function carried as statements and as text has said one thing twice in two
// languages.
//
// Every def reaching here has a plain signature: _Declare refused the others.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: no longer records a signature it cannot carry, since
//     one never gets this far
//   - 2026-10-01 23:57: carries the def's own statements, or its text
func (r *_Reading) _Function(def *syntax.DefStmt) *workflowpb.Function {
	params, _ := _Params(def)

	fn := &workflowpb.Function{Name: def.Name.Name, Params: params}

	stmts, ok := r._Modelled(def, params)
	if ok {
		fn.Code = &workflowpb.Function_Statements{
			Statements: &workflowpb.Statements{Statement: stmts},
		}

		return fn
	}

	fn.Code = &workflowpb.Function_Body{Body: _Body(r.text[def.Name.Name], def)}

	return fn
}

// _Spine is main as the flow carries it: statements when every line models,
// the inside of the def as text when one does not.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
func (r *_Reading) _Spine(flow *workflowpb.Flow) {
	def := r.defs[ENTRY]

	stmts, ok := r._Modelled(def, nil)
	if ok {
		flow.Spine = &workflowpb.Flow_Main{
			Main: &workflowpb.Statements{Statement: stmts},
		}

		return
	}

	flow.Spine = &workflowpb.Flow_Text{Text: _Body(r.text[ENTRY], def)}
}

// _Listed reports whether name is a function the flow lists, which is what a
// call must name to be a statement.
//
// main is declared and not listed: it is the spine, and nothing calls it.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
func (r *_Reading) _Listed(name string) bool {
	return name != ENTRY && r.defs[name] != nil
}

// _Builtin reports whether name means the builtin of that name: the script
// declares no function, constant or argument over it.
//
// Revisions:
//   - 2026-10-01 23:57: initial creation
func (r *_Reading) _Builtin(name string) bool {
	_, constant := r.constants[name]
	_, arg := r.args[name]

	return r.defs[name] == nil && !constant && !arg
}

// _Callee is the plain name a call calls, or empty.
//
// A call through anything but a bare identifier - a method, an element of a
// list, a value another call returned - is one a flow cannot point at. That is
// also what keeps " ".join(parts) out: it is a DotExpr, so it never reaches the
// join a workflow means.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation, as _Bare
//   - 2026-10-01 23:57: named for what it returns, since the node builders
//     took _Bare
func _Callee(call *syntax.CallExpr) string {
	name, ok := call.Fn.(*syntax.Ident)
	if !ok {
		return ""
	}

	return name.Name
}
