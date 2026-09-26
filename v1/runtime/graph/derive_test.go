package graph_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

const (
	// SAMPLES is where the scripts these tests derive live.
	SAMPLES = "../../../samples/"

	// SOURCED is a script written here rather than read, for the shapes no
	// sample happens to contain.
	SOURCED = "sourced.star"

	// LIBRARY is the one sample that defines no entry point.
	LIBRARY = "strings.star"
)

// _Derived is the report a script yields, or a failure.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Derived(t *testing.T, src string) *graph.Report {
	t.Helper()

	report, err := graph.Of([]byte(src), SOURCED, nil)
	if err != nil {
		t.Fatal(err)
	}

	return report
}

// _Sample is the report one of the shipped samples yields.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Sample(t *testing.T, name string) *graph.Report {
	t.Helper()

	src, err := os.ReadFile(SAMPLES + name)
	if err != nil {
		t.Fatal(err)
	}

	report, err := graph.Of(src, SAMPLES+name, nil)
	if err != nil {
		t.Fatal(err)
	}

	return report
}

// _Spun is the names of the steps on a report's spine, in order.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 23:53: skips the first, which names the entry point
func _Spun(report *graph.Report) []string {
	var names []string

	// Past the first, which is what the spine itself runs rather than
	// something it does.
	for _, step := range report.Graph.GetThreads()[0].GetStatic().GetSteps()[graph.BODY_FROM:] {
		names = append(names, _Kind(step))
	}

	return names
}

// TestOf_AnArgumentIsAStepBeforeTheCallItBelongsTo is the phase.
//
// syntax.Walk is pre-order and meets a call before its arguments, so a
// derivation built on it renders record(compute()) as record then compute -
// the reverse of what runs. The POC does exactly that, and its order_test.go
// records the defect; this is that test inverted, so the two cannot both be
// green.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_AnArgumentIsAStepBeforeTheCallItBelongsTo(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def first():",
		"    return 1",
		"",
		"def second():",
		"    return 2",
		"",
		"def main():",
		"    join(spawn(first), spawn(second))",
	}, "\n"))

	want := "spawn spawn join"
	if got := strings.Join(_Spun(report), " "); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestOf_ACallPassingACallKeepsItsBody is the same defect from the other side.
//
// record(compute()) cannot be carried: Call.args are values and a call is not
// one, so the step would render as record() and lose what it was passed. The
// function keeps its text instead - a graph that says less, never one that
// lies.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_ACallPassingACallKeepsItsBody(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def compute():",
		"    return 1",
		"",
		"def record(value):",
		"    return value",
		"",
		"def main():",
		"    record(compute())",
	}, "\n"))

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want the spine to model nothing, got %v", got)
	}

	if !strings.Contains(_Written(report, "main"), "record(compute())") {
		t.Fatalf("want main to carry its own text, got %q", _Written(report, "main"))
	}
}

// TestOf_NamesEveryTopLevelFunction is what a graph carries besides its
// threads.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_NamesEveryTopLevelFunction(t *testing.T) {
	report := _Sample(t, "failfast.star")

	var names []string

	for _, fn := range report.Graph.GetFunctions() {
		names = append(names, fn.GetName())
	}

	want := "counts_a_long_way gives_up main"
	if strings.Join(names, " ") != want {
		t.Fatalf("want %q, got %q", want, strings.Join(names, " "))
	}
}

// TestOf_RefusesAModule records that a file defining no entry point is
// something else loads, not a workflow.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_RefusesAModule(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + "strings.star")
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Of(src, SAMPLES+"strings.star", nil)
	if !errors.Is(err, graph.ErrNoMain) {
		t.Fatalf("want ErrNoMain, got %v", err)
	}

	if !strings.Contains(err.Error(), "strings.star") {
		t.Fatalf("want the file named, got %v", err)
	}
}

// TestOf_ReadsOnlyACallThroughABareName records what a graph can name. A
// method and an element of a list are calls it cannot point at.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_ReadsOnlyACallThroughABareName(t *testing.T) {
	bare := _Derived(t, strings.Join([]string{
		"def helper():",
		"    return 1",
		"",
		"def main():",
		"    helper()",
	}, "\n"))

	if got := strings.Join(_Spun(bare), " "); got != "helper" {
		t.Fatalf("want a bare call to be a step, got %q", got)
	}

	// The same script, with two calls a graph cannot point at. Neither becomes
	// a step, so main no longer models and carries its own text.
	through := _Derived(t, strings.Join([]string{
		"def helper():",
		"    return 1",
		"",
		"def main():",
		`    "a b".split(" ")`,
		"    helper()",
	}, "\n"))

	if got := _Spun(through); len(got) != 0 {
		t.Fatalf("want a method call to leave main unmodelled, got %v", got)
	}

	if !strings.Contains(_Written(through, "main"), "split") {
		t.Fatal("want the method call carried in the body")
	}
}

// TestOf_ASleepIsMilliseconds records the one conversion derivation makes. A
// script writes seconds and the schema counts milliseconds, as it does for
// Repeat, Retry and Timeout.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_ASleepIsMilliseconds(t *testing.T) {
	report := _Derived(t, "def main():\n    sleep(0.05)\n")

	steps := report.Graph.GetThreads()[0].GetStatic().GetSteps()[graph.BODY_FROM:]
	if len(steps) != 1 {
		t.Fatalf("want one step, got %d", len(steps))
	}

	pause := steps[0].GetSleep()
	if pause == nil {
		t.Fatalf("want a sleep, got %v", steps[0])
	}

	if pause.GetDurationMs() != 50 {
		t.Fatalf("want 0.05 seconds as 50ms, got %d", pause.GetDurationMs())
	}
}

// TestOf_AnUnrecognisedCallIsNotAStep records that print and state.set are not
// givings-up. They are calls the workflow has no opinion about, and they live
// in the body a graph carries whole.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_AnUnrecognisedCallIsNotAStep(t *testing.T) {
	report := _Sample(t, "hello.star")

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want no steps for a lone print, got %v", got)
	}

	if len(report.Unknown) != 0 {
		t.Fatalf("want no giving-up recorded, got %v", report.Unknown)
	}

	if report.Graph.GetThreads()[0].GetId() != graph.SPINE {
		t.Fatalf("want the spine, got %s", report.Graph.GetThreads()[0].GetId())
	}
}

// TestOf_TheSpineNamesTheEntryPoint records that the spine says what it runs
// the way every thread does, in its first step.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation, as TestOf_TheSpineNamesNoEntry
//   - 2026-09-21 23:53: reversed: the entry point is the spine's first step
func TestOf_TheSpineNamesTheEntryPoint(t *testing.T) {
	report := _Sample(t, "hello.star")

	steps := report.Graph.GetThreads()[0].GetStatic().GetSteps()
	if len(steps) == 0 {
		t.Fatal("want the spine to say what it runs")
	}

	if steps[0].GetCall().GetFunction() != graph.ENTRY {
		t.Fatalf("want %s first, got %v", graph.ENTRY, steps[0])
	}
}

// _Ids is every thread a report declares, in order.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Ids(report *graph.Report) []string {
	var ids []string

	for _, thread := range report.Graph.GetThreads() {
		ids = append(ids, thread.GetId())
	}

	return ids
}

// _Waited is the threads the nth join on a report's spine names.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Waited(report *graph.Report, nth int) []string {
	seen := 0

	for _, step := range report.Graph.GetThreads()[0].GetStatic().GetSteps() {
		if step.GetJoin() == nil {
			continue
		}

		if seen != nth {
			seen++

			continue
		}

		return step.GetJoin().GetThreads()
	}

	return nil
}

// TestOf_ASpawnForksAndItsFunctionBecomesAThread is the shape of the phase.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_ASpawnForksAndItsFunctionBecomesAThread(t *testing.T) {
	report := _Sample(t, "failfast.star")

	want := "thread_0 thread_1 thread_2"
	if got := strings.Join(_Ids(report), " "); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}

	if got := strings.Join(_Spun(report), " "); got != "spawn spawn join" {
		t.Fatalf("want two spawns and a join, got %q", got)
	}
}

// TestOf_ANestedSpawnIsAChildOfItsSpawner is the id rule, and the reason a
// list position could not carry it.
//
// The spine contributes no prefix, so its children are thread_1 and thread_2.
// alpha's own first child is thread_1_1, not thread_3: an id is a fact about
// structure, and thread_3 would be a fact about the order spawns were met.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_ANestedSpawnIsAChildOfItsSpawner(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def deep():",
		"    return 3",
		"",
		"def alpha():",
		"    join(spawn(deep))",
		"",
		"def beta():",
		"    return 2",
		"",
		"def main():",
		"    join(spawn(alpha), spawn(beta))",
	}, "\n"))

	want := "thread_0 thread_1 thread_1_1 thread_2"
	if got := strings.Join(_Ids(report), " "); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestOf_TwoSpawnsOfOneFunctionAreTwoThreads is the defect a seen-set has.
//
// samples/state.star spawns counted four times. A derivation that remembers
// every function it has read answers "seen it" for the second, and reports
// four threads where seven exist while calling the graph complete.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_TwoSpawnsOfOneFunctionAreTwoThreads(t *testing.T) {
	report := _Sample(t, "state.star")

	if got := len(_Ids(report)); got != 7 {
		t.Fatalf("want seven threads, got %d: %v", got, _Ids(report))
	}

	if got := strings.Join(_Waited(report, 1), " "); got != "thread_3 thread_4 thread_5 thread_6" {
		t.Fatalf("want the second join to name its own four threads, got %q", got)
	}
}

// TestOf_RefusesAFunctionThatSpawnsItself records that a cycle is a
// giving-up, detected on the chain being read rather than on everything seen.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_RefusesAFunctionThatSpawnsItself(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def loops():",
		"    join(spawn(loops))",
		"",
		"def main():",
		"    join(spawn(loops))",
	}, "\n"))

	if len(report.Unknown) != 1 {
		t.Fatalf("want one giving-up, got %v", report.Unknown)
	}

	if !strings.Contains(report.Unknown[0], "loops") {
		t.Fatalf("want the function named, got %v", report.Unknown)
	}
}

// TestOf_AJoinThroughAVariableNamesItsThread is why binding exists: six of
// twenty-two joins in this repository name variables rather than spawns.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_AJoinThroughAVariableNamesItsThread(t *testing.T) {
	report := _Sample(t, "cancel.star")

	if got := strings.Join(_Waited(report, 0), " "); got != "thread_1" {
		t.Fatalf("want the join to follow the handle, got %q", got)
	}
}

// TestOf_AJoinItCannotFollowLeavesItsFunctionUnmodelled records the direction
// the binding fails in. A handle bound inside a branch is beyond it, so the
// function carries its text rather than a join naming the wrong thread.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_AJoinItCannotFollowLeavesItsFunctionUnmodelled(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def slow():",
		"    return 1",
		"",
		"def main():",
		"    h = spawn(slow)",
		"    if True:",
		"        h = spawn(slow)",
		"    join(h)",
	}, "\n"))

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want main unmodelled, got %v", got)
	}

	// The thread the first spawn started is kept: it exists whether or not the
	// body that started it could be described. The one inside the branch is
	// not, because an if is not walked until phase 5 - so the graph is missing
	// a thread rather than wrong about one.
	if got := strings.Join(_Ids(report), " "); got != "thread_0 thread_1" {
		t.Fatalf("want the spawn it read and no other, got %q", got)
	}
}

// TestOf_AStringJoinIsNotThisJoin records that " ".join(parts) is a DotExpr,
// so it never reaches the join a workflow means.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_AStringJoinIsNotThisJoin(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def shout():",
		`    return " ".join(["a", "b"])`,
		"",
		"def main():",
		"    join(spawn(shout))",
	}, "\n"))

	joins := 0

	for _, name := range _Spun(report) {
		if name == graph.JOIN {
			joins++
		}
	}

	if joins != 1 {
		t.Fatalf("want one join step, got %d in %v", joins, _Spun(report))
	}

	if len(_Ids(report)) != 2 {
		t.Fatalf("want the string join to start no thread, got %v", _Ids(report))
	}
}

// TestOf_ASpawnedLambdaIsItsCallWithArguments is what makes a spawned site
// able to carry arguments at all: spawn passes none on, so a generator renders
// a fork with arguments as spawn(lambda: greet("alice")).
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_ASpawnedLambdaIsItsCallWithArguments(t *testing.T) {
	report := _Sample(t, "graph.star")

	entry := report.Graph.GetThreads()[1].GetStatic().GetSteps()[0].GetCall()
	if entry.GetFunction() != "greet" {
		t.Fatalf("want thread_1 to run greet, got %v", entry)
	}

	if len(entry.GetArgs()) != 1 || entry.GetArgs()[0].GetStringValue() != "alice" {
		t.Fatalf("want the lambda's argument carried, got %v", entry.GetArgs())
	}
}

// TestOf_RefusesALambdaOfAnyOtherShape keeps the one accepted spelling one.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func TestOf_RefusesALambdaOfAnyOtherShape(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def main():",
		"    join(spawn(lambda: 1 + 2))",
	}, "\n"))

	if len(report.Unknown) != 1 {
		t.Fatalf("want a giving-up, got %v", report.Unknown)
	}

	if len(_Ids(report)) != 1 {
		t.Fatalf("want only the spine, got %v", _Ids(report))
	}
}

// _Kind is what a step is, as the word a script would write for it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Kind(step *workflowpb.Step) string {
	switch {
	case step.GetFork() != nil:
		return graph.SPAWN

	case step.GetJoin() != nil:
		return graph.JOIN

	case step.GetCancel() != nil:
		return graph.CANCEL

	case step.GetSleep() != nil:
		return graph.SLEEP

	case step.GetRepeat() != nil:
		return graph.REPEAT

	case step.GetRetry() != nil:
		return graph.RETRY

	case step.GetTimeout() != nil:
		return graph.TIMEOUT

	case step.GetIf() != nil:
		return "if"

	case step.GetMatch() != nil:
		return "match"

	case step.GetCall() != nil:
		return step.GetCall().GetFunction()
	}

	return "?"
}

// _Written is the body a report carries for a function, or empty if a thread
// generates it instead.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Written(report *graph.Report, name string) string {
	for _, fn := range report.Graph.GetFunctions() {
		if fn.GetName() == name {
			return fn.GetBody()
		}
	}

	return ""
}

// TestOf_ABodyIsCarriedVerbatim is what a function that does not model keeps.
//
// Sliced from source rather than printed, because go.starlark.net parses and
// does not unparse. Its own nesting survives; the def's level does not, since
// the generator puts that back.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_ABodyIsCarriedVerbatim(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def main():",
		"    total = 0",
		"    for i in range(3):",
		"        total += i",
		"",
		"    print(total)",
	}, "\n"))

	want := strings.Join([]string{
		"total = 0",
		"for i in range(3):",
		"    total += i",
		"",
		"print(total)",
	}, "\n")

	if got := _Written(report, "main"); got != want {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestOf_ABodyIsSlicedByRuneNotByte is the trap a syntax.Position sets.
//
// It carries no byte offset and its column counts runes, so slicing on the
// column as if it were bytes cuts in the wrong place for any script holding a
// character outside ASCII.
//
// The accented character sits on the last line and before its end column,
// which is the only arrangement that bites: one written earlier, or on another
// line, never precedes a column this converts. Planting the byte offset
// against a gentler script passed, which is why this one is shaped like it is.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_ABodyIsSlicedByRuneNotByte(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def main():",
		`    print("ok")`,
		`    print("café")`,
	}, "\n"))

	want := "print(\"ok\")\nprint(\"café\")"

	if got := _Written(report, "main"); got != want {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestOf_CarriesParameters records that a signature travels with a function
// whether or not a thread generates its body.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_CarriesParameters(t *testing.T) {
	report := _Sample(t, "graph.star")

	for _, fn := range report.Graph.GetFunctions() {
		if fn.GetName() != "record" {
			continue
		}

		want := "name age admin tags meta note"
		if got := strings.Join(fn.GetParams(), " "); got != want {
			t.Fatalf("want %q, got %q", want, got)
		}

		return
	}

	t.Fatal("want record among the functions")
}

// TestOf_RefusesASignatureItCannotCarry records the one shape params cannot
// hold. Carrying the bare name would change the program: a call relying on the
// default would raise instead of defaulting - which is what a giving-up used to
// do, emitting def greet() over a body that read who.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as TestOf_GivesUpOnASignatureItCannotCarry
//   - 2026-09-21 08:09: a refusal, since the graph it recorded generated a
//     program the script was not
func TestOf_RefusesASignatureItCannotCarry(t *testing.T) {
	for _, def := range []string{"def main(x = 1):", "def main(*rest):", "def main(**named):"} {
		t.Run(def, func(t *testing.T) {
			_, err := graph.Of([]byte(def+"\n    print(1)\n"), SOURCED, nil)
			if !errors.Is(err, graph.ErrSignature) {
				t.Fatalf("want ErrSignature, got %v", err)
			}

			if !strings.Contains(err.Error(), "main") {
				t.Fatalf("want the function named, got %v", err)
			}
		})
	}
}

// TestOf_ATabIndentedBodyIsDedented records that a body loses whatever its
// def indented it with, not a count of spaces.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestOf_ATabIndentedBodyIsDedented(t *testing.T) {
	report := _Derived(t, "def main():\n\ttotal = 0\n\tfor i in range(2):\n\t\ttotal += i\n\tprint(total)\n")

	want := "total = 0\nfor i in range(2):\n\ttotal += i\nprint(total)"

	if got := _Written(report, "main"); got != want {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestOf_ASleepNeedsANumber records that sleep("1") states no duration, and
// is carried in the body rather than modelled as a sleep of nothing.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestOf_ASleepNeedsANumber(t *testing.T) {
	report := _Derived(t, "def step():\n    return 1\n\ndef main():\n    sleep(\"1\")\n    repeat(True, step)\n")

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want neither statement modelled, got %v", got)
	}
}

// TestOf_CarriesAConstantValue is the bug Graph.constants exists to fix: a
// module-level value was neither a function nor a thread, so derivation
// dropped it and every function reading it broke with undefined.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_CarriesAConstantValue(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		`LIMIT = ["slow", 3, True]`,
		"",
		"def main():",
		"    print(LIMIT)",
	}, "\n"))

	held := report.Graph.GetConstants()["LIMIT"]
	if held.GetValue() == nil {
		t.Fatalf("want LIMIT carried as a value, got %v", report.Graph.GetConstants())
	}

	items := held.GetValue().GetListValue().GetValues()
	if len(items) != 3 || items[0].GetStringValue() != "slow" {
		t.Fatalf("want the value read back, got %v", held)
	}
}

// TestOf_RefusesAConstantDictItCannotOrder is the limit a
// google.protobuf.Struct has. It is a map and a map has no order, while
// Starlark keeps a dict in the order it was written and prints it that way -
// so carrying one of several pairs would give a script that prints something
// the original did not.
//
// samples/patch.star and samples/pointers.star both open with one, and both
// are refused rather than reordered.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_RefusesAConstantDictItCannotOrder(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + "patch.star")
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Of(src, SAMPLES+"patch.star", nil)
	if !errors.Is(err, graph.ErrConstant) {
		t.Fatalf("want ErrConstant, got %v", err)
	}

	if !strings.Contains(err.Error(), "BEFORE") {
		t.Fatalf("want the constant named, got %v", err)
	}

	// One pair has no order to lose, so it is carried.
	held := _Derived(t, "ONE = {\"k\": 1}\n\ndef main():\n    print(ONE)\n")
	if held.Graph.GetConstants()["ONE"].GetValue() == nil {
		t.Fatal("want a single-pair dict carried")
	}
}

// TestOf_CarriesAComputedConstant is the oneof's other arm. Nothing in the
// corpus computes a constant, so this case is invented rather than found -
// which is worth saying, because an untested arm is a claim nobody checked.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_CarriesAComputedConstant(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def build(kind):",
		"    return kind",
		"",
		`LIMIT = build("fast")`,
		"",
		"def main():",
		"    print(LIMIT)",
	}, "\n"))

	held := report.Graph.GetConstants()["LIMIT"]
	if held.GetCall().GetFunction() != "build" {
		t.Fatalf("want a call carried, got %v", held)
	}

	if held.GetCall().GetArgs()[0].GetStringValue() != "fast" {
		t.Fatalf("want its argument carried, got %v", held.GetCall().GetArgs())
	}
}

// TestOf_RefusesAConstantItCannotCarry is where a constant differs from an
// argument. An argument that states no value is a classification and the call
// falls back; a constant has nowhere to ride along, so it is a refusal.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_RefusesAConstantItCannotCarry(t *testing.T) {
	_, err := graph.Of([]byte("LIMIT = 1 + 2\n\ndef main():\n    print(LIMIT)\n"), SOURCED, nil)
	if !errors.Is(err, graph.ErrConstant) {
		t.Fatalf("want ErrConstant, got %v", err)
	}

	if !strings.Contains(err.Error(), "LIMIT") {
		t.Fatalf("want the constant named, got %v", err)
	}
}

// TestOf_TheFlagshipSampleRoundTrips is the whole increment in one assertion.
//
// samples/graph.star carries a graph as a comment and the code generated from
// it. Deriving that code and generating from the result gives the same text
// back, byte for byte - so every step kind and every argument kind survives
// both directions.
//
// It is also the check on pass. The generator closes every def with one, and a
// body sliced out of generated source would carry it as content; emitted
// again, a second would appear under the first. This fails the moment that
// stops being handled.
//
// What it does **not** check is depth. A function that models nothing carries
// its text, and text round-trips too - measured by planting a derivation that
// reads no Match at all, which this test still passed. Fidelity and depth are
// two numbers and this is only the first, which is why
// TestOf_EveryStepKindOnTheSpine stands beside it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_TheFlagshipSampleRoundTrips(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + "graph.star")
	if err != nil {
		t.Fatal(err)
	}

	report, err := graph.Of(src, SAMPLES+"graph.star", nil)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(report.Graph)
	if err != nil {
		t.Fatal(err)
	}

	_, code, found := strings.Cut(string(src), "# Generated from the graph above:")
	if !found {
		t.Fatal("want the sample to carry its generated half")
	}

	if strings.TrimSpace(string(out)) != strings.TrimSpace(code) {
		t.Fatalf("want the sample's own code back\n--- authored\n%s\n--- derived\n%s", code, out)
	}
}

// TestOf_EveryStepKindOnTheSpine records that the flagship models completely,
// in the order it performs them.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_EveryStepKindOnTheSpine(t *testing.T) {
	report := _Sample(t, "graph.star")

	want := "spawn spawn join record repeat retry sleep timeout if match report"
	if got := strings.Join(_Spun(report), " "); got != want {
		t.Fatalf("want\n%q\ngot\n%q", want, got)
	}
}

// TestOf_ABareWrapperIsAStep is the shape a wrapper has to be written in: the
// count or the budget first, the callable last, and nothing taking its result.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_ABareWrapperIsAStep(t *testing.T) {
	report := _Sample(t, "graph.star")

	steps := report.Graph.GetThreads()[0].GetStatic().GetSteps()

	var repeat, retry, timeout *workflowpb.Step

	for _, step := range steps {
		switch {
		case step.GetRepeat() != nil:
			repeat = step
		case step.GetRetry() != nil:
			retry = step
		case step.GetTimeout() != nil:
			timeout = step
		}
	}

	if repeat.GetRepeat().GetCount() != 3 || repeat.GetRepeat().GetCall().GetFunction() != "tick" {
		t.Fatalf("want repeat(3, tick), got %v", repeat)
	}

	if retry.GetRetry().GetAttempts() != 5 || retry.GetRetry().GetCall().GetFunction() != "flaky" {
		t.Fatalf("want retry(5, flaky), got %v", retry)
	}

	// Seconds in the script, milliseconds in the schema.
	if timeout.GetTimeout().GetTimeoutMs() != 2000 {
		t.Fatalf("want timeout(2, ...) as 2000ms, got %v", timeout.GetTimeout())
	}

	// No builtin can spell a delay, so nothing derives one. The emitter
	// refuses a graph that asks for one; this is the symmetric half.
	if repeat.GetRepeat().GetDelayMs() != 0 || retry.GetRetry().GetDelayMs() != 0 {
		t.Fatal("want no delay, which no builtin can spell")
	}
}

// TestOf_AWrapperTakenForItsValueIsNotAStep is why six of nine wrapper calls
// in this repository do not model: no step has a result slot.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_AWrapperTakenForItsValueIsNotAStep(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def counted():",
		"    return 1",
		"",
		"def main():",
		"    last = repeat(3, counted)",
		"    print(last)",
	}, "\n"))

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want main to keep its body, got %v", got)
	}

	if !strings.Contains(_Written(report, "main"), "last = repeat(3, counted)") {
		t.Fatal("want the assignment carried as text")
	}
}

// TestOf_AnIfWithTwoSidesIsAnIf, and one with neither a call on each side is
// not a step at all.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_AnIfWithTwoSidesIsAnIf(t *testing.T) {
	report := _Sample(t, "graph.star")

	for _, step := range report.Graph.GetThreads()[0].GetStatic().GetSteps() {
		branch := step.GetIf()
		if branch == nil {
			continue
		}

		if branch.GetCondition().GetCall().GetFunction() != "both_greeted" {
			t.Fatalf("want the condition carried, got %v", branch.GetCondition())
		}

		if branch.GetThen().GetFunction() != "announce" || branch.GetElse().GetFunction() != "hush" {
			t.Fatalf("want both sides carried, got %v", branch)
		}

		return
	}

	t.Fatal("want an If among the steps")
}

// TestOf_ATwoStatementBranchIsNotAStep records where a branch stops. If.then
// is one Call.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_ATwoStatementBranchIsNotAStep(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def ready():",
		"    return True",
		"",
		"def go():",
		"    return 1",
		"",
		"def main():",
		"    if ready():",
		"        go()",
		"        go()",
	}, "\n"))

	if got := _Spun(report); len(got) != 0 {
		t.Fatalf("want main to keep its body, got %v", got)
	}
}

// TestOf_AMatchIsOneStepNotTwo is the decision worth the phase.
//
// Reading the assignment as a step and the chain as an If would be two steps
// for one decision, and would re-emit as a program that calls its expression
// once per case - which works for a pure function and misleads for anything
// else.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_AMatchIsOneStepNotTwo(t *testing.T) {
	report := _Sample(t, "graph.star")

	for _, step := range report.Graph.GetThreads()[0].GetStatic().GetSteps() {
		match := step.GetMatch()
		if match == nil {
			continue
		}

		if match.GetExpression().GetCall().GetFunction() != "kind" {
			t.Fatalf("want the expression carried, got %v", match.GetExpression())
		}

		if len(match.GetCases()) != 2 {
			t.Fatalf("want two cases, got %d", len(match.GetCases()))
		}

		if match.GetCases()[0].GetValue() != "alpha" || match.GetCases()[1].GetValue() != "beta" {
			t.Fatalf("want the case values carried, got %v", match.GetCases())
		}

		if match.GetDefault().GetFunction() != "on_other" {
			t.Fatalf("want the default carried, got %v", match.GetDefault())
		}

		// And the assignment is not also a step of its own.
		if strings.Count(strings.Join(_Spun(report), " "), "kind") != 0 {
			t.Fatalf("want kind() consumed by the match, got %v", _Spun(report))
		}

		return
	}

	t.Fatal("want a Match among the steps")
}

// TestOf_AnAuthoredChainOverAVariableIsNotAMatch records that the restriction
// falls out of the schema rather than being imposed here: Expression carries a
// string or a Call and never a variable.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_AnAuthoredChainOverAVariableIsNotAMatch(t *testing.T) {
	report := _Derived(t, strings.Join([]string{
		"def fast():",
		"    return 1",
		"",
		"def slow():",
		"    return 2",
		"",
		"def main():",
		`    mode = "fast"`,
		`    if mode == "fast":`,
		"        fast()",
		"    else:",
		"        slow()",
	}, "\n"))

	for _, step := range report.Graph.GetThreads()[0].GetStatic().GetSteps() {
		if step.GetMatch() != nil {
			t.Fatal("want no match over a variable the schema cannot carry")
		}
	}
}

// TestOf_InlinesWhatAScriptLoads is the whole of phase 6: a Graph has
// functions and threads and no module, so a script's modules are inlined or
// the graph cannot replace it.
//
// The whole module comes, not only the names the load mentions, because a
// loaded function may call its own neighbours and a graph with a dangling name
// is not a program.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_InlinesWhatAScriptLoads(t *testing.T) {
	report := _Sample(t, "concurrent.star")

	var names []string

	for _, fn := range report.Graph.GetFunctions() {
		names = append(names, fn.GetName())
	}

	want := "shout join_with first second main"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("want the module inlined first, got %q", got)
	}

	// And the inlined function keeps its own text, sliced from its own file.
	if !strings.Contains(_Written(report, "shout"), "upper()") {
		t.Fatalf("want shout's own body, got %q", _Written(report, "shout"))
	}
}

// TestOf_InlinesADiamondOnce records that a module reached twice by different
// paths is one module, keyed by the path it resolved to.
//
// app.star loads lib.star directly and helper.star, which loads lib.star too.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_InlinesADiamondOnce(t *testing.T) {
	src, err := os.ReadFile("../artifact/testdata/app.star")
	if err != nil {
		t.Fatal(err)
	}

	report, err := graph.Of(src, "../artifact/testdata/app.star", nil)
	if err != nil {
		t.Fatal(err)
	}

	doubles := 0

	for _, fn := range report.Graph.GetFunctions() {
		if fn.GetName() == "double" {
			doubles++
		}
	}

	if doubles != 1 {
		t.Fatalf("want double inlined once, got %d", doubles)
	}
}

// TestOf_RefusesACollisionNamingBothModules is the limit a flat list has, and
// the case that decides the phase.
//
// both.star looks like an aliasing problem and is not. Its closure declares
// side twice and which twice, and the two whiches return different strings.
// The aliases disambiguate only the entry; one level down both side bodies say
// which, and which one they mean depends on the file they sit in - which is
// exactly what a flat list throws away.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_RefusesACollisionNamingBothModules(t *testing.T) {
	// side.star is not an entry, so it is wrapped in one. Loading it alone is
	// fine; loading both is what a flat list cannot hold.
	entry := "load(\"left/side.star\", \"side\")\n\ndef main():\n    side()\n"

	_, err := graph.Of([]byte(entry), "../artifact/testdata/entry.star", nil)
	if err != nil {
		t.Fatalf("want one side inlined cleanly, got %v", err)
	}

	both := "load(\"left/side.star\", \"side\")\nload(\"right/side.star\", \"side\")\n\n" +
		"def main():\n    side()\n"

	_, err = graph.Of([]byte(both), "../artifact/testdata/entry.star", nil)
	if !errors.Is(err, graph.ErrCollision) {
		t.Fatalf("want ErrCollision, got %v", err)
	}

	for _, want := range []string{"left", "right"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want both modules named, got %v", err)
		}
	}
}

// TestOf_RefusesAnAliasedLoad records the second thing derivation will not do,
// for the same reason as the first: reconciling an alias means rewriting
// bodies, and derivation never rewrites a body.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_RefusesAnAliasedLoad(t *testing.T) {
	src, err := os.ReadFile("../artifact/testdata/both.star")
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Of(src, "../artifact/testdata/both.star", nil)
	if !errors.Is(err, graph.ErrAlias) {
		t.Fatalf("want ErrAlias, got %v", err)
	}
}

// TestOf_RefusesAModuleCycle is phase 3's chain check applied to module names.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestOf_RefusesAModuleCycle(t *testing.T) {
	src, err := os.ReadFile("../artifact/testdata/cycle_a.star")
	if err != nil {
		t.Fatal(err)
	}

	report, err := graph.Of(src, "../artifact/testdata/cycle_a.star", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Unknown) == 0 {
		t.Fatal("want the cycle recorded as a giving-up")
	}

	if !strings.Contains(strings.Join(report.Unknown, " "), "cycle") {
		t.Fatalf("want the module named, got %v", report.Unknown)
	}
}

// _Entries is every sample that defines an entry point, which is every one but
// the library.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Entries(t *testing.T) []string {
	t.Helper()

	found, err := filepath.Glob(SAMPLES + "*.star")
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, path := range found {
		name := filepath.Base(path)
		if name == LIBRARY {
			continue
		}

		names = append(names, name)
	}

	return names
}

// _Carried is every sample derivation will carry, which is every entry but the
// two whose module-level dict the schema cannot order.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Carried(t *testing.T) []string {
	t.Helper()

	var names []string

	for _, name := range _Entries(t) {
		if _Uncarried(name) {
			continue
		}

		names = append(names, name)
	}

	return names
}

// _Text is a string as the schema carries one.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Text(held string) *structpb.Value {
	return structpb.NewStringValue(held)
}
