// Regression probes for what a spawned thread is handed: a lambda written
// inside spawn takes the variables it reads at the spawn, frozen, and anything
// else that would carry a local variable into a thread is refused.
//
// Revisions:
//   - 2026-09-30 20:58: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Case is one function in the script CAPTURE, and what calling it gives:
// a value, a sentinel it fails with, or text its failure contains.
//
// Revisions:
//   - 2026-09-30 20:58: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-09 11:38: names the CAPTURE script, not a file (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Case struct {
	fn    string
	value string
	err   error
	text  string
}

const (
	FROZEN = "frozen"
	BEFORE = "referenced before assignment"

	// CALLED is the fixture with a main that calls one case's function.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CALLED = "%s\n\ndef main():\n    return %s()\n"
)

// CASES is every case in the fixture, in the order it declares them.
//
// Revisions:
//   - 2026-09-30 20:58: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrClosure, formerly ErrCaptures (C121, Q348)
var (
	CASES = []*_Case{
		{
			fn:    "plain",
			value: `["done"]`,
		},
		{
			fn:    "with_arguments",
			value: `["hello, alice"]`,
		},
		{
			fn:    "generated",
			value: `["hi, bob"]`,
		},
		{
			fn:    "fanout",
			value: `[0, 10, 20, 30, 40]`,
		},
		{
			fn:    "many",
			value: `200`,
		},
		{
			fn:    "reused",
			value: `["HELLO", "WORLD"]`,
		},
		{
			fn:    "dag",
			value: `[["a", "b"], ["b"], ["a"]]`,
		},
		{
			fn:    "shared_read",
			value: `[8080, 8080]`,
		},
		{
			fn:    "collected",
			value: `[1, 1]`,
		},
		{
			fn:    "wrapped",
			value: `"fetched archive"`,
		},
		{
			fn:    "elsewhere",
			value: `"total 6"`,
		},
		{
			fn:    "loaded",
			value: `["HELLO"]`,
		},
		{
			fn:   "shared_write",
			text: FROZEN,
		},
		{
			fn:   "main_writes",
			text: FROZEN,
		},
		{
			fn:   "too_early",
			text: BEFORE,
		},
		{
			fn:  "by_name",
			err: runtime.ErrClosure,
		},
		{
			fn:  "made_earlier",
			err: runtime.ErrClosure,
		},
		{
			fn:  "closure_taken",
			err: runtime.ErrClosure,
		},
	}
)

// TestCapture_EveryCase runs each case in the fixture through the compiler and
// the runtime a host uses, under -race like the rest of the suite.
//
// Each case is run as the fixture with a main that calls its function, a run
// calling main and nothing else.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-10-03 00:21: runs each case through a main of its own, there being no
//     Invoke of any other function
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:38: reads the scripts from memory (C114, Q281, Q289)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCapture_EveryCase(t *testing.T) {
	var (
		src = strings.TrimPrefix(
			CAPTURE,
			"\n",
		)
		loader = &_Files{
			files: map[string]string{
				"shout.star": strings.TrimPrefix(
					SHOUT,
					"\n",
				),
			},
		}
	)

	for _, tc := range CASES {
		t.Run(
			tc.fn,
			func(t *testing.T) {
				called := fmt.Appendf(
					nil,
					CALLED,
					src,
					tc.fn,
				)

				var (
					source = &common.Source{
						Entry:  "capture.star",
						Text:   called,
						Loader: loader,
					}
					built, err = runtime.Compile(source)
				)
				if err != nil {
					t.Fatalf(
						"compile: %v",
						err,
					)
				}

				var got string

				run, err := runtime.Start(
					t.Context(),
					built,
				)
				if err != nil {
					t.Fatal(err)
				}

				value, err := run.Wait()
				if err == nil {
					got = value.String()
				}

				_Expected(
					t,
					tc,
					got,
					err,
				)
			},
		)
	}
}

// _Expected fails unless an outcome is what a case says.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Expected(
	t *testing.T,
	tc *_Case,
	got string,
	err error,
) {
	t.Helper()

	switch {
	case tc.err != nil:
		if !errors.Is(
			err,
			tc.err,
		) {
			t.Fatalf(
				"got %q and %v, want %v",
				got,
				err,
				tc.err,
			)
		}

	case tc.text != "":
		if err == nil || !strings.Contains(
			err.Error(),
			tc.text,
		) {
			t.Fatalf(
				"got %q and %v, want a failure saying %q",
				got,
				err,
				tc.text,
			)
		}

	default:
		if err != nil || got != tc.value {
			t.Fatalf(
				"got %q and %v, want %s",
				got,
				err,
				tc.value,
			)
		}
	}
}

const (
	// CAPTURE is the script of capture.star: every case of what a spawned
	// thread is handed, each a function. The text opens with a newline, which
	// the test drops.
	//
	// Revisions:
	//   - 2026-09-30 20:58: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:38: holds the script, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CAPTURE = `
# Every case of what a spawned thread is handed, run by capture_test.go. Each
# case is a function, called by name; under each is what calling it gives.
#
# THE RULE
#
#   1. spawn(lambda: f(x)) runs f(x) inside the new thread, with x as it was
#      when spawn was called. Every variable the lambda reads that is not a
#      global is taken then - a loaded name included, which changes nothing,
#      since a load's value is frozen already.
#   2. What is taken is frozen at the spawn. Neither thread can change it
#      afterwards, and trying is an error rather than a race.
#   3. Anything else that would carry a local variable into a thread is refused
#      at the spawn, naming the variable: a nested function spawned by name, a
#      lambda made earlier and stored, a closure among what a lambda takes.
#   4. A top-level function needs none of this: spawn(worker) is as it was.
#
# HOW
#
#   The compiler gives a lambda written inside spawn(...) one parameter per
#   variable it reads, each defaulting to that variable:
#
#       spawn(lambda: f(x, y))    is compiled as    spawn(lambda x=x, y=y: f(x, y))
#
#   A default is evaluated where the lambda is written, which is the spawn.

load("shout.star", "shout")

def worker():
    return "done"

def greet(name, greeting):
    return greeting + ", " + name

def times_ten(x):
    return x * 10

# ---------------------------------------------------------------------------
# What works
# ---------------------------------------------------------------------------

# A top-level function with nothing to pass.
def plain():
    h = spawn(worker)

    return join(h)

#   compiled as   unchanged
#   result        ["done"]

# Passing arguments.
def with_arguments():
    greeting = "hello"

    h = spawn(lambda: greet("alice", greeting))

    return join(h)

#   compiled as   spawn(lambda greeting=greeting: greet("alice", greeting))
#   result        ["hello, alice"]

# Literal arguments only - what the graph emitter writes.
def generated():
    h = spawn(lambda: greet("bob", "hi"))

    return join(h)

#   compiled as   unchanged: the lambda reads no variable, greet being a global
#   result        ["hi, bob"]

# One thread per item - the shape that raced.
def fanout():
    handles = []

    for x in range(5):
        handles.append(spawn(lambda: times_ten(x)))

    return join(*handles)

#   compiled as   spawn(lambda x=x: times_ten(x)), once per item
#   result        [0, 10, 20, 30, 40]

# Enough threads that a race, were there one, would be seen.
def many():
    handles = []

    for x in range(200):
        handles.append(spawn(lambda: x))

    return len(set(join(*handles)))

#   compiled as   spawn(lambda x=x: x), once per item
#   result        200, every value its own

# One variable, reused between the spawns, read through a loaded name.
def reused():
    word = "hello"
    hello = spawn(lambda: shout(word))

    word = "world"
    world = spawn(lambda: shout(word))

    return join(hello, world)

#   compiled as   spawn(lambda shout=shout, word=word: shout(word))
#   result        ["HELLO", "WORLD"]

# A DAG: each task is handed the handles it waits for.
def fetch_a():
    return "a"

def fetch_b():
    return "b"

def both(ha, hb):
    return join(ha, hb)

def one(h):
    return join(h)

def dag():
    ha = spawn(fetch_a)
    hb = spawn(fetch_b)

    hc = spawn(lambda: both(ha, hb))
    hd = spawn(lambda: one(hb))
    he = spawn(lambda: one(ha))

    return join(hc, hd, he)

#   compiled as   spawn(lambda ha=ha, hb=hb: both(ha, hb)), and so on
#   result        [["a", "b"], ["b"], ["a"]]

# A value several threads read.
def port_of(config):
    return config["port"]

def shared_read():
    config = {"port": 8080}

    h1 = spawn(lambda: port_of(config))
    h2 = spawn(lambda: port_of(config))

    return join(h1, h2)

#   compiled as   spawn(lambda config=config: port_of(config))
#   result        [8080, 8080]; config is frozen from the first spawn on

# Results: a thread returns what it made, and join collects it.
def one_item():
    return [1]

def collected():
    h1 = spawn(one_item)
    h2 = spawn(one_item)

    parts = join(h1, h2)

    return parts[0] + parts[1]

#   compiled as   unchanged
#   result        [1, 1]

# A wrapper's lambda is not a spawn's, and keeps its meaning. retry waits for
# each attempt, so the lambda reading target when it runs races nothing.
def fetch(target):
    return "fetched " + target

def wrapped():
    target = "archive"

    return retry(3, lambda: fetch(target))

#   compiled as   unchanged
#   result        "fetched archive"

# A lambda anywhere else is untouched, loops and all.
def elsewhere():
    total = 0

    for x in [1, 2, 3]:
        total += x

    describe = lambda: "total %d" % total

    return describe()

#   compiled as   unchanged
#   result        "total 6"

# A top-level function using a name its file loaded.
def first():
    return shout("hello")

def loaded():
    h = spawn(first)

    return join(h)

#   compiled as   unchanged
#   result        ["HELLO"]: first is top-level, and shout was bound once,
#                 before anything ran

# ---------------------------------------------------------------------------
# What fails, and says so
# ---------------------------------------------------------------------------

# Writing to what a thread was handed.
def append_one(items):
    items.append(1)

def shared_write():
    items = []

    h1 = spawn(lambda: append_one(items))
    h2 = spawn(lambda: append_one(items))

    join(h1, h2)

    return items

#   compiled as   spawn(lambda items=items: append_one(items))
#   result        fails in the thread: cannot append to frozen list

# Main writing to what it handed over, even after the thread has finished.
def main_writes():
    config = {"port": 8080}

    h = spawn(lambda: port_of(config))
    got = join(h)

    config["port"] = 9090

    return got

#   compiled as   spawn(lambda config=config: port_of(config))
#   result        fails on main's write: config was frozen by the spawn

# A variable read before it is assigned.
def too_early():
    h = spawn(lambda: times_ten(y))
    y = 5

    return join(h)

#   compiled as   spawn(lambda y=y: times_ten(y))
#   result        fails at the spawn, every time: y is referenced before it is
#                 assigned

# ---------------------------------------------------------------------------
# What is refused at the spawn
# ---------------------------------------------------------------------------

# A nested function spawned by name, capturing a local.
def by_name():
    greeting = "hello"

    def greet_here():
        return greeting

    h = spawn(greet_here)

    return join(h)

#   compiled as   unchanged: only a lambda written inside spawn(...) is rewritten
#   result        refused: greet_here captures greeting

# A lambda made earlier and stored, then spawned.
def made_earlier():
    x = 1
    job = lambda: times_ten(x)

    h = spawn(job)

    return join(h)

#   compiled as   unchanged: this lambda is not written inside spawn(...)
#   result        refused: lambda captures x

# A closure among what a lambda takes.
def call(fn):
    return fn()

def closure_taken():
    y = 1

    def read_y():
        return y

    h = spawn(lambda: call(read_y))

    return join(h)

#   compiled as   spawn(lambda read_y=read_y: call(read_y))
#   result        refused: read_y captures y
`

	// SHOUT is the script of shout.star, a module CAPTURE loads, so a lambda
	// there can read a loaded name. The text opens with a newline, which the
	// test drops.
	//
	// Revisions:
	//   - 2026-11-09 11:38: initial creation (C114, Q281, Q287, Q289)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHOUT = `
# A module capture.star loads, so a lambda there can read a loaded name.

def shout(word):
    return word.upper()
`
)
