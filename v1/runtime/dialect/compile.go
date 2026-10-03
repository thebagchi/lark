package dialect

import (
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// SPAWN is the builtin whose lambda this dialect compiles differently.
const SPAWN = spelling.SPAWN

// _At is where a lambda begins, as a line and a column.
//
// Not a syntax.Position, which carries its file as a pointer: two parses of one
// source hold two pointers, so the same lambda would be two keys. Both trees
// are of one file, which is why the file can be left out.
type _At [2]int32

// Compile parses, resolves and compiles src as this dialect reads it, and
// returns the tree as it was written, resolved, beside the program.
//
// Two rules reach past OPTIONS. The first: a lambda written as spawn's
// argument takes, at the spawn, every variable it reads. spawn(lambda: f(x)) is compiled as
// spawn(lambda x=x: f(x)), and a default is evaluated where the lambda is
// written - so a thread spawned in a loop gets the x of its own iteration.
// Without this, every thread read the one x the loop went on writing, which
// was a data race: measured 2026-09-29, a thread per word printed WORLD WORLD
// in 20 of 20 runs, and go test -race reported it. A global is not a variable
// the lambda reads this way, and is left alone.
//
// Only a lambda written inside the call is rewritten, and only when spawn is
// the builtin. A script that binds spawn itself means its own function, and a
// lambda anywhere else means what a lambda always means.
//
// The second: the compiled tree carries what a run needs to report the lines
// a script's statements are, because the interpreter has no hook on a call. A
// statement call of a function the script defines at its top level or loads -
// once(), or got = once() - is compiled as a call of CALL with once first,
// which reports it, and passes BUILTIN as well when it is the whole of an if's
// branch or a match's arm. A spawn bound to a name passes that name as
// BINDING, and a spawn, repeat, retry or timeout handed a lambda that calls
// such a function passes the function as CALLEE. No script can write one of
// those keywords, and the builtin reading one takes it out before reading its
// own arguments. Which calls those are is read off the source here rather
// than asked when the call runs: asking whether a callee is one of its
// module's functions copies the module's globals on every call. The
// environment must predeclare CALL, as the artifact compiler does.
//
// Two parses, because resolving a tree mutates it and go.starlark.net will not
// resolve one twice: the first is resolved to learn what each lambda reads, the
// second is rewritten and compiled. The first is what is returned, because
// whatever reads the tree afterwards should read what was written.
//
// Returns the interpreter's own parse or resolve error, unwrapped: it opens with
// the file, line and column already.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-10-02 01:17: compiles a statement call of a script function through
//     CALL, and passes the hidden keywords a branch, a spawn and a wrapper read
func Compile(
	path string,
	src []byte,
	predeclared func(string) bool,
) (*syntax.File, *starlark.Program, error) {
	written, err := OPTIONS.Parse(path, src, 0)
	if err != nil {
		return nil, nil, err
	}

	err = resolve.File(written, predeclared, starlark.Universe.Has)
	if err != nil {
		return nil, nil, err
	}

	// The same source again. It parsed once, so it parses again; the error is
	// checked all the same, because nothing here should be believed unread.
	compiled, err := OPTIONS.Parse(path, src, 0)
	if err != nil {
		return nil, nil, err
	}

	_Take(compiled, _Taken(written))
	_Apply(compiled, _Marks(written))

	program, err := starlark.FileProgram(compiled, predeclared)
	if err != nil {
		return nil, nil, err
	}

	return written, program, nil
}

// _Taken is every lambda written as spawn's argument, with the variables each
// one reads, in the order the resolver met them.
//
// A lambda taking *args or **kwargs is left out: nothing may follow either in
// a parameter list, and spawn refuses a function taking them anyway.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Taken(written *syntax.File) map[_At][]string {
	taken := make(map[_At][]string)

	syntax.Walk(written, func(node syntax.Node) bool {
		lambda := _Spawned(node)
		if lambda == nil {
			return true
		}

		fn, ok := lambda.Function.(*resolve.Function)
		if !ok || fn.HasVarargs || fn.HasKwargs {
			return true
		}

		at := _Where(lambda.Lambda)

		for _, bind := range fn.FreeVars {
			taken[at] = append(taken[at], bind.First.Name)
		}

		return true
	})

	return taken
}

// _Spawned is the lambda a node hands the builtin spawn as its one argument, or
// nil when the node is anything else.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Spawned(node syntax.Node) *syntax.LambdaExpr {
	call, ok := node.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}

	name, ok := call.Fn.(*syntax.Ident)
	if !ok || name.Name != SPAWN || !_Predeclared(name) {
		return nil
	}

	lambda, ok := call.Args[0].(*syntax.LambdaExpr)
	if !ok {
		return nil
	}

	return lambda
}

// _Predeclared reports whether a resolved name is the environment's rather
// than one the script bound.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Predeclared(name *syntax.Ident) bool {
	bind, ok := name.Binding.(*resolve.Binding)

	return ok && bind.Scope == resolve.Predeclared
}

// _Take gives each lambda in taken a parameter per variable it reads,
// defaulting to that variable.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Take(compiled *syntax.File, taken map[_At][]string) {
	if len(taken) == 0 {
		return
	}

	syntax.Walk(compiled, func(node syntax.Node) bool {
		lambda, ok := node.(*syntax.LambdaExpr)
		if !ok {
			return true
		}

		for _, name := range taken[_Where(lambda.Lambda)] {
			lambda.Params = append(lambda.Params, _Default(name, lambda.Lambda))
		}

		return true
	})
}

// _Default is the parameter name=name.
//
// Placed where the lambda begins, so an error about the default - a variable
// read before it was assigned - points at the lambda that read it.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Default(name string, at syntax.Position) syntax.Expr {
	return &syntax.BinaryExpr{
		X:     &syntax.Ident{NamePos: at, Name: name},
		OpPos: at,
		Op:    syntax.EQ,
		Y:     &syntax.Ident{NamePos: at, Name: name},
	}
}

// _Where is where a lambda begins, without its file.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Where(pos syntax.Position) _At {
	return _At{pos.Line, pos.Col}
}
