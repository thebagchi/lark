package tests

// Proofs for the queue module: the capacity, the order, the waits, the close,
// what a script can and cannot push, and what stops the run. Each test names
// the decision it holds (Q408-Q420).
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q408-Q420)

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// SCRIPT is what a failure calls the script it was given.
	//
	// Revisions:
	//   - 2026-11-13 01:29: initial creation (C131)
	SCRIPT = "queue.star"

	// DEADLINE is how long a script gets before it is taken to have hung. A
	// regression in a pop does not fail, it blocks, so every test here runs
	// under this.
	//
	// Revisions:
	//   - 2026-11-13 01:29: initial creation (C131)
	DEADLINE = 20 * time.Second
)

// _Outcome is what one run of a script ended with.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131)
type _Outcome struct {
	got string
	err error
}

// _Ran compiles and runs src, under a deadline.
//
// Deadlined because a defect in a pop does not fail: it blocks, and a suite
// that has to be killed says nothing about which test was wrong.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation, from the helper of the event tests (C131)
func _Ran(
	t *testing.T,
	src string,
) (string, error) {
	t.Helper()

	return _RanIn(
		context.Background(),
		t,
		src,
	)
}

// _RanIn compiles and runs src under ctx and the deadline.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131)
func _RanIn(
	ctx context.Context,
	t *testing.T,
	src string,
) (string, error) {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: SCRIPT,
		Text: []byte(strings.TrimPrefix(
			src,
			"\n",
		)),
	})
	if err != nil {
		return "", err
	}

	answered := make(
		chan _Outcome,
		1,
	)

	go func() {
		run, err := runtime.Start(
			ctx,
			built,
		)
		if err != nil {
			answered <- _Outcome{
				err: err,
			}

			return
		}

		value, err := run.Wait()
		if err != nil {
			answered <- _Outcome{
				err: err,
			}

			return
		}

		answered <- _Outcome{
			got: value.String(),
		}
	}()

	select {
	case held := <-answered:
		return held.got, held.err

	case <-time.After(DEADLINE):
		t.Fatalf(
			"the script did not finish within %v: a pop is not returning",
			DEADLINE,
		)
	}

	return "", nil
}

// TestQueue_Answers runs each script of the GOLDEN table and compares what main
// returned with what the table holds: the capacity, the order, the answers of
// push and pop, the close, and the ways a queue travels between threads.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q408-Q420)
func TestQueue_Answers(t *testing.T) {
	for _, row := range GOLDEN {
		t.Run(
			row.name,
			func(t *testing.T) {
				got, err := _Ran(
					t,
					row.script,
				)
				if err != nil {
					t.Fatalf(
						"got error %v, want %s",
						err,
						row.want,
					)
				}

				if got != row.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						row.want,
					)
				}
			},
		)
	}
}

// TestQueue_StopsTheRun runs each script of the STOPS table and compares the
// error the run ended with against the sentinel the table names.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q411, Q412, Q413, Q417, Q418)
func TestQueue_StopsTheRun(t *testing.T) {
	for _, row := range STOPS {
		t.Run(
			row.name,
			func(t *testing.T) {
				_, err := _Ran(
					t,
					row.script,
				)
				if !errors.Is(
					err,
					row.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						row.want,
					)
				}
			},
		)
	}
}

// TestQueue_Refuses runs each script of the REFUSES table and looks for the
// text the table names in the error: the refusals that stop only the thread, and
// the frozen values.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q413, Q419, Q420)
func TestQueue_Refuses(t *testing.T) {
	for _, row := range REFUSES {
		t.Run(
			row.name,
			func(t *testing.T) {
				_, err := _Ran(
					t,
					row.script,
				)
				if err == nil {
					t.Fatalf(
						"the script ended without an error, want %q",
						row.want,
					)
				}

				if !strings.Contains(
					err.Error(),
					row.want,
				) {
					t.Fatalf(
						"got %v, want it to hold %q",
						err,
						row.want,
					)
				}
			},
		)
	}
}

// TestQueue_RunCancelEndsAPop keeps a forgotten pop from holding a run open:
// the host cancels the run, and the pop raises ErrCancelled (Q416, Q419).
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q416, Q419)
func TestQueue_RunCancelEndsAPop(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		stop()
	}()

	_, err := _RanIn(
		ctx,
		t,
		`
def main():
    q = queue.new()

    return q.pop()
`,
	)
	if !errors.Is(
		err,
		scheduler.ErrCancelled,
	) {
		t.Fatalf(
			"got %v, want ErrCancelled",
			err,
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// GOLDEN lists a script and the value its main returned when the table was
// reviewed.
//
// Revisions:
//   - 2026-11-13 01:29: initial creation (C131, Q408-Q420)
var (
	GOLDEN = []struct{ name, script, want string }{
		{
			"the default holds 64",
			`
def main():
    q = queue.new()
    held = 0

    for i in range(100):
        if q.push(i) == queue.FULL:
            break

        held += 1

    return held
`,
			`64`,
		},
		{
			"zero is the default",
			`
def main():
    q = queue.new(0)
    held = 0

    for i in range(100):
        if q.push(i) == queue.FULL:
            break

        held += 1

    return held
`,
			`64`,
		},
		{
			"a given capacity",
			`
def main():
    q = queue.new(3)
    held = 0

    for i in range(100):
        if q.push(i) == queue.FULL:
            break

        held += 1

    return held
`,
			`3`,
		},
		{
			"a capacity of one",
			`
def main():
    q = queue.new(1)

    return (q.push("a"), q.push("b"))
`,
			`(None, "full")`,
		},
		{
			"a full queue is not a failure and holds its items",
			`
def main():
    q = queue.new(2)
    q.push(1)
    q.push(2)
    refused = q.push(3)

    return (refused, q.pop(), q.pop())
`,
			`("full", (1, None), (2, None))`,
		},
		{
			"a pop makes room",
			`
def main():
    q = queue.new(1)
    q.push(1)
    q.pop()

    return q.push(2)
`,
			`None`,
		},
		{
			"the constants",
			`
def main():
    return (queue.FULL, queue.CLOSED)
`,
			`("full", "closed")`,
		},
		{
			"first in, first out",
			`
def main():
    q = queue.new()

    for i in range(5):
        q.push(i)

    return [q.pop()[0] for _ in range(5)]
`,
			`[0, 1, 2, 3, 4]`,
		},
		{
			"order holds across a full ring",
			`
def main():
    q = queue.new(3)
    got = []
    next = 0

    for round in range(40):
        for _ in range(2):
            if q.push(next) == None:
                next += 1

        got.append(q.pop()[0])

    # Each round pushes two and pops one, so two items are left.
    for _ in range(2):
        got.append(q.pop()[0])

    return got == list(range(len(got)))
`,
			`True`,
		},
		{
			"a pop answers the pair",
			`
def main():
    q = queue.new()
    q.push("x")

    return q.pop()
`,
			`("x", None)`,
		},
		{
			"None is an item and is not close",
			`
def main():
    q = queue.new()
    q.push(None)

    return q.pop()
`,
			`(None, None)`,
		},
		{
			"a close lets the items drain, then answers closed",
			`
def main():
    q = queue.new()
    q.push(1)
    q.push(2)
    q.close()

    return [q.pop(), q.pop(), q.pop(), q.pop()]
`,
			`[(1, None), (2, None), (None, "closed"), (None, "closed")]`,
		},
		{
			"a second close does nothing",
			`
def main():
    q = queue.new()
    q.push(1)

    return (q.close(), q.close(), q.pop(), q.pop())
`,
			`(None, None, (1, None), (None, "closed"))`,
		},
		{
			"a pop waits until a push",
			`
def waiter(q):
    return q.pop()

def main():
    q = queue.new()
    held = spawn(lambda: waiter(q))

    sleep(50)
    q.push("late")

    return join(held)[0]
`,
			`("late", None)`,
		},
		{
			"a pop waits until a close",
			`
def waiter(q):
    return q.pop()

def main():
    q = queue.new()
    held = spawn(lambda: waiter(q))

    sleep(50)
    q.close()

    return join(held)[0]
`,
			`(None, "closed")`,
		},
		{
			"a close wakes every waiting pop",
			`
def waiter(q):
    return q.pop()

def main():
    q = queue.new()
    first = spawn(lambda: waiter(q))
    second = spawn(lambda: waiter(q))
    third = spawn(lambda: waiter(q))

    sleep(50)
    q.close()

    return join(first, second, third)
`,
			`[(None, "closed"), (None, "closed"), (None, "closed")]`,
		},
		{
			"one producer, one consumer, in order",
			`
def produce(q):
    for i in range(200):
        while q.push(i) == queue.FULL:
            sleep(1)

    q.close()

def consume(q):
    got = []

    for _ in range(1000):
        item, err = q.pop()

        if err == queue.CLOSED:
            return got

        got.append(item)

    fail("no close")

def main():
    q = queue.new(8)
    sending = spawn(lambda: produce(q))
    reading = spawn(lambda: consume(q))

    join(sending)

    return join(reading)[0] == list(range(200))
`,
			`True`,
		},
		{
			"many producers, many consumers: each item once, each producer in order",
			`
def produce(q, who):
    for i in range(100):
        while q.push(who * 1000 + i) == queue.FULL:
            sleep(1)

def consume(q):
    got = []

    for _ in range(1000):
        item, err = q.pop()

        if err == queue.CLOSED:
            return got

        got.append(item)

    fail("no close")

def main():
    q = queue.new(16)
    makers = [spawn(lambda who=who: produce(q, who)) for who in range(4)]
    takers = [spawn(lambda: consume(q)) for _ in range(4)]

    for maker in makers:
        join(maker)

    q.close()

    lists = [join(taker)[0] for taker in takers]
    every = []

    for got in lists:
        every += got

        # Within one consumer, the items of one producer arrive in order.
        for who in range(4):
            mine = [x for x in got if x // 1000 == who]

            if mine != sorted(mine):
                fail("producer %d out of order" % who)

    expected = []

    for who in range(4):
        for i in range(100):
            expected.append(who * 1000 + i)

    return sorted(every) == expected
`,
			`True`,
		},
		{
			"a queue goes to a spawned thread",
			`
def worker(q):
    q.push("from the worker")

def main():
    q = queue.new()
    held = spawn(lambda: worker(q))

    join(held)

    return q.pop()
`,
			`("from the worker", None)`,
		},
		{
			"a queue as a module global works when frozen",
			`
Q = queue.new()

def worker():
    Q.push("from the global")

def main():
    join(spawn(worker))
    Q.push("from main")

    return (Q.pop(), Q.pop())
`,
			`(("from the global", None), ("from main", None))`,
		},
		{
			"a queue is returned from a function",
			`
def make():
    q = queue.new(2)
    q.push("made")

    return q

def main():
    return make().pop()
`,
			`("made", None)`,
		},
		{
			"two queues are two queues",
			`
def main():
    a = queue.new()
    b = queue.new()
    a.push(1)

    return (b.close(), a.pop(), b.pop())
`,
			`(None, (1, None), (None, "closed"))`,
		},
		{
			"type and methods",
			`
def main():
    q = queue.new()

    return (type(q), dir(q), str(q))
`,
			`("queue", ["close", "pop", "push"], "<queue 0/64>")`,
		},
		{
			"print shows the items held, the capacity and closed",
			`
def main():
    q = queue.new(5)
    a = str(q)

    q.push(1)
    q.push(2)
    q.push(3)

    b = str(q)

    q.pop()
    q.close()

    return (a, b, str(q))
`,
			`("<queue 0/5>", "<queue 3/5>", "<queue 2/5 closed>")`,
		},
		{
			"print of a full and of a default queue",
			`
def main():
    q = queue.new(1)
    q.push("x")
    q.push("y")

    return (str(q), str(queue.new(0)), str(queue.new()))
`,
			`("<queue 1/1>", "<queue 0/64>", "<queue 0/64>")`,
		},
		{
			"push and close are allowed inside an update",
			`
def main():
    q = queue.new()

    state.set("k", 1)

    def inside(v):
        q.push("from update")
        q.close()

        return v + 1

    state.update("k", inside)

    return (state.get("k"), q.pop(), q.pop())
`,
			`(2, ("from update", None), (None, "closed"))`,
		},
		{
			"a pop that nobody joined is cut off at the end of the run",
			`
def waiter(q):
    return q.pop()

def main():
    q = queue.new()
    spawn(lambda: waiter(q))
    sleep(20)

    return "done"
`,
			`"done"`,
		},
		{
			"an item is frozen and so is the one popped",
			`
def main():
    q = queue.new()
    items = [1, 2]
    q.push(items)

    got, _ = q.pop()

    return (got, got == items, list(got) + [3])
`,
			`([1, 2], True, [1, 2, 3])`,
		},
	}

	// STOPS lists a script that must stop the run, and the sentinel the run ends
	// with.
	//
	// Revisions:
	//   - 2026-11-13 01:29: initial creation (C131, Q411, Q412, Q413, Q417, Q418)
	STOPS = []struct {
		name, script string
		want         error
	}{
		{
			"a push after close",
			`
def main():
    q = queue.new()
    q.close()
    q.push(1)

    return "reached"
`,
			queue.ErrClosed,
		},
		{
			"a push after close in a thread nobody joined stops the whole run",
			`
def late(q):
    q.push(1)

def main():
    q = queue.new()
    waiting = queue.new()
    q.close()
    spawn(lambda: late(q))

    return waiting.pop()
`,
			queue.ErrClosed,
		},
		{
			"a function is not data",
			`
def helper():
    return 1

def main():
    q = queue.new()
    q.push(helper)

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a function inside a list is not data",
			`
def helper():
    return 1

def main():
    q = queue.new()
    q.push([1, {"k": helper}])

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a queue is not data for a queue",
			`
def main():
    q = queue.new()
    q.push(queue.new())

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a thread handle is not data",
			`
def main():
    q = queue.new()
    q.push(spawn(lambda: 1))

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a queue is not data for event.post",
			`
def main():
    event.post("e", queue.new())

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a queue is not data for state.set",
			`
def main():
    state.set("k", queue.new())

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a value nested past the depth limit",
			`
def main():
    q = queue.new()
    deep = []

    for _ in range(20000):
        deep = [deep]

    q.push(deep)

    return "reached"
`,
			trail.ErrDeep,
		},
		{
			"a negative capacity",
			`
def main():
    queue.new(-1)

    return "reached"
`,
			unpack.ErrRange,
		},
		{
			"a capacity past an int32",
			`
def main():
    queue.new(1099511627776)

    return "reached"
`,
			unpack.ErrRange,
		},
		{
			"a capacity that is a string",
			`
def main():
    queue.new("8")

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a capacity that is a float",
			`
def main():
    queue.new(1.5)

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a capacity that is a bool",
			`
def main():
    queue.new(True)

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a capacity that is None",
			`
def main():
    queue.new(None)

    return "reached"
`,
			unpack.ErrConvert,
		},
		{
			"a bad capacity in a thread nobody joined stops the whole run",
			`
def main():
    waiting = queue.new()
    spawn(lambda: queue.new(-5))

    return waiting.pop()
`,
			unpack.ErrRange,
		},
		{
			"a pop inside an update",
			`
def main():
    q = queue.new()
    q.push(1)
    state.set("k", 1)

    def inside(v):
        return q.pop()

    return state.update("k", inside)
`,
			scheduler.ErrNested,
		},
		{
			"a pop in a thread spawned inside an update",
			`
def main():
    q = queue.new()
    q.push(1)
    state.set("k", 1)

    def inside(v):
        return join(spawn(lambda: q.pop()))

    return state.update("k", inside)
`,
			scheduler.ErrNested,
		},
	}

	// REFUSES lists a script that ends in an error and the text that error holds.
	//
	// Revisions:
	//   - 2026-11-13 01:29: initial creation (C131, Q413, Q419, Q420)
	REFUSES = []struct{ name, script, want string }{
		{
			"the list a push froze cannot be appended to",
			`
def main():
    q = queue.new()
    items = [1]
    q.push(items)
    items.append(2)
`,
			`frozen list`,
		},
		{
			"the list a pop gave out is the frozen one",
			`
def main():
    q = queue.new()
    q.push([1])
    got, _ = q.pop()
    got.append(2)
`,
			`frozen list`,
		},
		{
			"a push the queue refused froze the value too",
			`
def main():
    q = queue.new(1)
    q.push(0)
    items = [1]
    q.push(items)
    items.append(2)
`,
			`frozen list`,
		},
		{
			"a pop takes no parameter",
			`
def main():
    q = queue.new()
    q.push(1)
    q.pop(100)
`,
			`queue.pop: got 1 arguments`,
		},
		{
			"a pop takes no keyword",
			`
def main():
    q = queue.new()
    q.push(1)
    q.pop(ms=100)
`,
			`queue.pop: unexpected keyword`,
		},
		{
			"a push takes one value",
			`
def main():
    queue.new().push()
`,
			`queue.push: got 0 arguments`,
		},
		{
			"a push takes only one value",
			`
def main():
    queue.new().push(1, 2)
`,
			`queue.push: got 2 arguments`,
		},
		{
			"a close takes no parameter",
			`
def main():
    queue.new().close(1)
`,
			`queue.close: got 1 arguments`,
		},
		{
			"new takes at most one",
			`
def main():
    queue.new(1, 2)
`,
			`queue.new: got 2 arguments`,
		},
		{
			"a queue has no other method",
			`
def main():
    queue.new().size()
`,
			`no .size field or method`,
		},
		{
			"a queue is not a key",
			`
def main():
    {queue.new(): 1}
`,
			`unhashable`,
		},
		{
			"a timeout around a pop cuts it",
			`
def patient():
    return queue.new().pop()

def main():
    return timeout(200, patient)
`,
			`timed out`,
		},
	}
)

// END GOLDEN
