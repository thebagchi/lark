package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/dialect"
)

var (
	// ErrNoMain is returned for a script defining no entry point. Such a file
	// is a module - something else loads it - and a module is not a workflow.
	ErrNoMain = errors.New("no entry point")

	// ErrConstant is returned for a module-level value a graph cannot carry.
	//
	// A giving-up rather than a classification, unlike an argument that states
	// no value. There is nowhere for a constant to ride along: dropping it
	// leaves every function that reads it failing with undefined, which is the
	// bug Graph.constants exists to fix.
	ErrConstant = errors.New("constant cannot be carried")
)

// Report is the graph a script yielded, and what could not be carried into it.
//
// The givings-up travel beside the graph rather than being logged or dropped.
// This package migrates a script, so a caller handed only a graph would
// believe the graph is the script - and for anything with a loop in it that is
// false.
type Report struct {
	Graph   *workflowpb.Graph
	Unknown []string
}

// _Reading is one derivation's own state.
//
// Separate from Report because a struct that is both the walk and its result
// is one a caller can corrupt by reading it.
type _Reading struct {
	into      Source
	defs      map[string]*syntax.DefStmt
	text      map[string]string
	owner     map[string]string
	order     []string
	constants map[string]*workflowpb.Constant
	loaded    map[string]bool
	loading   []string
	threads   []*workflowpb.Thread
	chain     []string
	unknown   []string
}

// Of reads a script and returns the graph it can see, and what it could not.
//
// Source rather than a tree, because a body is sliced out of the text it was
// written in and a tree carries no text. Parsing goes through dialect.OPTIONS,
// so this uses the dialect decision rather than owning one.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func Of(src []byte, from string, into Source) (*Report, error) {
	tree, err := dialect.OPTIONS.Parse(from, src, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", from, err)
	}

	if into == nil {
		into = new(_Dir)
	}

	reading := &_Reading{
		into:      into,
		defs:      make(map[string]*syntax.DefStmt),
		text:      make(map[string]string),
		owner:     make(map[string]string),
		constants: make(map[string]*workflowpb.Constant),
		loaded:    map[string]bool{from: true},
		loading:   []string{from},
	}

	err = reading._Read(tree, from, string(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", from, err)
	}

	if reading.defs[ENTRY] == nil {
		return nil, fmt.Errorf("%s: %w", from, ErrNoMain)
	}

	return reading._Report(), nil
}

// _Constants is every module-level name a body may read.
//
// A literal, or a call of a function this graph declares. Some constants are
// computed, and a Call naming a declared function stays inspectable and
// editable where a string of Starlark would not - arbitrary code before the
// entry point is still refused, and a named call of a declared function is not
// that.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Constants(tree *syntax.File, module string) error {
	for _, stmt := range tree.Stmts {
		assign, ok := stmt.(*syntax.AssignStmt)
		if !ok {
			continue
		}

		name, ok := assign.LHS.(*syntax.Ident)
		if !ok || assign.Op != syntax.EQ {
			return fmt.Errorf("%s: %w", module, ErrConstant)
		}

		owner, known := r.owner[name.Name]
		if known && owner != module {
			return fmt.Errorf("%s in %s and %s: %w", name.Name, owner, module, ErrCollision)
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
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Held(expr syntax.Expr) (*workflowpb.Constant, error) {
	value, ok := _Arg(expr)
	if ok {
		return &workflowpb.Constant{
			Kind: &workflowpb.Constant_Value{Value: value},
		}, nil
	}

	call, ok := expr.(*syntax.CallExpr)
	if !ok || r.defs[_Bare(call)] == nil || !_Stated(call) {
		return nil, ErrConstant
	}

	return &workflowpb.Constant{
		Kind: &workflowpb.Constant_Call{
			Call: &workflowpb.Call{Function: _Bare(call), Args: _Values(call)},
		},
	}, nil
}

// _Report is the graph this reading saw, with the entry point on the spine.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Report() *Report {
	graph := new(workflowpb.Graph)

	r._Thread(SPINE, nil, r.defs[ENTRY])

	graph.Threads = r.threads

	for _, name := range r.order {
		graph.Functions = append(graph.Functions, r._Function(r.defs[name]))
	}

	if len(r.constants) > 0 {
		graph.Constants = r.constants
	}

	return &Report{Graph: graph, Unknown: r.unknown}
}

// _Statement is the steps one statement takes.
//
// Only the statement kinds that can hold a call. A for, a while or an if is
// not a step and its contents are not read: a body that holds one does not
// model, and the function carries its own text instead.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 01:32: says whether the statement became steps whole, which is
//     what decides between a generated body and an authored one
func (r *_Reading) _Statement(lane *_Lane, stmt syntax.Stmt) ([]*workflowpb.Step, bool) {
	switch actual := stmt.(type) {
	case *syntax.ExprStmt:
		call, ok := actual.X.(*syntax.CallExpr)
		if !ok {
			steps, _ := r._Expression(lane, actual.X)

			return steps, false
		}

		return r._Expression(lane, call)

	case *syntax.ReturnStmt:
		steps, _ := r._Expression(lane, actual.Result)

		return steps, false

	case *syntax.AssignStmt:
		steps, _ := r._Expression(lane, actual.RHS)

		return steps, r._Bind(lane, actual, steps)

	case *syntax.IfStmt:
		step := r._Branch(actual)
		if step == nil {
			return nil, false
		}

		return []*workflowpb.Step{step}, true

	case *syntax.BranchStmt:
		// pass is punctuation, not code. The emitter writes one at the end of
		// every def it generates, so refusing to read one back would make a
		// generated script underivable by the thing that generated it. break
		// and continue belong to a loop, which does not model anyway.
		return nil, actual.Token == syntax.PASS
	}

	return nil, false
}

// _Expression is the steps an expression takes, arguments before the call they
// belong to.
//
// This is the phase. syntax.Walk is pre-order and meets a call before its
// arguments, so record(compute()) walks record first and derives as record
// then compute - the reverse of what runs, and worst on the shape every
// concurrent sample uses, where a join's arguments are the spawns it waits
// for. A graph whose steps are in the wrong order describes a different
// program, and nothing later can repair it.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Expression(lane *_Lane, expr syntax.Expr) ([]*workflowpb.Step, bool) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return r._Within(lane, expr)
	}

	// A builtin that names threads reads its own arguments, because what they
	// produce is the list of threads it waits for rather than steps before it.
	switch _Bare(call) {
	case SPAWN:
		return r._Spawn(lane, call)

	case JOIN, CANCEL:
		return r._Waits(lane, call, _Bare(call))
	}

	var steps []*workflowpb.Step

	for _, arg := range call.Args {
		made, _ := r._Expression(lane, arg)
		steps = append(steps, made...)
	}

	step, whole := r._Call(call)
	if step == nil {
		return steps, false
	}

	return append(steps, step), whole
}

// _Stated reports whether every argument of a call states a value.
//
// A call whose arguments do not is still a step - the ordering of what it
// calls is worth having - but it is not a faithful one: record(compute())
// renders as record() and loses what it was passed. So the function holding it
// keeps its body, and this is what says so.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Stated(call *syntax.CallExpr) bool {
	return len(_Values(call)) == len(call.Args)
}

// _Within is the steps the parts of a composite expression take, left to
// right, which is the order Starlark evaluates them in.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Within(lane *_Lane, expr syntax.Expr) ([]*workflowpb.Step, bool) {
	var steps []*workflowpb.Step

	whole := true

	for _, part := range _Parts(expr) {
		made, ok := r._Expression(lane, part)

		steps = append(steps, made...)
		whole = whole && ok
	}

	return steps, whole
}

// _Call is the step a call is and whether it carried the call whole, or nil for
// one the workflow has no opinion about.
//
// Nil rather than a giving-up: state.set, print and a method on a string are
// calls a graph carries in a body rather than as steps, and refusing them
// would refuse every script.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Call(call *syntax.CallExpr) (*workflowpb.Step, bool) {
	name := _Bare(call)

	if name == SLEEP {
		step := _Pauses(call)

		return step, step != nil
	}

	// A wrapper's last argument is a callable rather than a value, so what
	// makes it whole is that the wrapper read it - not that every argument
	// states a value, which is the test a plain call answers.
	if name == REPEAT || name == RETRY || name == TIMEOUT {
		step := r._Wrapper(name, call)

		return step, step != nil
	}

	if r.defs[name] == nil {
		return nil, false
	}

	step := &workflowpb.Step{Action: &workflowpb.Step_Call{
		Call: &workflowpb.Call{Function: name, Args: _Values(call)},
	}}

	return step, _Stated(call)
}

// _Bare is the plain name a call calls, or empty.
//
// A call through anything but a bare identifier - a method, an element of a
// list, a value another call returned - is one a graph cannot point at. That
// is also what keeps " ".join(parts) out: it is a DotExpr, so it never reaches
// the join a workflow means.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Bare(call *syntax.CallExpr) string {
	name, ok := call.Fn.(*syntax.Ident)
	if !ok {
		return ""
	}

	return name.Name
}

// _Values is a call's arguments as the values they state, or none at all.
//
// All or nothing: an argument that states no value - an expression, a name -
// means the call cannot be carried faithfully, and carrying the rest would
// invent a call the script never made.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Values(call *syntax.CallExpr) []*structpb.Value {
	var args []*structpb.Value

	for _, arg := range call.Args {
		value, ok := _Arg(arg)
		if !ok {
			return nil
		}

		args = append(args, value)
	}

	return args
}

// _Defs is every top-level function a file defines, in the order it defines
// them.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Defs(tree *syntax.File) []*syntax.DefStmt {
	var defs []*syntax.DefStmt

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			defs = append(defs, def)
		}
	}

	return defs
}

// _Pauses is the step a sleep is, in the milliseconds the schema counts.
//
// The builtin takes seconds and the schema counts milliseconds, so the
// conversion happens here rather than in whatever reads the graph. A sleep
// whose argument is not a number states nothing to convert, and is no step.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Pauses(call *syntax.CallExpr) *workflowpb.Step {
	if len(call.Args) != 1 {
		return nil
	}

	value, ok := _Arg(call.Args[0])
	if !ok {
		return nil
	}

	return &workflowpb.Step{Action: &workflowpb.Step_Sleep{
		Sleep: &workflowpb.Sleep{DurationMs: int32(value.GetNumberValue() * MILLIS)},
	}}
}

// _Function is one top-level def as the graph carries it: its name, its
// parameters and, where no thread models it, its body.
//
// Steps or a body, never both. A function some thread generated is carried by
// those steps, and sending its text as well would be one thing said twice in
// two languages.
//
// A signature this cannot carry - a default, a *args, a **kwargs - is a
// giving-up rather than a silent loss, because the generated def would drop it
// and a call relying on it would raise instead of defaulting.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Function(def *syntax.DefStmt) *workflowpb.Function {
	fn := &workflowpb.Function{Name: def.Name.Name}

	names, plain := _Params(def)
	if !plain {
		r._Gave(def.Name.Name, "signature carries a default, *args or **kwargs")
	}

	fn.Params = names

	if r._Generated(def.Name.Name) {
		return fn
	}

	fn.Body = _Body(r.text[def.Name.Name], def)

	return fn
}

// _Generated reports whether a thread carries steps for this function, which
// is the emitter's own rule for whose body it writes.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Generated(name string) bool {
	for _, thread := range r.threads {
		if _Runs(thread) != name {
			continue
		}

		if len(thread.GetStatic().GetSteps()) > 0 {
			return true
		}
	}

	return false
}
