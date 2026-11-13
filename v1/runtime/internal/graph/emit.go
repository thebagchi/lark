package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// INDENT is one level, as a script writes it, and NEWLINE what ends a line.
//
// Revisions:
//   - 2026-09-20 21:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	INDENT  = "    "
	NEWLINE = "\n"

	// END closes a suite that does not return. It is punctuation: the flow
	// does not store it, and a suite that already ends with it is left as it
	// is.
	//
	// Revisions:
	//   - 2026-09-20 21:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	END = "pass"

	// ELIF is the word an if that continues another is read by.
	//
	// Revisions:
	//   - 2026-09-20 21:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ELIF = "elif"

	// INIT names the entry's top-level statements where an error or a parsed
	// file needs a name for them. It is not a name in the script.
	//
	// Revisions:
	//   - 2026-11-05 00:01: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	INIT = "init"

	// EXPRESSION names an expression carried as text, where a parse error
	// needs a name for it.
	//
	// Revisions:
	//   - 2026-11-05 00:01: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	EXPRESSION = "expression"

	// FIRST is the line and column a parser counts from.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	FIRST = 1

	// MISSING is a position that is not in the file.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	MISSING = -1
)

var (
	// ErrForm is a statement this generator does not write, and a flow that
	// cannot be written: a function or main with nothing to write (code
	// unset, text that is empty or only comments, or a statement list with no
	// statements), and a call carrying both args and operands, two lists for
	// one argument list with nothing to say which is meant, and a name that is
	// not a Starlark identifier or is a keyword. It is also a value that is none: a
	// Value with no kind set, a null kind that holds anything but null, a string
	// that is not valid UTF-8, and a float that is NaN or an infinity. The wrap
	// names which.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 11:55: a name that is no identifier (C118, B259, Q300)
	//   - 2026-11-13 10:15: a value that is none (C132, Q407, Q421)
	ErrForm = errors.New("no form")
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
// A statement with no form this writes is ErrForm. So are a function or main
// with no body, an empty statement list, and body text that is only comments.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
//   - 2026-10-02 00:04: the emit POC's printer, lifted: builds a syntax tree
//     from a Flow and prints it, closes every suite with pass, trims every
//     trailing comment, and keeps each line comment where it was written
//   - 2026-11-04 19:26: writes the flow's init after the arguments (C44c,
//     B054)
//   - 2026-11-05 19:07: renders SOURCE itself, with no one-use _Print (C58)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
func Emit(flow *workflowpb.Flow) ([]byte, error) {
	prog, err := _Build(flow)
	if err != nil {
		return nil, err
	}

	_Close(prog)

	written, err := _Rendered(&_Part{
		Gen: prog,
	})
	if err != nil {
		return nil, err
	}

	return []byte(written), nil
}

// _Build is the file as a syntax tree, before suite closers are written.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow, and
//     keeping where each body's comments go
//   - 2026-11-05 23:24: the sources map holds *Lines in place of *_Lines (C60, Q188)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Build(flow *workflowpb.Flow) (*Program, error) {
	var prog = &Program{
		elif:    map[*syntax.IfStmt]struct{}{},
		sources: map[string]*Lines{},
		cuts:    map[string][]syntax.Comment{},
		notes:   map[syntax.Node][]syntax.Comment{},
		elses:   map[*syntax.IfStmt][]syntax.Comment{},
		tails:   map[syntax.Stmt][]syntax.Comment{},
	}

	functions, err := _Functions(
		prog,
		flow,
	)
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

	initial, err := _Init(
		prog,
		flow,
	)
	if err != nil {
		return nil, err
	}

	main, err := _Main(
		prog,
		flow,
	)
	if err != nil {
		return nil, err
	}

	prog.functions = functions
	prog.constants = constants
	prog.arguments = arguments
	prog.init = initial
	prog.main = main

	return prog, nil
}

// _Init is the statements the entry script runs when it is set up, or none:
// a call as the flow states it, and any other statement as the script wrote
// it.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
//   - 2026-11-12 18:21: parses the text of a statement at the margin, with _Margined (C128, B279)
func _Init(
	prog *Program,
	flow *workflowpb.Flow,
) ([]syntax.Stmt, error) {
	var stmts []syntax.Stmt

	for _, held := range flow.GetInit() {
		var (
			part []syntax.Stmt
			err  error
		)

		switch form := held.GetForm().(type) {
		case *workflowpb.Init_Call:
			part, err = _Single(_Call(form.Call))

		case *workflowpb.Init_Text:
			part, err = _Margined(
				prog,
				INIT,
				form.Text,
			)

		default:
			err = fmt.Errorf(
				"%s: %w",
				INIT,
				ErrForm,
			)
		}

		if err != nil {
			return nil, err
		}

		stmts = append(
			stmts,
			part...,
		)
	}

	return stmts, nil
}

// _Functions is every function, in the order the flow declares them.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the defs, which the printer places
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Functions(
	prog *Program,
	flow *workflowpb.Flow,
) ([]*syntax.DefStmt, error) {
	var defs []*syntax.DefStmt

	for _, fn := range flow.GetFunctions() {
		def, err := _Function(
			prog,
			fn,
		)
		if err != nil {
			return nil, err
		}

		defs = append(
			defs,
			def,
		)
	}

	return defs, nil
}

// Function is one def, without the blank line after it.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the def node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Function
//   - 2026-11-10 01:00: now uses Function, Program, Code, were _Function, _Program, _Code
//     (C120a, Q314)
func _Function(
	prog *Program,
	fn *workflowpb.Function,
) (*syntax.DefStmt, error) {
	if fn.GetName() == "" {
		return nil, fmt.Errorf(
			"function: %w",
			ErrForm,
		)
	}

	body, err := Code(
		prog,
		fn,
	)
	if err != nil {
		return nil, err
	}

	return _Def(
		fn.GetName(),
		fn.GetParams(),
		body,
	), nil
}

// _Main is def main.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the def node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Main(
	prog *Program,
	flow *workflowpb.Flow,
) (*syntax.DefStmt, error) {
	body, err := _Entry(
		prog,
		flow,
	)
	if err != nil {
		return nil, err
	}

	return _Def(
		common.ENTRY,
		nil,
		body,
	), nil
}
