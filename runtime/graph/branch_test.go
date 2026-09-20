package graph_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/graph"
)

// _Ask is a condition that calls a function.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func _Ask(name string, args ...*structpb.Value) *workflowpb.Condition {
	return &workflowpb.Condition{
		Kind: &workflowpb.Condition_Call{
			Call: &workflowpb.Call{Function: name, Args: args},
		},
	}
}

// _To is a branch calling a function, with whatever arguments it passes.
//
// Revisions:
//   - 2026-09-20 21:12: initial creation
func _To(name string, args ...*structpb.Value) *workflowpb.Call {
	return &workflowpb.Call{Function: name, Args: args}
}

// TestBranch_AnIfIndentsItsBranches is the first step kind that is a block.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func TestBranch_AnIfIndentsItsBranches(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_If{
		If: &workflowpb.If{Condition: _Ask("ready"), Then: _To("go"), Else: _To("stop")},
	}})

	want := "    if ready():\n        go()\n    else:\n        stop()\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestBranch_AnEmptyBranchIsSkipped records that an empty side is a skip, not
// a call to nothing.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func TestBranch_AnEmptyBranchIsSkipped(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_If{
		If: &workflowpb.If{Condition: _Ask("ready"), Then: _To("go")},
	}})

	if strings.Contains(got, "else") {
		t.Fatalf("want no else at all, got\n%s", got)
	}

	if !strings.Contains(got, "    if ready():\n        go()\n") {
		t.Fatalf("want the taken side, got\n%s", got)
	}
}

// TestBranch_ALiteralConditionIsALiteral records that a condition need not be
// a call.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func TestBranch_ALiteralConditionIsALiteral(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_If{
		If: &workflowpb.If{
			Condition: &workflowpb.Condition{
				Kind: &workflowpb.Condition_Value{Value: true},
			},
			Then: _To("go"),
		},
	}})

	if !strings.Contains(got, "if True:") {
		t.Fatalf("want Starlark's own spelling, got\n%s", got)
	}
}

// TestBranch_AMatchEvaluatesItsExpressionOnce is the decision worth the phase.
//
// A match whose expression is a function would otherwise call it once per case,
// which is a different program - and one that works for a pure function and
// misleads for anything else.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func TestBranch_AMatchEvaluatesItsExpressionOnce(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_Match{
		Match: &workflowpb.Match{
			Expression: &workflowpb.Expression{
				Kind: &workflowpb.Expression_Call{Call: _To("kind")},
			},
			Cases: []*workflowpb.Case{
				{Value: "a", Call: _To("first")},
				{Value: "b", Call: _To("second")},
			},
			Default: _To("other"),
		},
	}})

	if strings.Count(got, "kind()") != 1 {
		t.Fatalf("want the expression evaluated once, got\n%s", got)
	}

	want := "    _match = kind()\n" +
		"    if _match == \"a\":\n        first()\n" +
		"    elif _match == \"b\":\n        second()\n" +
		"    else:\n        other()\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestBranch_TheScriptRuns is the check that a branch is a program.
//
// Revisions:
//   - 2026-09-20 21:07: initial creation
func TestBranch_TheScriptRuns(t *testing.T) {
	out, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{
			_Fn("ready", "return True"),
			_Fn("go", "return 1"),
			_Fn("stop", "return 2"),
			_Fn("main", ""),
		},
		Threads: []*workflowpb.GraphThread{_Spine(&workflowpb.Step{
			Action: &workflowpb.Step_If{
				If: &workflowpb.If{Condition: _Ask("ready"), Then: _To("go"), Else: _To("stop")},
			},
		})},
	})
	if err != nil {
		t.Fatal(err)
	}

	art, err := runtime.NewCompiler().Compile(SCRIPT, out)
	if err != nil {
		t.Fatalf("want a compiling script, got %v\n%s", err, out)
	}

	_, err = art.Run(t.Context())
	if err != nil {
		t.Fatalf("want it to run, got %v\n%s", err, out)
	}
}

// TestBranch_ABranchTakesArgumentsDirectly is sub-phase 1.1 for branches, and
// the one place it differs from a wrapper.
//
// A branch calls; it does not hand a callable to something else. So it passes
// its arguments directly rather than closing over them in a lambda.
//
// Revisions:
//   - 2026-09-20 21:13: initial creation
func TestBranch_ABranchTakesArgumentsDirectly(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_If{
		If: &workflowpb.If{
			Condition: _Ask("ready", structpb.NewStringValue("now")),
			Then:      _To("greet", structpb.NewStringValue("alice")),
		},
	}})

	want := "    if ready(\"now\"):\n        greet(\"alice\")\n"

	if !strings.Contains(got, want) {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}

	if strings.Contains(got, "lambda") {
		t.Fatal("want a branch to call directly, not to build a callable")
	}
}
