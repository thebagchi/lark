package dialect

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
	"github.com/thebagchi/lark/v1/runtime/guard"
)

// Compile compiles a script as this dialect reads it. It works at the level of
// the syntax tree: it takes the script twice, written, which the caller has
// parsed and resolved, and compiled, a copy of it made by Copied before the
// first was resolved, and it returns the program. It parses nothing, merges
// nothing and reads no module.
//
// There is one way to compile a script in this runtime (plan Q36): the library
// reads the script and every module it loads, merges them into one tree
// (package merge), and hands that tree here. This function is the last step
// of that path, and is exported only because artifact, a package beside this
// one, takes it; a host compiles through the library's Compile, which merges.
// Calling this with a tree of one file compiles that file with no merge.
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
// Two trees, because resolving a tree mutates it and go.starlark.net will not
// resolve one twice: written is resolved to learn what each lambda reads,
// compiled is rewritten and compiled, and is resolved here by the interpreter.
// compiled is a copy of written made by Copied before written was resolved,
// rather than a second parse: the parse was most of a compile, and the copy is a
// fraction of it. The trees may hold the statements of many files, as a merge
// does: every node keeps the position it was written at, so a failure names the
// file and line its author wrote.
//
// Returns the interpreter's own resolve error for compiled, unwrapped: it opens
// with the file, line and column already.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-10-02 01:17: compiles a statement call of a script function through
//     CALL, and passes the hidden keywords a branch, a spawn and a wrapper read
//   - 2026-10-28 22:49: compiles a copy of the parsed tree in place of a second
//     parse, and refuses a tree with a node it cannot copy
//   - 2026-11-02 22:58: takes the two trees already made, so the caller parses, merges and
//     resolves, and this is the one way to compile (C44a)
//   - 2026-11-02 22:58: says what it takes and why it is the one compile (plan Q36)
//   - 2026-11-04 19:30: a nil tree, a nil predeclared or a tree that panics
//     the compile is ErrUncopied, not a panic (C87)
//   - 2026-11-10 11:32: uses rewrite.Marked, which moved to internal/rewrite (C120b, Q315)
//   - 2026-11-13 17:40: names package merge, formerly amalgam (C134, Q446)
func Compile(
	written *syntax.File,
	compiled *syntax.File,
	predeclared func(string) bool,
) (*starlark.Program, error) {
	if written == nil || compiled == nil {
		return nil, ErrUncopied
	}

	var (
		program *starlark.Program
		err     error
	)

	raised := guard.Contained(func() {
		rewrite.Marked(
			compiled,
			written,
		)

		program, err = starlark.FileProgram(
			compiled,
			predeclared,
		)
	})
	if raised != nil {
		return nil, fmt.Errorf(
			"%w: %w",
			ErrUncopied,
			raised,
		)
	}

	return program, err
}
