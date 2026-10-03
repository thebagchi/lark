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
	// through $GEN.Suite, $GEN.Items and $GEN.Node, which is how one template
	// with no {{define}} reaches the parts of a language that nests.
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
	// started, constants and arguments stay on consecutive lines, and main is
	// written as any function is. In a suite, a nested def has a blank line
	// before it, above any comment on it; a comment written above a statement
	// stays above it, and one written at the end of the block stands above the
	// block's last line. A comment above an elif or an else stays above it. A
	// suite's body is always on the lines after its header, so one written on
	// one line is rewritten. A node that was parsed keeps the text it was
	// written with, without its end-of-line comments, unless it holds a suite;
	// a node the flow built is spelled from its parts. The flow builds no
	// return, and a lambda only to pass arguments, with no parameters of its
	// own, so a parsed one is the only kind there is.
	SOURCE = `
{{- $GEN := .Gen -}}
{{- $SUITE := .Suite -}}
{{- $ITEMS := .Items -}}
{{- $NODE := .Node -}}
{{- $INDENT := .Indent -}}
{{- $NEWLINE := "\n" -}}
{{- $SEPARATOR := ", " -}}
{{- $MARGIN := "" -}}
{{- if $SUITE -}}
    {{- $STATEMENTS := $SUITE.Stmts -}}
    {{- $LAST := $GEN.Last $STATEMENTS -}}
    {{- range $INDEX, $STATEMENT := $STATEMENTS -}}
        {{- if $INDEX -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- if ($GEN.As $STATEMENT).Def -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- if eq $INDEX $LAST -}}
            {{- range $NOTE := $GEN.Tails $STATEMENTS -}}
                {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
            {{- end -}}
        {{- end -}}
        {{- range $NOTE := $GEN.Notes $STATEMENT -}}
            {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
        {{- end -}}
        {{- $GEN.Node $STATEMENT $INDENT -}}
    {{- end -}}
{{- else if $ITEMS -}}
    {{- range $INDEX, $ITEM := $ITEMS.Exprs -}}
        {{- if $INDEX -}}
            {{- $SEPARATOR -}}
        {{- end -}}
        {{- $GEN.Node $ITEM $MARGIN -}}
    {{- end -}}
{{- else if $NODE -}}
    {{- $AS := $GEN.As $NODE -}}
    {{- with $DEF := $AS.Def -}}
        {{- $NAME := $GEN.Name $DEF -}}
        {{- $PARAMS := $GEN.Items $DEF.Params -}}
        {{- $BODY := $GEN.Suite $DEF.Body ($GEN.Inner $INDENT) -}}
        {{- $INDENT}}def {{$NAME}}({{$PARAMS}}):
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $FOR := $AS.For -}}
        {{- $VARS := $GEN.Node $FOR.Vars $MARGIN -}}
        {{- $OVER := $GEN.Node $FOR.X $MARGIN -}}
        {{- $BODY := $GEN.Suite $FOR.Body ($GEN.Inner $INDENT) -}}
        {{- $INDENT}}for {{$VARS}} in {{$OVER}}:
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $WHILE := $AS.While -}}
        {{- $CONDITION := $GEN.Node $WHILE.Cond $MARGIN -}}
        {{- $BODY := $GEN.Suite $WHILE.Body ($GEN.Inner $INDENT) -}}
        {{- $INDENT}}while {{$CONDITION}}:
        {{- if $BODY -}}
            {{- $NEWLINE}}{{$BODY -}}
        {{- end -}}
    {{- else with $IF := $AS.If -}}
        {{- $WORD := "if" -}}
        {{- if $GEN.Chained $IF -}}
            {{- $WORD = "elif" -}}
        {{- end -}}
        {{- $CONDITION := $GEN.Node $IF.Cond $MARGIN -}}
        {{- $THEN := $GEN.Suite $IF.True ($GEN.Inner $INDENT) -}}
        {{- $INDENT}}{{$WORD}} {{$CONDITION}}:
        {{- if $THEN -}}
            {{- $NEWLINE}}{{$THEN -}}
        {{- end -}}
        {{- with $ELIF := $GEN.Elif $IF -}}
            {{- $NEWLINE -}}
            {{- range $NOTE := $GEN.Notes $ELIF -}}
                {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
            {{- end -}}
            {{- $GEN.Node $ELIF $INDENT -}}
        {{- else -}}
            {{- if $IF.False -}}
                {{- $NEWLINE -}}
                {{- range $NOTE := $GEN.Elses $IF -}}
                    {{- $INDENT}}{{$NOTE.Text}}{{$NEWLINE -}}
                {{- end -}}
                {{- $INDENT}}else:{{$NEWLINE}}
                {{- $GEN.Suite $IF.False ($GEN.Inner $INDENT) -}}
            {{- end -}}
        {{- end -}}
    {{- else with $TEXT := $GEN.Span $NODE -}}
        {{- $INDENT}}{{$TEXT -}}
    {{- else with $EXPRESSION := $AS.Expression -}}
        {{- $INDENT}}{{$GEN.Node $EXPRESSION.X $MARGIN -}}
    {{- else with $ASSIGN := $AS.Assign -}}
        {{- $LEFT := $GEN.Node $ASSIGN.LHS $MARGIN -}}
        {{- $RIGHT := $GEN.Node $ASSIGN.RHS $MARGIN -}}
        {{- $INDENT}}{{$LEFT}} {{$ASSIGN.Op}} {{$RIGHT -}}
    {{- else with $BRANCH := $AS.Branch -}}
        {{- $INDENT}}{{$BRANCH.Token -}}
    {{- else with $IDENT := $AS.Ident -}}
        {{- $IDENT.Name -}}
    {{- else with $LITERAL := $AS.Literal -}}
        {{- $LITERAL.Raw -}}
    {{- else with $CALL := $AS.Call -}}
        {{- $GEN.Node $CALL.Fn $MARGIN}}({{$GEN.Items $CALL.Args}})
    {{- else with $LAMBDA := $AS.Lambda -}}
        lambda: {{$GEN.Node $LAMBDA.Body $MARGIN -}}
    {{- else with $LIST := $AS.List -}}
        [{{$GEN.Items $LIST.List}}]
    {{- else with $DICT := $AS.Dict -}}
        {{- "{"}}{{$GEN.Items $DICT.List}}{{"}" -}}
    {{- else with $ENTRY := $AS.Entry -}}
        {{- $GEN.Node $ENTRY.Key $MARGIN}}: {{$GEN.Node $ENTRY.Value $MARGIN -}}
    {{- else with $BINARY := $AS.Binary -}}
        {{- $GEN.Node $BINARY.X $MARGIN}} {{$BINARY.Op}} {{$GEN.Node $BINARY.Y $MARGIN -}}
    {{- else -}}
        {{- $GEN.Refuse $NODE -}}
    {{- end -}}
{{- else -}}
    {{- $FUNCTIONS := $GEN.Functions -}}
    {{- $CONSTANTS := $GEN.Constants -}}
    {{- $ARGUMENTS := $GEN.Arguments -}}
    {{- $MAIN := $GEN.Main -}}
    {{- range $INDEX, $FUNCTION := $FUNCTIONS -}}
        {{- if $INDEX -}}
            {{- $NEWLINE -}}
        {{- end -}}
        {{- $GEN.Node $FUNCTION $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- range $CONSTANT := $CONSTANTS -}}
        {{$CONSTANT.LHS.Name}} = {{$GEN.Node $CONSTANT.RHS $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- range $ARGUMENT := $ARGUMENTS -}}
        {{$ARGUMENT.LHS.Name}} = {{$GEN.Node $ARGUMENT.RHS $MARGIN}}{{$NEWLINE -}}
    {{- end -}}
    {{- if or $FUNCTIONS $CONSTANTS $ARGUMENTS -}}
        {{- $NEWLINE -}}
    {{- end -}}
    {{- $GEN.Node $MAIN $MARGIN}}{{$NEWLINE -}}
{{- end -}}
`
)

var (
	// PARSED is SOURCE parsed, the first time a file is written, and kept for
	// every file after it: SOURCE is a constant, and parsing it again was a
	// share of every Emit. Its error is kept too, so a template that will not
	// parse fails each Emit rather than the process.
	PARSED = sync.OnceValues(func() (*template.Template, error) {
		return template.New("starlark").Parse(SOURCE)
	})
)

// _Part is what one pass of SOURCE writes, at Indent: the statements of Suite
// when it is set, else the items of Items when that is set, else Node when it
// is set, else the whole file.
//
// Suite and Items are pointers so that a suite with no statements, or a call
// with no arguments, is still that part, and not the file. The fields are
// exported because a template reaches nothing else.
type _Part struct {
	Gen    *_Program
	Suite  *_Suite
	Items  *_Items
	Node   syntax.Node
	Indent string
}

// _Suite is the statements of one suite: a def's, a loop's or a branch's.
type _Suite struct {
	Stmts []syntax.Stmt
}

// _Items is the expressions of one list a script separates with commas: a
// call's arguments, a def's parameters, a list's items, a dict's entries.
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

// _Print writes the file with SOURCE.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-01 15:52: no blank line between sections
//   - 2026-10-01 16:08: the template lays the file out, with a blank line
//     before each function
//   - 2026-10-02 11:04: parses SOURCE once, for every pass that writes a part
//   - 2026-10-03 20:43: leaves SOURCE to PARSED, which parses it once for every file
func _Print(prog *_Program) ([]byte, error) {
	written, err := _Rendered(&_Part{Gen: prog})
	if err != nil {
		return nil, err
	}

	return []byte(written), nil
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
		return "", fmt.Errorf("template: %w", err)
	}

	var out strings.Builder

	err = held.Execute(&out, part)
	if err != nil {
		return "", fmt.Errorf("script: %w", err)
	}

	return out.String(), nil
}

// Functions is every function, in the order the flow declared them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
func (p *_Program) Functions() []*syntax.DefStmt {
	return p.functions
}

// Constants is every constant's assignment, in the order the file writes them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-10-03 20:43: the assignments, which the template writes as it writes any,
//     rather than their names, which it had to look each up by
func (p *_Program) Constants() []syntax.Stmt {
	return p.constants
}

// Arguments is every argument's declaration, in the order the file writes
// them.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
//   - 2026-10-03 20:43: the declarations, which the template writes as it writes any
//     assignment, rather than their names
func (p *_Program) Arguments() []syntax.Stmt {
	return p.arguments
}

// Main is the entry function.
//
// Revisions:
//   - 2026-10-01 16:08: initial creation
func (p *_Program) Main() *syntax.DefStmt {
	return p.main
}

// Suite is the statements of a suite at indent, written by a pass of SOURCE.
// Empty when there are none.
//
// Revisions:
//   - 2026-10-02 11:04: initial creation, in place of Body, which took a def
//   - 2026-10-02 11:26: the pass itself, rather than through _RenderBody, which is gone
func (p *_Program) Suite(stmts []syntax.Stmt, indent string) (string, error) {
	return _Rendered(&_Part{Gen: p, Suite: &_Suite{Stmts: stmts}, Indent: indent})
}

// Items is the expressions of a list a script separates with commas, written
// by a pass of SOURCE. Empty when there are none.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
func (p *_Program) Items(exprs []syntax.Expr) (string, error) {
	return _Rendered(&_Part{Gen: p, Items: &_Items{Exprs: exprs}})
}

// Node is one node at indent, written by a pass of SOURCE.
//
// Returns ERR_FORM for no node at all, which a pass would otherwise read as the
// whole file.
//
// Revisions:
//   - 2026-10-02 11:04: initial creation
//   - 2026-10-02 11:26: refuses no node, which would be read as the file
func (p *_Program) Node(node syntax.Node, indent string) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node: %w", ERR_FORM)
	}

	return _Rendered(&_Part{Gen: p, Node: node, Indent: indent})
}

// As is node as each kind SOURCE writes, with only its own kind's field set,
// so the template asks once and reads the answer for each kind it writes.
//
// Revisions:
//   - 2026-10-03 20:46: initial creation, in place of a query for each kind
func (p *_Program) As(node syntax.Node) *_Kinds {
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
func (p *_Program) Name(node *syntax.DefStmt) (string, error) {
	if node == nil || node.Name == nil {
		return "", fmt.Errorf("def: %w", ERR_FORM)
	}

	return node.Name.Name, nil
}

// Inner is the indentation one level further in than indent, where a suite's
// statements start.
//
// Revisions:
//   - 2026-10-02 10:58: initial creation
func (p *_Program) Inner(indent string) string {
	return indent + INDENT
}

// Last is the position of the last statement in a list, where the block's own
// closing comments go.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
func (p *_Program) Last(stmts []syntax.Stmt) int {
	return len(stmts) - 1
}

// Tails is the comments written at the end of a block, which stand above its
// last line.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
func (p *_Program) Tails(stmts []syntax.Stmt) []syntax.Comment {
	if len(stmts) == 0 {
		return nil
	}

	return p.tails[stmts[0]]
}

// Notes is the comments written above a statement.
//
// Revisions:
//   - 2026-10-02 08:40: initial creation
func (p *_Program) Notes(stmt syntax.Stmt) []syntax.Comment {
	return p.notes[stmt]
}

// Span is the text a parsed node was written with, without its end-of-line
// comments, or empty for a node the flow built.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
func (p *_Program) Span(node syntax.Node) string {
	text, ok := _Span(p, node)
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
func (p *_Program) Chained(node *syntax.IfStmt) bool {
	_, marked := p.elif[node]

	return marked
}

// Elif is the if an if's else holds when that if continues it, or nil when
// the else is an else, or there is none.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
func (p *_Program) Elif(node *syntax.IfStmt) *syntax.IfStmt {
	return _ElifOf(node, p)
}

// Elses is the comments written above an if's else.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
func (p *_Program) Elses(node *syntax.IfStmt) []syntax.Comment {
	return p.elses[node]
}

// Refuse is the failure for a node no branch of SOURCE writes.
//
// Returns ERR_FORM, always: a node the flow built that is none of the kinds a
// script is written with is a flow this cannot write.
//
// Revisions:
//   - 2026-10-02 11:26: initial creation
func (p *_Program) Refuse(node syntax.Node) (string, error) {
	return "", fmt.Errorf("%T: %w", node, ERR_FORM)
}
