package graph_test

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

// _Body renders a spine holding the steps given and returns just its body.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation
func _Body(t *testing.T, steps ...*workflowpb.Step) string {
	t.Helper()

	out, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{_Fn("main", "")},
		Threads:   []*workflowpb.Thread{_Spine(steps...)},
	})
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

// TestBounded_TheWrappersGenerateTheirLines is phase 6's five renderings.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation
func TestBounded_TheWrappersGenerateTheirLines(t *testing.T) {
	cases := []struct {
		step *workflowpb.Step
		want string
	}{
		{
			step: &workflowpb.Step{Action: &workflowpb.Step_Repeat{
				Repeat: &workflowpb.Repeat{
					Call:  &workflowpb.Call{Function: "greet"},
					Count: 3,
				},
			}},
			want: "repeat(3, greet)",
		},
		{
			step: &workflowpb.Step{Action: &workflowpb.Step_Retry{
				Retry: &workflowpb.Retry{
					Call:     &workflowpb.Call{Function: "fetch"},
					Attempts: 5,
				},
			}},
			want: "retry(5, fetch)",
		},
		{
			step: &workflowpb.Step{Action: &workflowpb.Step_Timeout{
				Timeout: &workflowpb.Timeout{
					Call:      &workflowpb.Call{Function: "slow"},
					TimeoutMs: 2000,
				},
			}},
			want: "timeout(2, slow)",
		},
		{
			step: _Sleep(0.05),
			want: "sleep(0.05)",
		},
	}

	for _, item := range cases {
		t.Run(item.want, func(t *testing.T) {
			got := _Body(t, item.step)

			if !strings.Contains(got, INDENT_ONCE+item.want+"\n") {
				t.Fatalf("want %q, got\n%s", item.want, got)
			}
		})
	}
}

// TestBounded_ThereIsNoTrailingCall records the shape that went.
//
// A wrapper used to give back a callable, so a generated line ended in an extra
// pair of brackets. Nothing built one and called it twice, nothing passed one
// anywhere, and composing them did not work - retry(3, timeout(5, fn)) was
// refused, because a wrapper returned a builtin where retry wanted a function.
// So the wrappers call straight away, and the brackets are gone.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation, as TestBounded_TheTrailingCallIsThere
//   - 2026-09-20 21:15: inverted; the factory shape was removed
func TestBounded_ThereIsNoTrailingCall(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_Repeat{
		Repeat: &workflowpb.Repeat{Call: &workflowpb.Call{Function: "greet"}, Count: 3},
	}})

	if strings.Contains(got, "repeat(3, greet)()") {
		t.Fatalf("want no trailing call, got\n%s", got)
	}

	if !strings.Contains(got, INDENT_ONCE+"repeat(3, greet)\n") {
		t.Fatalf("want the wrapper called by being written, got\n%s", got)
	}
}

// TestBounded_RefusesADelay records the schema being wider than the language.
//
// Repeat and Retry declare delay_ms and the builtins take none. Generating the
// call without it produces a script that runs, gives the right answer, and
// hammers whatever it talks to.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation
func TestBounded_RefusesADelay(t *testing.T) {
	_, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{_Fn("main", "")},
		Threads: []*workflowpb.Thread{_Spine(&workflowpb.Step{
			Action: &workflowpb.Step_Repeat{
				Repeat: &workflowpb.Repeat{
					Call:    &workflowpb.Call{Function: "greet"},
					Count:   3,
					DelayMs: 500,
				},
			},
		})},
	})

	if !errors.Is(err, graph.ErrNoDelay) {
		t.Fatalf("want ErrNoDelay, got %v", err)
	}

	for _, want := range []string{"repeat", "greet", "500"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want %q named in the refusal, got %v", want, err)
		}
	}
}

// TestBounded_AWholeNumberOfSecondsIsAnInteger records the rule a JSON argument
// already follows: Starlark has two number types where the schema has one, and
// they differ under // and %.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation
//   - 2026-09-21 00:59: a sleep carries seconds, so this is the value rule
//     rather than a conversion
func TestBounded_AWholeNumberOfSecondsIsAnInteger(t *testing.T) {
	got := _Body(t, _Sleep(2))

	if !strings.Contains(got, "sleep(2)") {
		t.Fatalf("want sleep(2), got\n%s", got)
	}

	if strings.Contains(got, "sleep(2.0)") {
		t.Fatal("want an integer, not a float")
	}
}

// TestBounded_TheScriptRuns is the check that these lines are a program.
//
// Revisions:
//   - 2026-09-20 21:06: initial creation
func TestBounded_TheScriptRuns(t *testing.T) {
	out, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{
			_Fn("step", "return 1"),
			_Fn("main", ""),
		},
		Threads: []*workflowpb.Thread{_Spine(
			&workflowpb.Step{Action: &workflowpb.Step_Repeat{
				Repeat: &workflowpb.Repeat{
					Call:  &workflowpb.Call{Function: "step"},
					Count: 3,
				},
			}},
			_Sleep(0.01),
		)},
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

// TestBounded_AWrappedSiteTakesArguments is what sub-phase 1.1 bought.
//
// Before it, a Repeat named a function as a string with no Call to hang
// arguments on, so `repeat(3, greet("alice"))` could not be authored and the
// workaround was a helper per argument list — three helpers for one idea, and
// three names in a report.
//
// Revisions:
//   - 2026-09-20 21:13: initial creation
func TestBounded_AWrappedSiteTakesArguments(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_Repeat{
		Repeat: &workflowpb.Repeat{
			Call: &workflowpb.Call{Function: "greet", Args: []*structpb.Value{
				structpb.NewStringValue("alice"),
			}},
			Count: 3,
		},
	}})

	if !strings.Contains(got, `repeat(3, lambda: greet("alice"))`) {
		t.Fatalf("want a wrapped site with arguments, got\n%s", got)
	}
}

// TestBounded_AWrappedSiteWithoutArgumentsIsABareName records the other half:
// the lambda appears only where a call passes arguments.
//
// Revisions:
//   - 2026-09-20 21:13: initial creation
func TestBounded_AWrappedSiteWithoutArgumentsIsABareName(t *testing.T) {
	got := _Body(t, &workflowpb.Step{Action: &workflowpb.Step_Repeat{
		Repeat: &workflowpb.Repeat{Call: &workflowpb.Call{Function: "greet"}, Count: 3},
	}})

	if strings.Contains(got, "lambda") {
		t.Fatalf("want no lambda where there are no arguments, got\n%s", got)
	}
}

// _Sleep is a pause written in the seconds a script says, in the milliseconds
// the schema counts.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Sleep(seconds float64) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Sleep{
			Sleep: &workflowpb.Sleep{DurationMs: int32(seconds * 1000)},
		},
	}
}
