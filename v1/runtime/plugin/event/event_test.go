// Proofs for event.post and event.wait: the orderings, the timeout, the charge,
// and what a script can tell apart.
package event_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin/event"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// SCRIPT is what a failure calls the script it was given.
	SCRIPT = "event.star"

	// _DEADLINE is how long a script gets before it is taken to have hung. A
	// regression in a wait does not fail, it blocks, so every test here runs
	// under this.
	_DEADLINE = 20 * time.Second

	// _NARROW is a ceiling that one small event fits inside, and that neither
	// one event of 128KB nor twenty thousand named ones do.
	_NARROW = 64 << 10
)

// _Ran compiles and runs src with the event names present, under a deadline.
//
// Deadlined because a defect in a wait does not fail: it blocks, and a suite
// that has to be killed says nothing about which test was wrong.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func _Ran(t *testing.T, src string) (string, error) {
	t.Helper()

	built, err := runtime.Compile(&runtime.Source{Entry: SCRIPT, Text: []byte(src)})
	if err != nil {
		return "", err
	}

	type outcome struct {
		got string
		err error
	}

	answered := make(chan outcome, 1)

	go func() {
		value, err := runtime.Start(context.Background(), built).Wait()
		if err != nil {
			answered <- outcome{err: err}

			return
		}

		answered <- outcome{got: value.String()}
	}()

	select {
	case held := <-answered:
		return held.got, held.err

	case <-time.After(_DEADLINE):
		t.Fatalf("the script did not finish within %v: a wait is not returning", _DEADLINE)
	}

	return "", nil
}

// TestEvent_OneThreadWaitsForAnother is the whole point, in the order that
// matters least: the waiter is already waiting when the post arrives.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_OneThreadWaitsForAnother(t *testing.T) {
	got, err := _Ran(t, `
def waiter():
    value, err = event.wait("loaded", 10000)

    return [value, err]

def main():
    held = spawn(waiter)

    sleep(50)
    event.post("loaded", {"rows": 12})

    return join(held)[0]
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got != `[{"rows": 12}, None]` {
		t.Fatalf("got %s", got)
	}
}

// TestEvent_APostBeforeAnyoneWaitsIsStillSeen is the ordering a transient signal
// would lose, and it is the reason this is a latch.
//
// The poster runs first and finishes. A signal that only woke whoever was
// waiting at the time would leave the waiter here waiting for something that had
// already happened - a deadlock that depends on scheduling, which is the worst
// kind to debug.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_APostBeforeAnyoneWaitsIsStillSeen(t *testing.T) {
	got, err := _Ran(t, `
def main():
    event.post("ready", "done")

    sleep(50)

    value, err = event.wait("ready", 10000)

    return [value, err]
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got != `["done", None]` {
		t.Fatalf("got %s", got)
	}
}

// TestEvent_EveryWaiterSeesIt is the other half of a latch: one post, three
// threads, all released.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_EveryWaiterSeesIt(t *testing.T) {
	got, err := _Ran(t, `
def waiter():
    value, err = event.wait("go", 10000)

    return value

def main():
    first = spawn(waiter)
    second = spawn(waiter)
    third = spawn(waiter)

    sleep(50)
    event.post("go", 7)

    return [join(first)[0], join(second)[0], join(third)[0]]
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got != "[7, 7, 7]" {
		t.Fatalf("got %s", got)
	}
}

// TestEvent_ARunOutWaitSaysSo is the timeout, and the reason the answer is a
// pair.
//
// An event posted as None and a wait that expired would be the same answer if a
// wait returned only the value. The second half is what tells them apart.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_ARunOutWaitSaysSo(t *testing.T) {
	got, err := _Ran(t, `
def main():
    value, err = event.wait("never", 50)

    return [value, err]
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got != `[None, "timed out"]` {
		t.Fatalf("got %s", got)
	}

	// And the pair is what tells a posted None from a wait that ran out.
	got, err = _Ran(t, `
def main():
    event.post("empty", None)

    value, err = event.wait("empty", 10000)

    return [value, err]
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got != "[None, None]" {
		t.Fatalf("a posted None gave %s, which a timeout must not match", got)
	}
}

// TestEvent_ATimeoutAroundAWaitCutsIt is the debt every blocking builtin owes,
// and the one I have already failed to pay once elsewhere.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_ATimeoutAroundAWaitCutsIt(t *testing.T) {
	_, err := _Ran(t, `
def patient():
    return event.wait("never", 300000)

def main():
    return timeout(200, patient)
`)
	if err == nil {
		t.Fatal("a timeout around a three-hundred-second wait returned nothing")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("got %v, want the outer timeout", err)
	}
}

// TestEvent_PostingTwiceIsRefused records that a latch says one thing happened.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_PostingTwiceIsRefused(t *testing.T) {
	_, err := _Ran(t, `
def main():
    event.post("once", 1)
    event.post("once", 2)

    return "reached"
`)
	if !errors.Is(err, event.ERR_POSTED) {
		t.Fatalf("got %v, want ERR_POSTED", err)
	}
}

// TestEvent_WhatCannotCrossIsRefused is the rule the store already has, for the
// same reason: another thread reads this.
//
// The function comes back from a call, so the source does not show it and this
// is the refusal made when the post runs. One the source shows is refused
// before the run, which is the next test.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
//   - 2026-09-30 22:41: the function comes back from a call, so the run is what
//     refuses it
func TestEvent_WhatCannotCrossIsRefused(t *testing.T) {
	_, err := _Ran(t, `
def helper():
    return 1

def pick():
    return helper

def main():
    event.post("code", pick())

    return "reached"
`)
	if !errors.Is(err, event.ERR_NOT_DATA) {
		t.Fatalf("got %v, want ERR_NOT_DATA", err)
	}
}

// TestEvent_AVisibleFunctionIsRefusedBeforeItRuns is the half the source can
// see, as it is for the store.
//
// A name this file declares, or a lambda written in place, is certain before
// anything runs - and an author would rather hear it then than from a run that
// stops. The compiler does not know what post means: it asks every plugin
// whether the source is acceptable, and this one answers.
//
// Revisions:
//   - 2026-09-30 22:41: initial creation
func TestEvent_AVisibleFunctionIsRefusedBeforeItRuns(t *testing.T) {
	cases := map[string]string{
		"a declared function": "def helper():\n    return 1\n\ndef main():\n" +
			"    event.post(\"k\", helper)\n",
		"a lambda": "def main():\n    event.post(\"k\", lambda: 1)\n",
		"in parentheses": "def helper():\n    return 1\n\ndef main():\n" +
			"    event.post(\"k\", (helper))\n",
		"and nested ones": "def helper():\n    return 1\n\ndef main():\n" +
			"    event.post(\"k\", ((helper)))\n",
	}

	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.Compile(
				&runtime.Source{Entry: name + ".star", Text: []byte(script)},
			)
			if !errors.Is(err, event.ERR_NOT_DATA) {
				t.Fatalf("compiling gave %v, want ERR_NOT_DATA before it ran", err)
			}
		})
	}
}

// TestEvent_ARefusedPostStopsTheWholeRun is why a refused post goes through the
// door a failed assertion does, as a refused store does.
//
// A thread nobody joins fails silently: its error reaches the report and never
// becomes the run's result. So a spawned worker posting twice, or posting a
// function, would say nothing about it, and the script would finish as though
// it had worked. main here outlives the worker, and returns "run survived" only
// if the run was left standing. The function comes back from a call, so it is
// the run that refuses it rather than the compile.
//
// Revisions:
//   - 2026-09-30 21:55: initial creation
//   - 2026-09-30 22:41: the function comes back from a call, so the refusal is
//     the run's
func TestEvent_ARefusedPostStopsTheWholeRun(t *testing.T) {
	got, err := _Ran(t, `
def poster():
    event.post("once", 1)
    event.post("once", 2)

def main():
    unjoined = spawn(poster)

    sleep(5000)

    return "run survived"
`)
	if !errors.Is(err, event.ERR_POSTED) {
		t.Fatalf("a second post nobody joins: got %s, %v; want ERR_POSTED", got, err)
	}

	got, err = _Ran(t, `
def helper():
    return 1

def pick():
    return helper

def poster():
    event.post("code", pick())

def main():
    unjoined = spawn(poster)

    sleep(5000)

    return "run survived"
`)
	if !errors.Is(err, event.ERR_NOT_DATA) {
		t.Fatalf("a function posted nobody joins: got %s, %v; want ERR_NOT_DATA", got, err)
	}
}

// TestEvent_WaitingIsCancelledWithTheRun keeps a forgotten wait from holding a
// run open.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_WaitingIsCancelledWithTheRun(t *testing.T) {
	built, err := runtime.Compile(
		&runtime.Source{Entry: SCRIPT, Text: []byte(`
def main():
    value, err = event.wait("never", 300000)

    return err
`)},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		stop()
	}()

	answered := make(chan error, 1)

	go func() {
		_, err := runtime.Start(ctx, built).Wait()
		answered <- err
	}()

	select {
	case err := <-answered:
		if !errors.Is(err, scheduler.ERR_CANCELLED) {
			t.Fatalf("got %v, want ERR_CANCELLED", err)
		}

	case <-time.After(_DEADLINE):
		t.Fatalf("a cancelled run did not end within %v", _DEADLINE)
	}
}

// TestEvent_ManyWaitersBothSidesOfThePost is the case a closed channel is
// chosen for.
//
// Sixteen threads wait before the post and sixteen ask after it. A broadcast
// that only woke whoever was waiting at the time would release the first
// sixteen and hang the rest; one that only answered afterwards would hang the
// first. A closed channel does both, because it is readable forever and by any
// number of receivers.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_ManyWaitersBothSidesOfThePost(t *testing.T) {
	got, err := _Ran(t, `
def waiter():
    value, err = event.wait("go", 10000)

    if err:
        return -1

    return value

def main():
    before = []

    for i in range(16):
        held = spawn(waiter)
        before.append(held)

    sleep(50)
    event.post("go", 5)

    after = []

    for i in range(16):
        held = spawn(waiter)
        after.append(held)

    total = 0

    for held in before:
        total += join(held)[0]

    for held in after:
        total += join(held)[0]

    return total
`)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Thirty-two waiters, five each, and none of them -1.
	if got != "160" {
		t.Fatalf("got %s, want 160: some waiter missed the post", got)
	}
}

// TestEvent_AnEventIsChargedForWhatItHolds is the rule the store already has,
// for the reason the store has it: there is no delete, so a posted event is held
// until the run ends.
//
// One event each time, under one ceiling, with only the value's size differing.
// Naming an event is charged as well, so a test posting many would be refused
// for the names whether or not the values were charged - it has to be one event,
// small enough that its name cannot be what refuses it.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_AnEventIsChargedForWhatItHolds(t *testing.T) {
	_, err := _Under(t, _NARROW, `
def main():
    event.post("small", "x" * 1024)

    return "posted"
`)
	if err != nil {
		t.Fatalf("one event of 1KB under %d bytes: %v", _NARROW, err)
	}

	_, err = _Under(t, _NARROW, `
def main():
    event.post("large", "x" * (128 * 1024))

    return "posted"
`)
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf("one event of 128KB under %d bytes: %v, want ERR_MEMORY", _NARROW, err)
	}
}

// TestEvent_NamingOneIsChargedToo covers the side that allocates without
// posting.
//
// A waiter names an event nobody has posted, which is how a waiter arrives
// first - and is also how a script could make a million channels without ever
// posting anything. Charged where the event is made rather than where it is
// posted, so both sides pay.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func TestEvent_NamingOneIsChargedToo(t *testing.T) {
	_, err := _Under(t, _NARROW, `
def main():
    for i in range(20000):
        value, err = event.wait("never%d" % i, 0)

    return "waited"
`)
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf(
			"twenty thousand named events under %d bytes: %v, want ERR_MEMORY",
			_NARROW,
			err,
		)
	}
}

// _Under runs src with the run allowed only ceiling bytes.
//
// Revisions:
//   - 2026-09-30 21:02: initial creation
func _Under(t *testing.T, ceiling int64, src string) (string, error) {
	t.Helper()

	built, err := runtime.Compile(&runtime.Source{Entry: SCRIPT, Text: []byte(src)})
	if err != nil {
		return "", err
	}

	value, err := runtime.Start(
		context.Background(),
		built,
		runtime.WithMemory(ceiling),
	).Wait()
	if err != nil {
		return "", err
	}

	return value.String(), nil
}
