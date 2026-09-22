package graph

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// INDENT is one level, as a script writes it.
	INDENT = "    "

	// END closes every generated function.
	//
	// A block in this language ends where its indentation does, and nothing
	// marks that. In a file of generated defs a reader has to find the end of
	// one by looking for the start of the next; a last line at the body's own
	// indentation says it here.
	//
	// It is unreachable after a return and that is fine - it is punctuation,
	// not code. It also means a def with nothing in it needs no special case,
	// because this is already a statement.
	END = "pass"

	// SEPARATOR joins a function's parameters between the brackets.
	SEPARATOR = ", "
)

// SOURCE is the whole of the rendering.
//
// Constants come after the defs. A constant may be a call of a function this
// graph declares, so the functions have to exist by then; and a def resolves
// the names in its body when it runs rather than when it is written, so a body
// reading a constant declared below it is fine.
//
// Arguments come after the constants. Nothing forces the order - a constant
// cannot read an argument, because a constant is a literal or a call of a
// declared function and neither reads a name - so it is a reader's decision,
// taken 2026-09-22 22:26.
//
// One unnamed template with its bindings at the top, per
// .guidelines/styles.md. It lays out lines and never indentation: everything
// indented arrives already indented, from a method that got it right in Go
// where it can be tested. Starlark has no formatter to correct a template's
// own whitespace, and its whitespace is syntax.
//
// The output opens with a blank line. The newline before a def separates one
// function from the next, so the first carries it at the front; Starlark
// ignores it, and trimming it in Go would be the generator compensating for
// the template, which is the coupling Body exists to avoid.
const SOURCE = `
{{- $GEN := .}}
{{- $FUNCTIONS := $GEN.Functions}}
{{- range $FUNCTIONS}}
{{- $NAME := .Name}}
{{- $PARAMS := $GEN.Params .}}
{{- $BODY := $GEN.Body .}}
def {{$NAME}}({{$PARAMS}}):
{{- if $BODY}}
{{$BODY}}
{{- end}}
{{$GEN.Close}}
{{end}}
{{- range $NAME := $GEN.Constants}}
{{$NAME}} = {{$GEN.Bound $NAME}}
{{end}}
{{- range $NAME := $GEN.Arguments}}
{{$NAME}} = {{$GEN.Declared $NAME}}
{{end}}`

// _Gen is what the template calls. Its methods are exported because a template
// reaches nothing else; the type is not, because nothing outside this package
// builds one.
type _Gen struct {
	graph *workflowpb.Graph
}

// Emit is the Starlark a graph's functions define, ready for a compiler.
//
// It does not compile. A caller wanting an artifact passes this to a compiler,
// so a body that is not Starlark is a compile error with a line number rather
// than something this package invents a message for.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func Emit(graph *workflowpb.Graph) ([]byte, error) {
	rendered, err := template.New("starlark").Parse(SOURCE)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}

	var out bytes.Buffer

	err = rendered.Execute(&out, &_Gen{graph: graph})
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return out.Bytes(), nil
}

// Constants is every module-level name a generated script binds, sorted.
//
// Sorted because a map has no order and a generator that emits a different
// file each run is one nobody can diff.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (g *_Gen) Constants() []string {
	var names []string

	for name := range g.graph.GetConstants() {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// Bound is what a constant is bound to, as a script writes it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (g *_Gen) Bound(name string) (string, error) {
	held := g.graph.GetConstants()[name]

	if held.GetCall() != nil {
		return g._Invocation(held.GetCall())
	}

	return _Value(held.GetValue())
}

// Arguments is every argument a generated script declares, sorted by the name
// it binds.
//
// Sorted for the reason Constants is: a map has no order, and a generator that
// emits a different file each run is one nobody can diff.
//
// Revisions:
//   - 2026-09-22 22:44: initial creation
func (g *_Gen) Arguments() []string {
	var names []string

	for name := range g.graph.GetArgs() {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// Declared is the call a script declares an argument with.
//
// An argument with no default is written with none. Writing None there would
// turn an argument a run must supply into one that defaults to nothing, which
// is a different script - and deriving what was emitted would no longer give
// back the graph that was emitted.
//
// Revisions:
//   - 2026-09-22 22:44: initial creation
func (g *_Gen) Declared(name string) (string, error) {
	declared := g.graph.GetArgs()[name]

	supplied := strconv.Quote(declared.GetName())

	if declared.GetDefault() == nil {
		return fmt.Sprintf("%s(%s)", ARG, supplied), nil
	}

	value, err := _Value(declared.GetDefault())
	if err != nil {
		return "", fmt.Errorf("%s: %w", declared.GetName(), err)
	}

	return fmt.Sprintf("%s(%s%s%s)", ARG, supplied, SEPARATOR, value), nil
}

// Functions is every function the graph declares, in the order it declared
// them.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func (g *_Gen) Functions() []*workflowpb.Function {
	return g.graph.GetFunctions()
}

// Params is a function's parameter list as it is written between the brackets.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func (g *_Gen) Params(fn *workflowpb.Function) string {
	return strings.Join(fn.GetParams(), SEPARATOR)
}

// Body is a function's statements, indented one level, ready to sit under a
// def. Empty for a function with none, so the template can leave the line out.
//
// Generated from a thread's steps where a thread runs this function, and the
// authored body otherwise. That is open question 4's answer written as one
// branch: the steps on a thread past its own Call are the calls inside the
// function that Call names, and a function no thread runs is a leaf.
//
// A body on a function a thread does run is **dropped**. The steps describe a
// thread and a body describes a leaf, and a graph sending both has said one
// thing twice in two languages.
//
// A body is statements and not a def, so the indentation is this package's to
// add. A body that already carries its own is indented as given, so a nested if
// keeps its shape - and a body indented inconsistently becomes a Starlark error
// at a position, which is where a syntax question belongs.
//
// A blank line is left blank rather than filled with spaces. Trailing
// whitespace is what a formatter strips, a reader cannot see it, and there is
// no formatter here.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
//   - 2026-09-20 21:02: generates from a thread's steps where one runs this
//     function, and refuses a function with neither steps nor a body
func (g *_Gen) Body(fn *workflowpb.Function) (string, error) {
	if !g.Runs(fn) {
		if strings.TrimSpace(fn.GetBody()) == "" {
			return "", fmt.Errorf("%s: %w", fn.GetName(), ErrNoBody)
		}

		return _Indent(fn.GetBody()), nil
	}

	return g.Steps(fn)
}

// Close is the last line of every generated function.
//
// The template says where a close goes; this says what it looks like, and that
// split is the point. A literal `    pass` in the template would read better -
// a reader would see the whole shape of a def in one place - and it would put
// an indent in the template, which is the one thing the template must not lay
// out.
//
// The failure it avoids is silent. Measured 2026-09-20 21:00: a close that lost
// its four spaces is a top-level pass, which parses and runs. The function
// quietly has no close and nothing says so.
//
// Revisions:
//   - 2026-09-20 21:00: initial creation
func (g *_Gen) Close() string {
	return INDENT + END
}

// _Indent puts one level in front of every line that has anything on it.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
//   - 2026-09-20 21:00: no longer appends the close, which the template now
//     asks for by name
func _Indent(body string) string {
	trimmed := strings.TrimRight(body, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""

			continue
		}

		lines[i] = INDENT + line
	}

	return strings.Join(lines, "\n")
}
