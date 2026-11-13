package graph

import (
	"fmt"
	"strings"
	"sync"
	"text/template"

	"go.starlark.net/syntax"
)

const (
	// SOURCE is the script, in one unnamed template. Each pass of it writes one
	// part at an indentation: the statements of a suite, the items of a list
	// separated as a script separates them, one node, or, given none of those,
	// the whole file. A part inside another is written by a pass of its own,
	// through the functions Suite, Items and Node, each of which takes $GEN
	// first, which is how one template with no {{define}} reaches the parts of
	// a language that nests. Every query the program answers is such a function,
	// set in PARSED, so a call is a lookup in a map and no search of the
	// program's methods by reflection on each pass.
	//
	// Every decision about the script is made here: the words, the brackets and
	// separators, where a line breaks, where a blank line or a comment goes, how
	// far in a body is, and which way each node is written. The program answers
	// only what the flow holds: a node as one kind or nil, a name, the comments
	// placed around a statement, the text a parsed node was written as, and
	// which if continues another.
	//
	// The template's own indentation is for reading it, and none of it reaches
	// the script: every action that shapes the script trims the whitespace
	// around it, so each line the script gets ends in an explicit $NEWLINE. Nor
	// does a pass indent what another pass wrote: a part arrives already at its
	// indentation, because Starlark has no formatter and its whitespace is
	// syntax.
	//
	// In the file, a blank line stands before each function once the file has
	// started, constants and arguments stay on consecutive lines, the init
	// statements follow them, and main is written as any function is. In a
	// suite, a nested def has a blank line before it, above any comment on it;
	// a comment written above a statement
	// stays above it, and one written at the end of the block stands above the
	// block's last line. A comment above an elif or an else stays above it. A
	// suite's body is always on the lines after its header, so one written on
	// one line is rewritten. A node that was parsed keeps the text it was
	// written with, without its end-of-line comments, unless it holds a suite;
	// a node the flow built is spelled from its parts. The flow builds no
	// return, and a lambda only to pass arguments, with no parameters of its
	// own, so a parsed one is the only kind there is.
	//
	// Revisions:
	//   - 2026-11-02 16:33: a pass binds only the names its branch uses, rather
	//     than eight names in a header
	//   - 2026-11-02 17:06: a query is a function, named as the method, that
	//     takes $GEN first, rather than
	//     a method of it
	//   - 2026-11-04 19:26: the file writes the init after the arguments
	//     (C44c, B054)
	SOURCE = `
{{- $GEN := .Gen -}}
{{- if .Suite -}}
    {{- $INDENT := .Indent -}}
    {{- $NEWLINE := "\n" -}}
    {{- $STATEMENTS := .Suite.Stmts -}}
    {{- $LAST := Last $GEN $STATEMENTS -}}
    {{- range $INDEX, $STATEMENT := $STATEMENTS -}}
        {{- if $INDEX -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- if (As $GEN $STATEMENT).Def -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- if eq $INDEX $LAST -}}
            {{- range $NOTE := Tails $GEN $STATEMENTS -}}
                {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
            {{- end -}}
        {{- end -}}
        {{- range $NOTE := Notes $GEN $STATEMENT -}}
            {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
        {{- end -}}
        {{- Node $GEN $STATEMENT $INDENT -}}
    {{- end -}}
{{- else if .Items -}}
    {{- $SEPARATOR := ", " -}}
    {{- $MARGIN := "" -}}
    {{- range $INDEX, $ITEM := .Items.Exprs -}}
        {{- if $INDEX -}}
            {{- $SEPARATOR -}}
        {{- end -}}
        {{- Node $GEN $ITEM $MARGIN -}}
    {{- end -}}
{{- else if .Node -}}
    {{- $NODE := .Node -}}
    {{- $INDENT := .Indent -}}
    {{- $NEWLINE := "\n" -}}
    {{- $MARGIN := "" -}}
    {{- $AS := As $GEN $NODE -}}
    {{- with $DEF := $AS.Def -}}
        {{- $NAME := Name $GEN $DEF -}}
        {{- $PARAMS := Items $GEN $DEF.Params -}}
        {{- $BODY := Suite $GEN $DEF.Body (Inner $GEN $INDENT) -}}
        {{- $INDENT}}def {{$NAME}}({{$PARAMS}}):
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $FOR := $AS.For -}}
        {{- $VARS := Node $GEN $FOR.Vars $MARGIN -}}
        {{- $OVER := Node $GEN $FOR.X $MARGIN -}}
        {{- $BODY := Suite $GEN $FOR.Body (Inner $GEN $INDENT) -}}
        {{- $INDENT}}for {{$VARS}} in {{$OVER}}:
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $WHILE := $AS.While -}}
        {{- $CONDITION := Node $GEN $WHILE.Cond $MARGIN -}}
        {{- $BODY := Suite $GEN $WHILE.Body (Inner $GEN $INDENT) -}}
        {{- $INDENT}}while {{$CONDITION}}:
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $IF := $AS.If -}}
        {{- $WORD := "if" -}}
        {{- if Chained $GEN $IF -}}
            {{- $WORD = "elif" -}}
        {{- end -}}
        {{- $CONDITION := Node $GEN $IF.Cond $MARGIN -}}
        {{- $THEN := Suite $GEN $IF.True (Inner $GEN $INDENT) -}}
        {{- $INDENT}}{{$WORD}} {{$CONDITION}}:
        {{- if $THEN -}}
            {{- $NEWLINE}}{{$THEN -}}
        {{- end -}}
        {{- with $ELIF := Elif $GEN $IF -}}
            {{- $NEWLINE -}}
            {{- range $NOTE := Notes $GEN $ELIF -}}
                {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
            {{- end -}}
            {{- Node $GEN $ELIF $INDENT -}}
        {{- else -}}
            {{- if $IF.False -}}
                {{- $NEWLINE -}}
                {{- range $NOTE := Elses $GEN $IF -}}
                    {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
                {{- end -}}
                {{- $INDENT}}else:{{$NEWLINE}}
                {{- Suite $GEN $IF.False (Inner $GEN $INDENT) -}}
            {{- end -}}
        {{- end -}}
    {{- else with $TEXT := Span $GEN $NODE -}}
        {{- $INDENT}}{{$TEXT -}}
    {{- else with $EXPRESSION := $AS.Expression -}}
        {{- $INDENT}}{{Node $GEN $EXPRESSION.X $MARGIN -}}
    {{- else with $ASSIGN := $AS.Assign -}}
        {{- $LEFT := Node $GEN $ASSIGN.LHS $MARGIN -}}
        {{- $RIGHT := Node $GEN $ASSIGN.RHS $MARGIN -}}
        {{- $INDENT}}{{$LEFT}} {{$ASSIGN.Op}} {{$RIGHT -}}
    {{- else with $BRANCH := $AS.Branch -}}
        {{- $INDENT}}{{$BRANCH.Token -}}
    {{- else with $IDENT := $AS.Ident -}}
        {{- $IDENT.Name -}}
    {{- else with $LITERAL := $AS.Literal -}}
        {{- $LITERAL.Raw -}}
    {{- else with $CALL := $AS.Call -}}
        {{- Node $GEN $CALL.Fn $MARGIN}}({{Items $GEN $CALL.Args}})
    {{- else with $LAMBDA := $AS.Lambda -}}
        lambda: {{Node $GEN $LAMBDA.Body $MARGIN -}}
    {{- else with $LIST := $AS.List -}}
        [{{Items $GEN $LIST.List}}]
    {{- else with $DICT := $AS.Dict -}}
        {{- "{"}}{{Items $GEN $DICT.List}}{{"}" -}}
    {{- else with $ENTRY := $AS.Entry -}}
        {{- Node $GEN $ENTRY.Key $MARGIN}}: {{Node $GEN $ENTRY.Value $MARGIN -}}
    {{- else with $BINARY := $AS.Binary -}}
        {{- Node $GEN $BINARY.X $MARGIN}} {{$BINARY.Op}} {{Node $GEN $BINARY.Y $MARGIN -}}
    {{- else -}}
        {{- Refuse $GEN $NODE -}}
    {{- end -}}
{{- else -}}
    {{- $NEWLINE := "\n" -}}
    {{- $MARGIN := "" -}}
    {{- $FUNCTIONS := Functions $GEN -}}
    {{- $CONSTANTS := Constants $GEN -}}
    {{- $ARGUMENTS := Arguments $GEN -}}
    {{- $INIT := Init $GEN -}}
    {{- $MAIN := Main $GEN -}}
    {{- range $INDEX, $FUNCTION := $FUNCTIONS -}}
        {{- if $INDEX -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- Node $GEN $FUNCTION $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- range $CONSTANT := $CONSTANTS -}}
        {{$CONSTANT.LHS.Name}} = {{Node $GEN $CONSTANT.RHS $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- range $ARGUMENT := $ARGUMENTS -}}
        {{$ARGUMENT.LHS.Name}} = {{Node $GEN $ARGUMENT.RHS $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- with $STATEMENTS := Suite $GEN $INIT $MARGIN -}}
        {{- $STATEMENTS}}{{$NEWLINE -}}
    {{- end -}}
    {{- if or $FUNCTIONS $CONSTANTS $ARGUMENTS $INIT -}}
        {{- $NEWLINE -}}
    {{- end -}}
    {{- Node $GEN $MAIN $MARGIN}}{{$NEWLINE -}}
{{- end -}}
`
)

var (
	// PARSED is SOURCE parsed, the first time a file is written, and kept for
	// every file after it: SOURCE is a constant, and parsing it again was a
	// share of every Emit. Its error is kept too, so a template that will not
	// parse fails each Emit rather than the process.
	//
	// init sets it, and not this declaration, because the functions it gives
	// SOURCE reach _Rendered, which reaches PARSED, and Go refuses an
	// initialization cycle.
	//
	// Revisions:
	//   - 2026-11-02 17:06: set by init, with the queries as functions of
	//     SOURCE, rather than methods found by reflection
	PARSED func() (*template.Template, error)
)

// init sets PARSED. See PARSED for why it is not set in its declaration.
//
// Revisions:
//   - 2026-11-02 17:06: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func init() {
	PARSED = sync.OnceValues(func() (*template.Template, error) {
		return template.New("starlark").Funcs(template.FuncMap{
			"Arguments": (*Program).Arguments,
			"As":        (*Program).As,
			"Chained":   (*Program).Chained,
			"Constants": (*Program).Constants,
			"Elif":      (*Program).Elif,
			"Elses":     (*Program).Elses,
			"Functions": (*Program).Functions,
			"Inner":     (*Program).Inner,
			"Items":     (*Program).Items,
			"Last":      (*Program).Last,
			"Main":      (*Program).Main,
			"Name":      (*Program).Name,
			"Node":      (*Program).Node,
			"Notes":     (*Program).Notes,
			"Refuse":    (*Program).Refuse,
			"Init":      (*Program).Init,
			"Span":      (*Program).Span,
			"Suite":     (*Program).Suite,
			"Tails":     (*Program).Tails,
		}).Parse(SOURCE)
	})
}

// _Part is what one pass of SOURCE writes, at Indent: the statements of Suite
// when it is set, else the items of Items when that is set, else Node when it
// is set, else the whole file.
//
// Suite and Items are pointers so that a suite with no statements, or a call
// with no arguments, is still that part, and not the file. The fields are
// exported because a template reaches nothing else.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
type _Part struct {
	Gen    *Program
	Suite  *_Suite
	Items  *_Items
	Node   syntax.Node
	Indent string
}

// _Suite is the statements of one suite: a def's, a loop's or a branch's.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Suite struct {
	Stmts []syntax.Stmt
}

// _Items is the expressions of one list a script separates with commas: a
// call's arguments, a def's parameters, a list's items, a dict's entries.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Items struct {
	Exprs []syntax.Expr
}

// _Kinds is one node as each kind SOURCE writes: every field nil but the one
// its kind fills, and all of them nil for a kind SOURCE does not write.
//
// One query that answers every kind, rather than one query for each, which
// SOURCE asked in turn - fifteen calls to reach a binary expression. Reading a
// field is not a call, so finding a node's kind is one call and a few reads:
// measured on the fifteenth kind, a third of the time of the calls in turn. The
// fields are exported because a template reaches nothing else.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Kinds struct {
	Def        *syntax.DefStmt
	For        *syntax.ForStmt
	While      *syntax.WhileStmt
	If         *syntax.IfStmt
	Expression *syntax.ExprStmt
	Assign     *syntax.AssignStmt
	Branch     *syntax.BranchStmt
	Ident      *syntax.Ident
	Literal    *syntax.Literal
	Call       *syntax.CallExpr
	Lambda     *syntax.LambdaExpr
	List       *syntax.ListExpr
	Dict       *syntax.DictExpr
	Entry      *syntax.DictEntry
	Binary     *syntax.BinaryExpr
}

// _Rendered is one pass of SOURCE over part.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
//   - 2026-10-02 11:04: a pass of the one template, which the program holds
//   - 2026-10-03 20:43: a pass of PARSED, which no program holds
func _Rendered(part *_Part) (string, error) {
	held, err := PARSED()
	if err != nil {
		return "", fmt.Errorf(
			"template: %w",
			err,
		)
	}

	var out strings.Builder

	err = held.Execute(
		&out,
		part,
	)
	if err != nil {
		return "", fmt.Errorf(
			"script: %w",
			err,
		)
	}

	return out.String(), nil
}

// Functions is every function, in the order the flow declared them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Functions() []*syntax.DefStmt {
	return p.functions
}

// Constants is every constant's assignment, in the order the file writes them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-10-03 20:43: the assignments, which the template writes as it writes any,
//     rather than their names, which it had to look each up by
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Constants() []syntax.Stmt {
	return p.constants
}

// Arguments is every argument's declaration, in the order the file writes
// them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-10-03 20:43: the declarations, which the template writes as it writes any
//     assignment, rather than their names
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Arguments() []syntax.Stmt {
	return p.arguments
}

// Init is the entry script's top-level statements, in the order it wrote
// them, which the file holds after its arguments.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Init() []syntax.Stmt {
	return p.init
}

// Main is the entry function.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Main() *syntax.DefStmt {
	return p.main
}

// Suite is the statements of a suite at indent, written by a pass of SOURCE.
// Empty when there are none.
//
// Revisions:
//   - 2026-10-02 11:04: initial creation, in place of Body, which took a def
//   - 2026-10-02 11:26: the pass itself, rather than through _RenderBody, which is gone
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Suite(
	stmts []syntax.Stmt,
	indent string,
) (string, error) {
	return _Rendered(&_Part{
		Gen: p,
		Suite: &_Suite{
			Stmts: stmts,
		},
		Indent: indent,
	})
}

// Items is the expressions of a list a script separates with commas, written
// by a pass of SOURCE. Empty when there are none.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Items(exprs []syntax.Expr) (string, error) {
	return _Rendered(&_Part{
		Gen: p,
		Items: &_Items{
			Exprs: exprs,
		},
	})
}

// Node is one node at indent, written by a pass of SOURCE.
//
// Returns ErrForm for no node at all, which a pass would otherwise read as the
// whole file.
//
// Revisions:
//   - 2026-10-02 11:04: initial creation
//   - 2026-10-02 11:26: refuses no node, which would be read as the file
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Node(
	node syntax.Node,
	indent string,
) (string, error) {
	if node == nil {
		return "", fmt.Errorf(
			"node: %w",
			ErrForm,
		)
	}

	return _Rendered(&_Part{
		Gen:    p,
		Node:   node,
		Indent: indent,
	})
}

// As is node as each kind SOURCE writes, with only its own kind's field set,
// so the template asks once and reads the answer for each kind it writes.
//
// Revisions:
//   - 2026-10-03 20:46: initial creation, in place of a query for each kind
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) As(node syntax.Node) *_Kinds {
	kinds := new(_Kinds)

	switch held := node.(type) {
	case *syntax.DefStmt:
		kinds.Def = held

	case *syntax.ForStmt:
		kinds.For = held

	case *syntax.WhileStmt:
		kinds.While = held

	case *syntax.IfStmt:
		kinds.If = held

	case *syntax.ExprStmt:
		kinds.Expression = held

	case *syntax.AssignStmt:
		kinds.Assign = held

	case *syntax.BranchStmt:
		kinds.Branch = held

	case *syntax.Ident:
		kinds.Ident = held

	case *syntax.Literal:
		kinds.Literal = held

	case *syntax.CallExpr:
		kinds.Call = held

	case *syntax.LambdaExpr:
		kinds.Lambda = held

	case *syntax.ListExpr:
		kinds.List = held

	case *syntax.DictExpr:
		kinds.Dict = held

	case *syntax.DictEntry:
		kinds.Entry = held

	case *syntax.BinaryExpr:
		kinds.Binary = held
	}

	return kinds
}

// Name is a function's name, as it is written after def.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Name(node *syntax.DefStmt) (string, error) {
	if node == nil || node.Name == nil {
		return "", fmt.Errorf(
			"def: %w",
			ErrForm,
		)
	}

	return node.Name.Name, nil
}

// Inner is the indentation one level further in than indent, where a suite's
// statements start.
//
// Revisions:
//   - 2026-10-02 10:58: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Inner(indent string) string {
	return indent + INDENT
}

// Last is the position of the last statement in a list, where the block's own
// closing comments go.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Last(stmts []syntax.Stmt) int {
	return len(stmts) - 1
}

// Tails is the comments written at the end of a block, which stand above its
// last line.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
//   - 2026-11-06 22:02: reads a nil program as one with no comments (C100c, B147)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Tails(stmts []syntax.Stmt) []syntax.Comment {
	if p == nil || len(stmts) == 0 {
		return nil
	}

	return p.tails[stmts[0]]
}

// Notes is the comments written above a statement.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Notes(stmt syntax.Stmt) []syntax.Comment {
	return p.notes[stmt]
}

// Span is the text a parsed node was written with, without its end-of-line
// comments, or empty for a node the flow built.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Span(node syntax.Node) string {
	text, ok := _Span(
		p,
		node,
	)
	if !ok {
		return ""
	}

	return text
}

// Chained reports whether an if continues the if before it, and so is written
// as an elif.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Chained(node *syntax.IfStmt) bool {
	_, marked := p.elif[node]

	return marked
}

// Elif is the if an if's else holds when that if continues it, or nil when
// the else is an else, or there is none.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Elif(node *syntax.IfStmt) *syntax.IfStmt {
	return _ElifOf(
		node,
		p,
	)
}

// Elses is the comments written above an if's else.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Elses(node *syntax.IfStmt) []syntax.Comment {
	return p.elses[node]
}

// Refuse is the failure for a node no branch of SOURCE writes.
//
// Returns ErrForm, always: a node the flow built that is none of the kinds a
// script is written with is a flow this cannot write.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) Refuse(node syntax.Node) (string, error) {
	return "", fmt.Errorf(
		"%T: %w",
		node,
		ErrForm,
	)
}
