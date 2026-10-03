package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// INDENT is one level, as a script writes it, and NEWLINE what ends a line.
	INDENT  = "    "
	NEWLINE = "\n"

	// END closes a suite that does not return. It is punctuation: the flow
	// does not store it, and a suite that already ends with it is left as it
	// is.
	END = "pass"

	// ELIF is the word an if that continues another is read by.
	ELIF = "elif"

	// FIRST is the line and column a parser counts from.
	FIRST = 1

	// MISSING is a position that is not in the file.
	MISSING = -1
)

var (
	// ERR_NO_BODY is a function or main with nothing to write: code unset, text
	// that is empty or only comments, or a statement list with no statements.
	ERR_NO_BODY = errors.New("function has no body")

	// ERR_FORM is a statement this generator does not write.
	ERR_FORM = errors.New("no form")
)

// Emit is the Starlark a flow describes, ready for a compiler.
//
// The flow is built as a Starlark syntax tree. Body text is parsed into that
// tree. A suite whose last statement is not a return gains a pass: a
// function, and the body of an if, an elif, an else, a for and a while. A
// suite that is only pass stays one pass. A suite that returns has no pass
// after the return. One template renders the file. A suite's body is on the
// following lines, so one written on its header's line is rewritten there.
//
// A generated script has only line comments. A comment at the end of a line
// is trimmed. A comment on a line of its own stays where it was written: above
// the statement, the elif or the else it preceded, or at the end of the block
// it was written in, where it stands above that block's pass, or above the
// block's final return. The only blank line is the one before a function,
// above any comment on that function. The flow does not store that pass.
//
// It does not compile. A caller wanting an artifact passes this to a compiler,
// so a body that is not Starlark is a compile error with a line number rather
// than something this package invents a message for.
//
// A statement with no form this writes is ERR_FORM. A function or main with no
// body, an empty statement list, and body text that is only comments, are
// ERR_NO_BODY.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
//   - 2026-10-02 00:04: the emit POC's printer, lifted: builds a syntax tree
//     from a Flow and prints it, closes every suite with pass, trims every
//     trailing comment, and keeps each line comment where it was written
func Emit(flow *workflowpb.Flow) ([]byte, error) {
	prog, err := _Build(flow)
	if err != nil {
		return nil, err
	}

	_Close(prog)

	return _Print(prog)
}

// _Build is the file as a syntax tree, before suite closers are written.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow, and
//     keeping where each body's comments go
func _Build(flow *workflowpb.Flow) (*_Program, error) {
	prog := &_Program{
		elif:    map[*syntax.IfStmt]struct{}{},
		sources: map[string]*_Lines{},
		cuts:    map[string][]syntax.Comment{},
		notes:   map[syntax.Node][]syntax.Comment{},
		elses:   map[*syntax.IfStmt][]syntax.Comment{},
		tails:   map[syntax.Stmt][]syntax.Comment{},
	}

	functions, err := _Functions(prog, flow)
	if err != nil {
		return nil, err
	}

	constants, err := _Constants(flow)
	if err != nil {
		return nil, err
	}

	arguments, err := _Arguments(flow)
	if err != nil {
		return nil, err
	}

	main, err := _Main(prog, flow)
	if err != nil {
		return nil, err
	}

	prog.functions = functions
	prog.constants = constants
	prog.arguments = arguments
	prog.main = main

	return prog, nil
}

// _Functions is every function, in the order the flow declares them.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the defs, which the printer places
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
func _Functions(prog *_Program, flow *workflowpb.Flow) ([]*syntax.DefStmt, error) {
	var defs []*syntax.DefStmt

	for _, fn := range flow.GetFunctions() {
		def, err := _Function(prog, fn)
		if err != nil {
			return nil, err
		}

		defs = append(defs, def)
	}

	return defs, nil
}

// _Function is one def, without the blank line after it.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the def node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Function
func _Function(prog *_Program, fn *workflowpb.Function) (*syntax.DefStmt, error) {
	if fn.GetName() == "" {
		return nil, fmt.Errorf("function: %w", ERR_FORM)
	}

	body, err := _Code(prog, fn)
	if err != nil {
		return nil, err
	}

	return _Def(fn.GetName(), fn.GetParams(), body), nil
}

// _Main is def main.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the def node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
func _Main(prog *_Program, flow *workflowpb.Flow) (*syntax.DefStmt, error) {
	body, err := _Entry(prog, flow)
	if err != nil {
		return nil, err
	}

	return _Def(ENTRY, nil, body), nil
}
