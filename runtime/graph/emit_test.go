package graph_test

import (
	"errors"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/graph"
)

const (
	// GREET is the worked example throughout .doc/workflow.md.
	GREET = "greet"

	// SCRIPT is what a compiler is handed, so a test names it once.
	SCRIPT = "emitted.star"

	// INDENT_ONCE is the one level a body is allowed to start with.
	INDENT_ONCE = "    "
)

// _Fn builds one authored function.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func _Fn(name string, body string, params ...string) *workflowpb.Function {
	return &workflowpb.Function{Name: name, Params: params, Body: body}
}

// _Emit renders a graph of the functions given.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func _Emit(t *testing.T, fns ...*workflowpb.Function) string {
	t.Helper()

	out, err := graph.Emit(&workflowpb.Graph{Functions: fns})
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

// TestEmit_AFunctionBecomesADef is phase 4: a name, its parameters, and its
// body one level in.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_AFunctionBecomesADef(t *testing.T) {
	got := _Emit(t, _Fn(GREET, `return "hello " + name`, "name"))

	want := "def greet(name):\n    return \"hello \" + name\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want\n%s\ngot\n%s", want, got)
	}
}

// TestEmit_ParamsKeepTheirOrder records that a parameter list is a list.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_ParamsKeepTheirOrder(t *testing.T) {
	got := _Emit(t, _Fn(GREET, "return a", "a", "b", "c"))

	if !strings.Contains(got, "def greet(a, b, c):") {
		t.Fatalf("want a, b, c in order, got\n%s", got)
	}
}

// TestEmit_NoParamsIsEmptyBrackets records that nothing is not a space.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_NoParamsIsEmptyBrackets(t *testing.T) {
	got := _Emit(t, _Fn(GREET, "return 1"))

	if !strings.Contains(got, "def greet():") {
		t.Fatalf("want empty brackets, got\n%s", got)
	}
}

// TestEmit_ABodyKeepsItsOwnShape is why indentation is added rather than
// normalised: a UI that sent a nested block gets that block back.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_ABodyKeepsItsOwnShape(t *testing.T) {
	got := _Emit(t, _Fn(GREET, "if name:\n    return name\n\nreturn \"nobody\"", "name"))

	want := "def greet(name):\n    if name:\n        return name\n\n    return \"nobody\"\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestEmit_ABlankLineStaysBlank records that a blank line is not indented.
//
// Trailing whitespace is what a formatter strips, a reader cannot see it, and
// there is no formatter here.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_ABlankLineStaysBlank(t *testing.T) {
	got := _Emit(t, _Fn(GREET, "a = 1\n\nreturn a"))

	if strings.Contains(got, "    \n") {
		t.Fatalf("want a blank line with nothing on it, got %q", got)
	}
}

// TestEmit_EveryBodyIsClosed records the close on every function, not only an
// empty one.
//
// A block here ends where its indentation does and nothing marks that, so in a
// file of generated defs a reader finds the end of one by looking for the start
// of the next. The close says it. Unreachable after a return, which is fine:
// it is punctuation, not code.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation, as TestEmit_AnEmptyBodyIsPass
//   - 2026-09-20 20:57: every body is closed, so the empty case stopped being
//     the only one worth asserting
func TestEmit_EveryBodyIsClosed(t *testing.T) {
	got := _Emit(t, _Fn(GREET, `return "hello " + name`, "name"))

	if !strings.Contains(got, "    return \"hello \" + name\n    pass\n") {
		t.Fatalf("want the body closed, got\n%q", got)
	}

}

// TestEmit_RefusesAFunctionWithNothingBehindIt records what replaced the empty
// body case.
//
// A function with no body and no thread running it is a name and nothing else.
// It used to render as a def containing only the close; it is refused now,
// because emitting an empty function for it is a guess at what a UI meant.
//
// Revisions:
//   - 2026-09-20 21:03: initial creation
func TestEmit_RefusesAFunctionWithNothingBehindIt(t *testing.T) {
	_, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{_Fn("nothing", "   ")},
	})

	if !errors.Is(err, graph.ErrNoBody) {
		t.Fatalf("want ErrNoBody, got %v", err)
	}

	if !strings.Contains(err.Error(), "nothing") {
		t.Fatalf("want the function named, got %v", err)
	}
}

// TestEmit_TheCloseIsAtTheBodysOwnIndentation records where the close sits: one
// level, like the statements it follows, not at column zero and not deeper.
//
// Deeper would reparent it into whatever block the body ended with, which for a
// body ending in an if would put it inside the branch.
//
// Revisions:
//   - 2026-09-20 20:57: initial creation
func TestEmit_TheCloseIsAtTheBodysOwnIndentation(t *testing.T) {
	got := _Emit(t, _Fn(GREET, "if name:\n    return name", "name"))

	want := "    if name:\n        return name\n    pass\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want the close outside the if, got\n%q", got)
	}
}

// TestEmit_TheTemplateAddsNoIndentationOfItsOwn is the failure the whole
// template rule exists to prevent.
//
// styles.md's conventions were written for Go, where gofmt removes whatever
// layout a template emitted along the way. Starlark has no formatter and its
// whitespace is syntax: two spaces before a def is a compile error.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_TheTemplateAddsNoIndentationOfItsOwn(t *testing.T) {
	got := _Emit(t, _Fn("first", "return 1"), _Fn("second", "return 2"))

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "def ") {
			continue
		}

		if strings.HasPrefix(line, " ") && !strings.HasPrefix(line, INDENT_ONCE) {
			t.Fatalf("want no stray indentation, got %q in\n%q", line, got)
		}
	}

	if !strings.Contains(got, "\ndef second():") {
		t.Fatalf("want the second def at column zero, got\n%q", got)
	}

	// The output opens with a blank line, and that is within the rule rather
	// than against it. The newline before a def is the separator between
	// functions, so the first one carries it at the front. The rule is that the
	// template lays out lines and never indentation; a blank line is a line,
	// and Starlark ignores it.
	//
	// Trimming it in Go would be the generator compensating for the template,
	// which is the coupling Body exists to avoid.
	if strings.TrimLeft(got, "\n") != strings.TrimLeft(got, " \n\t") {
		t.Fatalf("want nothing but newlines before the first def, got %q", got)
	}
}

// TestEmit_TheResultCompiles is the only check that any of this is Starlark
// rather than text shaped like it.
//
// Revisions:
//   - 2026-09-20 20:56: initial creation
func TestEmit_TheResultCompiles(t *testing.T) {
	src := _Emit(t,
		_Fn(GREET, `return "hello " + name`, "name"),
		_Fn("main", `return greet("world")`),
	)

	art, err := runtime.NewCompiler().Compile(SCRIPT, []byte(src))
	if err != nil {
		t.Fatalf("want the emitted source to compile, got %v\n%s", err, src)
	}

	value, err := art.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if value.String() != `"hello world"` {
		t.Fatalf("want the graph's own answer, got %s", value)
	}
}
