package graph_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

const (
	// PASSING spawns a function that spawns another, handing it its own
	// parameter.
	PASSING = `
def shout(word):
    return word.upper()

def task(word):
    h = spawn(lambda: shout(word))
    join(h)

def main():
    h = spawn(lambda: task("hello"))
    join(h)
`

	// ARGUED hands a thread the argument a run was given.
	ARGUED = `
host = arg("host", "localhost")

def connect(where):
    print("connecting to", where)

def main():
    h = spawn(lambda: connect(host))
    join(h)
`

	// WRAPPED passes a parameter through a retry's lambda. It derived as
	// retry(3, fetch), which a graph then generated and could not run.
	WRAPPED = `
def fetch(url):
    print("fetched", url)

def task(url):
    retry(3, lambda: fetch(url))

def main():
    h = spawn(lambda: task("archive"))
    join(h)
`

	// HANDED passes two handles to a spawned function. It derived as
	// spawn(task_c), which a graph then generated and could not run.
	HANDED = `
def fetch_a():
    return "a"

def fetch_b():
    return "b"

def task_c(ha, hb):
    print(join(ha, hb))

def main():
    ha = spawn(fetch_a)
    hb = spawn(fetch_b)
    hc = spawn(lambda: task_c(ha, hb))
    join(hc)
`
)

// _Of is the thread with this id, failing when the report has none.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func _Of(t *testing.T, report *graph.Report, id string) *workflowpb.Thread {
	t.Helper()

	for _, thread := range report.Graph.GetThreads() {
		if thread.GetId() == id {
			return thread
		}
	}

	t.Fatalf("no %s in %v", id, _Ids(report))

	return nil
}

// _ForkCalling is a fork that carries the call its thread runs.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func _ForkCalling(id string, call *workflowpb.Call) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: id, Func: call}},
	}
}

// _Same fails unless a script and the script its graph generates say the same
// thing when they run.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func _Same(t *testing.T, src string) {
	t.Helper()

	written := _Run(t, SOURCED, []byte(src))
	generated := _Run(t, SOURCED, _Emitted(t, []byte(src), SOURCED))

	if written.failure != "" || *written != *generated {
		t.Fatalf("written said %+v, generated said %+v", *written, *generated)
	}
}

// TestOf_ASpawnPassesAParameter proves a function can hand the thread it spawns
// its own parameter, and that the fork says so as well as the thread.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestOf_ASpawnPassesAParameter(t *testing.T) {
	report := _Derived(t, PASSING)

	want := &workflowpb.Call{
		Function: "shout",
		Args:     []*workflowpb.Parameters{_Parameter("word")},
	}

	if got := _First(_Of(t, report, "thread_1_1")); !proto.Equal(got, want) {
		t.Fatalf("thread_1_1 runs %v, want %v", got, want)
	}

	steps := _Of(t, report, "thread_1").GetStatic().GetSteps()
	if len(steps) < 2 || !proto.Equal(steps[1].GetFork().GetFunc(), want) {
		t.Fatalf("want task's fork to carry %v, got %v", want, steps)
	}

	if len(report.Unknown) > 0 {
		t.Fatalf("want nothing given up, got %v", report.Unknown)
	}

	if err := graph.Check(report.Graph); err != nil {
		t.Fatalf("check: %v", err)
	}

	_Same(t, PASSING)
}

// TestOf_ARunArgumentIsAParameter proves a thread can be handed what a run was
// given, rather than only a value written into the graph.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestOf_ARunArgumentIsAParameter(t *testing.T) {
	report := _Derived(t, ARGUED)

	got := _First(_Of(t, report, "thread_1"))
	if len(got.GetArgs()) != 1 || got.GetArgs()[0].GetParameter() != "host" {
		t.Fatalf("want connect(host), got %v", got)
	}

	if err := graph.Check(report.Graph); err != nil {
		t.Fatalf("check: %v", err)
	}

	_Same(t, ARGUED)
}

// TestOf_AWrappedLambdaPassesAParameter proves a retry's lambda keeps the
// parameter it passes, where it used to derive with the argument gone.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestOf_AWrappedLambdaPassesAParameter(t *testing.T) {
	report := _Derived(t, WRAPPED)

	steps := _Of(t, report, "thread_1").GetStatic().GetSteps()
	if len(steps) < 2 || steps[1].GetRetry().GetCall().GetArgs()[0].GetParameter() != "url" {
		t.Fatalf("want task's retry to pass url, got %v", steps)
	}

	_Same(t, WRAPPED)
}

// TestOf_ASpawnPassingAHandleIsReported proves an argument a graph cannot carry
// is said rather than dropped, and that the function passing it keeps its text,
// so the graph still runs.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestOf_ASpawnPassingAHandleIsReported(t *testing.T) {
	report := _Derived(t, HANDED)

	if !slices.Contains(report.Unknown, "task_c: "+graph.UNCARRIED) {
		t.Fatalf("want task_c reported, got %v", report.Unknown)
	}

	if steps := _Of(t, report, graph.SPINE).GetStatic().GetSteps(); len(steps) != 1 {
		t.Fatalf("want main kept as its text, got %d steps", len(steps))
	}

	_Same(t, HANDED)
}

// TestEmit_WritesAParameterAsItsName proves a parameter is generated as the
// name it holds, and a constant it names is one the script binds.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestEmit_WritesAParameterAsItsName(t *testing.T) {
	call := &workflowpb.Call{
		Function: "greet",
		Args:     []*workflowpb.Parameters{_Parameter("GREETING")},
	}

	built := _Authored(
		[]*workflowpb.Step{_ForkCalling("thread_1", call), _JoinOf("thread_1")},
		&workflowpb.Thread{Id: "thread_1", State: &workflowpb.Thread_Static{
			Static: &workflowpb.Static{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: proto.CloneOf(call)}},
			}},
		}},
	)
	built.Functions = []*workflowpb.Function{
		_Fn("greet", `print("hello " + who)`, "who"),
		_Fn(graph.ENTRY, ""),
	}
	built.Constants = map[string]*workflowpb.Constant{
		"GREETING": {Kind: &workflowpb.Constant_Value{Value: _Text("bob")}},
	}

	if err := graph.Check(built); err != nil {
		t.Fatalf("check: %v", err)
	}

	out, err := graph.Emit(built)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "h1 = spawn(lambda: greet(GREETING))") {
		t.Fatalf("want the name passed, got\n%s", out)
	}
}

// TestEmit_GeneratesAForkFromItsOwnCall proves the fork's call is the one
// generated, where a fork carries one. Check refuses a graph whose two copies
// differ; generating is not checking, so this graph shows which one wins.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestEmit_GeneratesAForkFromItsOwnCall(t *testing.T) {
	call := &workflowpb.Call{
		Function: "greet",
		Args:     []*workflowpb.Parameters{_Valued(_Text("alice"))},
	}

	built := _Authored(
		[]*workflowpb.Step{_ForkCalling("thread_1", call), _JoinOf("thread_1")},
		_Runs("thread_1", "greet"),
	)
	built.Functions = []*workflowpb.Function{
		_Fn("greet", `print("hello " + who)`, "who"),
		_Fn(graph.ENTRY, ""),
	}

	out, err := graph.Emit(built)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `spawn(lambda: greet("alice"))`) {
		t.Fatalf("want the fork's own call, got\n%s", out)
	}
}

// TestCheck_RefusesAThreadNamedWhereItWasNotForked proves a join or a cancel
// names only threads forked earlier on the thread holding it, since those are
// the only handles a generated script has in reach.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_RefusesAThreadNamedWhereItWasNotForked(t *testing.T) {
	cancelled := &workflowpb.Step{
		Action: &workflowpb.Step_Cancel{
			Cancel: &workflowpb.Cancel{Threads: []string{"thread_1"}},
		},
	}

	cases := []struct {
		name  string
		spine []*workflowpb.Step
	}{
		{name: "joined with no fork", spine: []*workflowpb.Step{_JoinOf("thread_1")}},
		{name: "joined before its fork", spine: []*workflowpb.Step{
			_JoinOf("thread_1"),
			_Fork("thread_1"),
		}},
		{name: "cancelled with no fork", spine: []*workflowpb.Step{cancelled}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			err := graph.Check(_Authored(item.spine, _Runs("thread_1", "worker")))
			if !errors.Is(err, graph.ErrNotForked) {
				t.Fatalf("got %v, want ErrNotForked", err)
			}
		})
	}
}

// TestCheck_RefusesAForkWhoseCallIsNotItsThreads keeps the two copies of one
// call in step.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_RefusesAForkWhoseCallIsNotItsThreads(t *testing.T) {
	forked := &workflowpb.Call{
		Function: "greet",
		Args:     []*workflowpb.Parameters{_Valued(_Text("alice"))},
	}

	built := _Authored(
		[]*workflowpb.Step{_ForkCalling("thread_1", forked), _JoinOf("thread_1")},
		_Lane("thread_1", "greet", _Valued(_Text("bob"))),
	)

	if err := graph.Check(built); !errors.Is(err, graph.ErrForkCall) {
		t.Fatalf("got %v, want ErrForkCall", err)
	}
}

// TestCheck_TakesAForkWithoutACall keeps every graph written before forks
// carried their call a graph.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_TakesAForkWithoutACall(t *testing.T) {
	built := _Authored(
		[]*workflowpb.Step{_Fork("thread_1"), _JoinOf("thread_1")},
		_Runs("thread_1", "worker"),
	)

	if err := graph.Check(built); err != nil {
		t.Fatalf("got %v, want it taken", err)
	}
}

// TestCheck_ResolvesAParameterWhereItsCallIsMade proves a parameter is looked
// up where the call is written: a forked thread's first step is its parent's
// call, so the parent's params are what it sees.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_ResolvesAParameterWhereItsCallIsMade(t *testing.T) {
	task := &workflowpb.Call{
		Function: "task",
		Args:     []*workflowpb.Parameters{_Valued(_Text("hello"))},
	}
	shout := &workflowpb.Call{
		Function: "shout",
		Args:     []*workflowpb.Parameters{_Parameter("word")},
	}

	built := _Authored(
		[]*workflowpb.Step{_ForkCalling("thread_1", task), _JoinOf("thread_1")},
		&workflowpb.Thread{Id: "thread_1", State: &workflowpb.Thread_Static{
			Static: &workflowpb.Static{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: proto.CloneOf(task)}},
				_ForkCalling("thread_1_1", shout),
				_JoinOf("thread_1_1"),
			}},
		}},
		&workflowpb.Thread{Id: "thread_1_1", State: &workflowpb.Thread_Static{
			Static: &workflowpb.Static{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: proto.CloneOf(shout)}},
			}},
		}},
	)
	built.Functions = []*workflowpb.Function{
		_Fn("shout", "return word.upper()", "word"),
		_Fn("task", "", "word"),
		_Fn(graph.ENTRY, ""),
	}

	if err := graph.Check(built); err != nil {
		t.Fatalf("got %v, want task's word in reach of shout", err)
	}

	built.Functions[1].Params = []string{"phrase"}

	if err := graph.Check(built); !errors.Is(err, graph.ErrUnresolved) {
		t.Fatalf("got %v, want ErrUnresolved once task has no word", err)
	}
}

// TestCheck_RefusesAParameterInAConstant keeps a constant computed from values:
// it is bound before anything a name could reach.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_RefusesAParameterInAConstant(t *testing.T) {
	built := _Authored(nil)
	built.Functions = []*workflowpb.Function{
		_Fn("build", "return 1", "size"),
		_Fn(graph.ENTRY, ""),
	}
	built.Constants = map[string]*workflowpb.Constant{
		"LIMIT": {Kind: &workflowpb.Constant_Call{Call: &workflowpb.Call{
			Function: "build",
			Args:     []*workflowpb.Parameters{_Parameter("build")},
		}}},
	}

	if err := graph.Check(built); !errors.Is(err, graph.ErrUnresolved) {
		t.Fatalf("got %v, want ErrUnresolved", err)
	}
}

// TestCheck_RefusesAnArgumentWithNeitherKind proves an argument holding
// nothing is refused rather than generated as nothing.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func TestCheck_RefusesAnArgumentWithNeitherKind(t *testing.T) {
	built := _Authored([]*workflowpb.Step{
		_CallOf("greet", &workflowpb.Parameters{}),
	})
	built.Functions = []*workflowpb.Function{_Fn("greet", "", "who"), _Fn(graph.ENTRY, "")}

	if err := graph.Check(built); !errors.Is(err, graph.ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue", err)
	}
}

// _First is the call a thread runs, as its first step says.
//
// Revisions:
//   - 2026-09-30 00:50: initial creation
func _First(thread *workflowpb.Thread) *workflowpb.Call {
	return thread.GetStatic().GetSteps()[0].GetCall()
}
