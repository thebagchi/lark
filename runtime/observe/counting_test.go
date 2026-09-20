package observe_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	// STARTED and ENDED are the two moments a reporter is told about, and NEVER
	// is neither - a reporter that behaves.
	STARTED = "started"
	ENDED   = "ended"
	NEVER   = ""

	// BLEW_UP is what a faulty reporter raises, so a test can look for it in
	// whatever the run failed with.
	BLEW_UP = "this host's reporter is broken"
)

// _Counter is a Reporter that counts what it was told, per function.
//
// It exists because the polling test cannot see the thing phase 6 forbids. A
// retried attempt that wrongly reports itself ended is only visibly failed
// between that report and the next attempt's start, which is microseconds -
// so a poll almost never lands in it and a broken implementation passes.
// Counting the calls asks the same question without a race in it.
type _Counter struct {
	guard sync.Mutex
	began map[string]int
	ended map[string]int
}

// _NewCounter returns a counter that has been told nothing.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func _NewCounter() *_Counter {
	return &_Counter{
		began: make(map[string]int),
		ended: make(map[string]int),
	}
}

// Started counts a function beginning.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func (c *_Counter) Started(thread string, name string, attempt int32) {
	c.guard.Lock()
	defer c.guard.Unlock()

	c.began[name]++
}

// Ended counts a function finishing.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func (c *_Counter) Ended(thread string, name string, err error) {
	c.guard.Lock()
	defer c.guard.Unlock()

	c.ended[name]++
}

// _Tally is how many times a function began and ended.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func (c *_Counter) _Tally(name string) (int, int) {
	c.guard.Lock()
	defer c.guard.Unlock()

	return c.began[name], c.ended[name]
}

// TestReport_EndsAWrapperOnceHoweverManyAttempts is phase 6's central claim,
// asked without a race: three attempts, one ending.
//
// Reporting every attempt is the natural implementation and the wrong one. A
// caught failure is not the function failing, and a host watching for failures
// would see two that never happened.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_EndsAWrapperOnceHoweverManyAttempts(t *testing.T) {
	counter := _NewCounter()

	built := _Compile(t, RETRYING)

	_, err := built.Run(scheduler.WithReporter(t.Context(), counter))
	if err != nil {
		t.Fatal(err)
	}

	began, ended := counter._Tally("flaky")

	if began != TRIES {
		t.Fatalf("want %d attempts begun, got %d", TRIES, began)
	}

	if ended != 1 {
		t.Fatalf("want one ending for %d attempts, got %d", TRIES, ended)
	}
}

// TestReport_EndsARepeatOnce is the same claim for repeat, which differs only
// in stopping at the first error.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_EndsARepeatOnce(t *testing.T) {
	counter := _NewCounter()

	_, err := _Compile(t, REPEATING).Run(scheduler.WithReporter(t.Context(), counter))
	if err != nil {
		t.Fatal(err)
	}

	began, ended := counter._Tally("step")

	if began != TRIES || ended != 1 {
		t.Fatalf("want %d begun and 1 ended, got %d and %d", TRIES, began, ended)
	}
}

// _Exploding is a reporter with a host's bug in it: it raises on the moment
// chosen, and counts what it was told before that.
type _Exploding struct {
	guard sync.Mutex
	on    string
	told  int
}

// Started raises if this is the moment, and otherwise counts.
//
// Revisions:
//   - 2026-09-20 12:01: initial creation
func (e *_Exploding) Started(thread string, name string, attempt int32) {
	e._Maybe(STARTED)
}

// Ended raises if this is the moment, and otherwise counts.
//
// Revisions:
//   - 2026-09-20 12:01: initial creation
func (e *_Exploding) Ended(thread string, name string, err error) {
	e._Maybe(ENDED)
}

// _Maybe raises when the moment is this one.
//
// Revisions:
//   - 2026-09-20 12:01: initial creation
func (e *_Exploding) _Maybe(moment string) {
	e.guard.Lock()
	defer e.guard.Unlock()

	e.told++

	if e.on == moment {
		panic(BLEW_UP)
	}
}

// TestReport_APanickingReporterEndsTheRun is the decision taken on 2026-09-20
// 12:00, and the property that decision is: a host's callback raising on a
// thread of this runtime's is loud.
//
// It is asked at both moments, because they are guarded by different things. A
// reporter raising while a spawn starts is inside a builtin, caught by the
// evaluation. One raising when a spawned thread ends is on a bare goroutine
// after that evaluation's guard has already returned - the one site that used
// to absorb the panic and let the run report success it had never reported.
//
// Revisions:
//   - 2026-09-20 12:01: initial creation
func TestReport_APanickingReporterEndsTheRun(t *testing.T) {
	for _, moment := range []string{STARTED, ENDED} {
		blows := &_Exploding{on: moment}

		_, err := _Compile(t, BRANCHING).Run(scheduler.WithReporter(t.Context(), blows))
		if err == nil {
			t.Fatalf("%s: want a run whose reporter raised to fail", moment)
		}

		if !strings.Contains(err.Error(), BLEW_UP) {
			t.Fatalf("%s: want the panic's own words, got %v", moment, err)
		}
	}
}

// TestReport_AWorkingReporterDoesNotEndTheRun is the other half: the guard must
// not fail a run whose reporter behaved.
//
// Revisions:
//   - 2026-09-20 12:01: initial creation
func TestReport_AWorkingReporterDoesNotEndTheRun(t *testing.T) {
	quiet := &_Exploding{on: NEVER}

	_, err := _Compile(t, BRANCHING).Run(scheduler.WithReporter(t.Context(), quiet))
	if err != nil {
		t.Fatalf("want a run with a working reporter to succeed, got %v", err)
	}

	if quiet.told == 0 {
		t.Fatal("want the reporter to have been told something")
	}
}
