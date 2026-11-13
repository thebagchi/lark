// Regression probes for change C26: lark does not crash the host. A script
// that builds a value that holds itself, or one nested past the limit, gets an
// error from the walk and not a Go stack overflow, which nothing can recover.
//
// The decision (plan.md section 6, Q2): a cycle is an ordinary thread error;
// a value nested past common.DEPTH ends the run the way assert does, and
// retry catches it the way it catches assert; the walk looks at the context
// every common.POLL elements.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// PAST is how deep the probes nest a value: twice the limit.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PAST = 2 * common.DEPTH

	// NOCRASH_LIMIT is how long a probe may take before it is called hung.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	NOCRASH_LIMIT = 30 * time.Second

	// DAG_LEVELS is how many times the cancel probe doubles a list that holds
	// two references to the last one. 2^40 paths no walk finishes.
	//
	// Revisions:
	//   - 2026-11-02 16:21: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DAG_LEVELS = 40
)

// TestNoCrash_ACycleEndsOnlyItsThread proves a self-containing list gives the
// ordinary thread error "cycle in value", after the builtin's name, said once;
// that join reports it; and that it is not the error that ends a run.
//
// Starting condition: no plugins beyond the defaults.
// Success: the run's error is a trail.ErrCyclic and not a trail.ErrFatal.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func TestNoCrash_ACycleEndsOnlyItsThread(t *testing.T) {
	t.Parallel()

	_, err := _Value(
		t,
		`
def searcher():
    x = []
    x.append(x)
    return find_key(x, "k")

def main():
    h = spawn(searcher)
    return join(h)
`,
	)
	if !errors.Is(
		err,
		trail.ErrCyclic,
	) {
		t.Fatalf(
			"got %v, want ErrCyclic",
			err,
		)
	}

	if errors.Is(
		err,
		trail.ErrFatal,
	) {
		t.Fatalf(
			"a cycle must not be fatal: %v",
			err,
		)
	}

	if strings.Count(
		err.Error(),
		"find_key",
	) != 1 || !strings.Contains(
		err.Error(),
		"find_key: cycle in value",
	) {
		t.Fatalf(
			"text %q, want find_key said once, then the cycle",
			err,
		)
	}
}

// TestNoCrash_ACycleInAThreadNobodyJoinsLeavesTheRunRunning proves the script
// carries on when it does not join the thread that met the cycle.
//
// Success: main returns its own value.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_ACycleInAThreadNobodyJoinsLeavesTheRunRunning(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def searcher():
    x = {}
    x["self"] = x
    return find_key(x, "k")

def main():
    h = spawn(searcher)
    sleep(200)
    return "carried on"
`,
	)
	if err != nil {
		t.Fatalf(
			"a cycle in an unjoined thread ended the run: %v",
			err,
		)
	}

	if got.String() != `"carried on"` {
		t.Fatalf(
			"got %s",
			got,
		)
	}
}

// TestNoCrash_ASharedChildIsNotACycle proves find_key accepts a list reached by
// two routes, and a dict as well.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_ASharedChildIsNotACycle(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def main():
    shared = [{"a": 1}]
    both = [shared, shared, {"x": shared}]
    return find_key(both, "a")
`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "1" {
		t.Fatalf(
			"got %s, want 1",
			got,
		)
	}
}

// TestNoCrash_ACycleIsDataToTheStore proves the walks that keep a set of what
// they have seen still accept a value that holds itself: state.set stores it,
// state.get copies it, and patch_json carries it as a value. A cycle is an
// error only where the walk cannot represent it.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_ACycleIsDataToTheStore(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def main():
    x = [1]
    x.append(x)
    state.set("k", x)
    y = state.get("k")
    p = patch_json({}, [{"op": "add", "path": "/a", "value": x}])
    return [len(y), len(p["a"])]
`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "[2, 2]" {
		t.Fatalf(
			"got %s, want [2, 2]",
			got,
		)
	}
}

// _Nest is a script line that binds name to a list nested PAST deep.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Nest(name string) string {
	return fmt.Sprintf(
		"    %[1]s = []\n    for i in range(%[2]d):\n        %[1]s = [%[1]s]\n",
		name,
		PAST,
	)
}

// TestNoCrash_ADeepValueEndsTheRunLikeAssert proves each guarded walk refuses a
// value nested past common.DEPTH with trail.ErrDeep, which wraps trail.ErrFatal,
// and that the text is "value nested too deep" after the builtin's name.
//
// One row for each walk of the research inventory a script reaches without a
// plugin process: find_key, deep.IsData (state.set, event.post), deep.Copy
// (patch_json value), the pointer of patch_json, spawn defaults, and the text
// of json.decode.
//
// Starting condition: PAST is twice common.DEPTH.
// Success: errors.Is(err, trail.ErrDeep) and the text holds the sentence.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestNoCrash_ADeepValueEndsTheRunLikeAssert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "find_key",
			body: _Nest("x") + `    return find_key(x, "k")`,
			want: "find_key: value nested too deep",
		},
		{
			name: "state.set",
			body: _Nest("x") + `    return state.set("k", x)`,
			want: "state.set \"k\": value nested too deep",
		},
		{
			name: "event.post",
			body: _Nest("x") + `    return event.post("k", x)`,
			want: "event.post \"k\": value nested too deep",
		},
		{
			name: "patch_json value",
			body: _Nest("x") + `    op = {"op": "add", "path": "/a", "value": x}
    return patch_json({}, [op])`,
			want: "value nested too deep",
		},
		{
			name: "patch_json pointer",
			body: fmt.Sprintf(
				`    return patch_json({}, [{"op": "remove", "path": "/a" * %d}])`,
				PAST,
			),
			want: "value nested too deep",
		},
		{
			name: "json.decode",
			body: fmt.Sprintf(
				`    return json.decode("[" * %[1]d + "]" * %[1]d)`,
				PAST,
			),
			want: "json.decode: value nested too deep",
		},
		{
			name: "spawn default argument",
			body: _Nest("x") + `    def work(y = x):
        return 1
    return join(spawn(work))`,
			want: "spawn: value nested too deep",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _Value(
					t,
					"def main():\n"+tc.body+"\n",
				)
				if !errors.Is(
					err,
					trail.ErrDeep,
				) {
					t.Fatalf(
						"got %v, want ErrDeep",
						err,
					)
				}

				if !errors.Is(
					err,
					trail.ErrFatal,
				) {
					t.Fatalf(
						"ErrDeep must wrap ErrFatal: %v",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"text %q, want it to hold %q",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// TestNoCrash_AValueAtTheLimitPasses proves the limit is a limit and not a
// refusal of every nested value: common.DEPTH containers pass find_key, and
// the same text passes json.decode.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_AValueAtTheLimitPasses(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		fmt.Sprintf(
			`
def main():
    x = []
    for i in range(%[1]d - 1):
        x = [x]
    text = "[" * %[1]d + "]" * %[1]d
    json.decode(text)
    return find_key(x, "k")
`,
			common.DEPTH,
		),
	)
	if err != nil {
		t.Fatalf(
			"a value at the limit was refused: %v",
			err,
		)
	}

	if got.String() != "None" {
		t.Fatalf(
			"got %s, want None",
			got,
		)
	}
}

// TestNoCrash_BracketsInAStringAreNotNesting proves json.decode counts the
// brackets outside strings: a string of PAST open brackets decodes.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_BracketsInAStringAreNotNesting(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		fmt.Sprintf(
			`
def main():
    text = '["' + "[" * %d + '\\"[", "]"]'
    return len(json.decode(text))
`,
			PAST,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "2" {
		t.Fatalf(
			"got %s, want 2",
			got,
		)
	}
}

// TestNoCrash_ADeepValueInAThreadNobodyJoinsEndsTheRun proves too deep ends the
// run like assert even when no thread joins the one that met it.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestNoCrash_ADeepValueInAThreadNobodyJoinsEndsTheRun(t *testing.T) {
	t.Parallel()

	done := make(
		chan error,
		1,
	)

	go func() {
		_, err := _Value(
			t,
			"def worker():\n"+_Nest("x")+`    return find_key(x, "k")

def main():
    spawn(worker)
    for i in range(100000000):
        sleep(1)
    return "not reached"
`,
		)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(
			err,
			trail.ErrDeep,
		) {
			t.Fatalf(
				"got %v, want ErrDeep",
				err,
			)
		}
	case <-time.After(NOCRASH_LIMIT):
		t.Fatal("the run was not ended by a deep value in an unjoined thread")
	}
}

// TestNoCrash_RetryCatchesADeepValueLikeAssert proves too deep follows assert's
// rule inside a retry: the thread that meets it does not end the run, because
// the attempt is catching.
//
// The attempt spawns a thread that meets a value nested too deep, and does not
// join it. A thread spawned inside an attempt is inside that attempt, so the
// failure stays with the attempt and the run goes on to its own result. The same
// script outside a retry ends the run (the test above this one).
//
// Success: main returns 7.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_RetryCatchesADeepValueLikeAssert(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		"def worker():\n"+_Nest("x")+`    return find_key(x, "k")

def main():
    def attempt():
        spawn(worker)
        sleep(300)
        return 7

    return retry(2, attempt)
`,
	)
	if err != nil {
		t.Fatalf(
			"retry did not catch a deep value like an assert: %v",
			err,
		)
	}

	if got.String() != "7" {
		t.Fatalf(
			"got %s, want 7",
			got,
		)
	}
}

// TestNoCrash_RetryRunsAgainOnADeepValue proves retry re-runs an attempt that
// met a value nested too deep, as it does for an assertion: attempts 1 and 2
// fail, and attempt 3 returns 7.
//
// Revisions:
//   - 2026-11-02 15:51: initial creation (C26 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_RetryRunsAgainOnADeepValue(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		"def main():\n"+_Nest("x")+`    tries = [0]

    def attempt():
        tries[0] += 1
        if tries[0] < 3:
            find_key(x, "k")
        return tries[0]

    return retry(3, attempt)
`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "3" {
		t.Fatalf(
			"got %s, want 3",
			got,
		)
	}
}

// TestNoCrash_RetryThatNeverSucceedsEndsTheRunLikeAssert runs a retry of 3
// whose attempt always fails, once with an assertion (the reference) and once
// with a value nested too deep. Both give up after 3 attempts, and the run ends
// with the cause of the last attempt.
//
// Revisions:
//   - 2026-11-02 15:51: initial creation (C26 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestNoCrash_RetryThatNeverSucceedsEndsTheRunLikeAssert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		fails string
		want  error
	}{
		{
			"assert",
			`assert(False, "no")`,
			core.ErrAssert,
		},
		{
			"too deep",
			`find_key(x, "k")`,
			trail.ErrDeep,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _Value(
					t,
					"def main():\n"+_Nest("x")+`
    def attempt():
        `+tc.fails+`

    return retry(3, attempt)
`,
				)
				if !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.want,
					)
				}

				if !strings.Contains(
					err.Error(),
					"gave up after 3 attempts",
				) {
					t.Fatalf(
						"text %q does not say 3 attempts",
						err,
					)
				}
			},
		)
	}
}

// TestNoCrash_RepeatEndsTheRunAtOnceLikeAssert runs a repeat of 3 whose first
// attempt fails, once with an assertion (the reference) and once with a value
// nested too deep. Both end the run at attempt 1 with their own cause; repeat
// keeps assert's rule and does not run the attempt again.
//
// Revisions:
//   - 2026-11-02 15:51: initial creation (C26 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestNoCrash_RepeatEndsTheRunAtOnceLikeAssert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		fails string
		want  error
	}{
		{
			"assert",
			`assert(False, "no")`,
			core.ErrAssert,
		},
		{
			"too deep",
			`find_key(x, "k")`,
			trail.ErrDeep,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _Value(
					t,
					"def main():\n"+_Nest("x")+`
    def attempt():
        `+tc.fails+`

    return repeat(3, attempt)
`,
				)
				if !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.want,
					)
				}

				if strings.Contains(
					err.Error(),
					"attempt 2",
				) {
					t.Fatalf(
						"repeat ran a second attempt: %v",
						err,
					)
				}

				t.Logf(
					"repeat: %v",
					err,
				)
			},
		)
	}
}

// TestNoCrash_CancelStopsALongWalk proves a cancel reaches a Go walk. The list
// is a DAG of 2^DAG_LEVELS paths, so find_key on it never finishes by itself;
// before the walker looked at the context, cancel(h) could not stop it.
//
// Success: join reports runtime.ErrCancelled, within NOCRASH_LIMIT.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoCrash_CancelStopsALongWalk(t *testing.T) {
	t.Parallel()

	done := make(
		chan error,
		1,
	)

	go func() {
		_, err := _Value(
			t,
			fmt.Sprintf(
				`
def search():
    a = []
    for i in range(%d):
        a = [a, a]
    return find_key(a, "k")

def main():
    h = spawn(search)
    sleep(200)
    cancel(h)
    return join(h)
`,
				DAG_LEVELS,
			),
		)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(
			err,
			scheduler.ErrCancelled,
		) {
			t.Fatalf(
				"got %v, want ErrCancelled",
				err,
			)
		}
	case <-time.After(NOCRASH_LIMIT):
		t.Fatal("cancel did not stop the walk")
	}
}
