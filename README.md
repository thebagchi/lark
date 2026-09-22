# lark

Embed [Starlark](https://github.com/bazelbuild/starlark) in Go, with concurrency.

A script and every module it loads compile into one artifact. Functions run on
goroutines and interpreter threads of their own, and a script starts, waits for
and stops that work itself:

```python
load("util.star", "shout")

def hello():
    return shout("hello")

def world():
    return shout("world")

def main():
    assert(1 + 1 == 2, "arithmetic still works")

    parts = join(spawn(hello), spawn(world))

    return " ".join(parts)
```

```
"HELLO WORLD"
```

## Try it

```
make binaries
./bin/lark.bin -s samples/concurrent.star
./bin/lark.bin -s samples/concurrent.star -t   # the graph it describes, as JSON
```

A script and the graph it describes are two forms of one workflow, and `lark`
runs either and translates either into the other:

```
lark -s script.star        run the script
lark -s script.star -t     print the graph it describes, as JSON
lark -g graph.json         run the graph
lark -g graph.json -t      print the Starlark it generates
lark -s script.star -l dir keep a transcript in dir/script.star.log
lark -s script.star -b out.bin  compile into a bundle instead of running
```

The two translations are inverses, so this prints what the first line printed:

```
lark -s samples/concurrent.star -t > graph.json && lark -g graph.json
```

A graph names no modules: deriving one inlines what the script loaded, so what
comes back is self-contained. A graph is checked before anything is generated
from it, and a graph that will not run is refused with the reason.

```
"HELLO WORLD"
```

`samples/` has one script per idea, each with a comment saying what it is for:

| Script | Shows |
| --- | --- |
| `hello.star` | the smallest script this will run |
| `strings.star` | a library, with no `main` of its own |
| `modules.star` | `load`, resolving beside the loading file |
| `concurrent.star` | `spawn` and `join` |
| `failfast.star` | a failed `assert` stopping the whole run, and `join` giving up early |
| `failkinds.star` | `assert` against `fail` — swap one line and time it |
| `state.star` | `state` passing data between threads |
| `rcu.star` | read-copy-update: read a copy, change it, publish it |
| `flow.star` | `repeat`, `retry`, `timeout` and `n()` |
| `graph.star` | a `workflow.Graph` as JSON, and the Starlark generated from it |
| `pointers.star` | RFC 6901: `extract_json`, `match_json`, `len_json`, `find_key` |
| `patch.star` | RFC 6902: `patch_json`, and the input left unchanged |
| `clock.star` | `time` — durations and instants |
| `numbers.star` | `math`, and what it returns |
| `cancel.star` | `cancel`, and what joining a cancelled handle gives |
| `encode.star` | the `json` plugin |

Four exit non-zero on purpose — `cancel.star`, `failfast.star`,
`failkinds.star` and `strings.star` — because what a failure looks like is part
of the interface.

## Install

```
go get github.com/thebagchi/lark
```

Go 1.26 or newer.

## Running a script

A host imports one package and writes a loader. The loader is how the runtime
reaches a module a script asks for — a directory, an archive, a database, a map
in memory; the runtime does not care.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/thebagchi/lark/runtime"
)

type Disk struct{ root string }

func (d *Disk) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

func (d *Disk) Load(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, name))
}

func main() {
	loader := &Disk{root: "scripts"}

	src, err := loader.Load("greet.star")
	if err != nil {
		log.Fatal(err)
	}

	artifact, err := runtime.NewCompiler(runtime.WithLoader(loader)).
		Compile("greet.star", src)
	if err != nil {
		log.Fatal(err)
	}

	value, err := artifact.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
}
```

Without `WithLoader`, a module is a file beside the one that loaded it:
`load("util.star", …)` inside `scripts/greet.star` reads `scripts/util.star`.

## What a script gets

| Name | What it does |
| --- | --- |
| `spawn(fn)` | Runs `fn` on a goroutine and an interpreter thread of its own. Returns a handle. |
| `join(h, …)` | Waits for each handle and returns what each produced, in the order given. |
| `cancel(h, …)` | Stops each handle. Does not wait. |
| `assert(cond, msg)` | Stops the **whole run** when `cond` is false. `assert(msg = "...")` always stops it. |
| `fail(msg, …)` | Starlark's own. Aborts the **calling thread**. |
| `sleep(seconds)` | Pauses this evaluation. A cancel cuts it short. |
| `repeat(n, fn)` | Calls `fn` exactly `n` times and returns the last result. |
| `retry(n, fn)` | Calls `fn` until one attempt succeeds. Retries **only** assertions. |
| `timeout(seconds, fn)` | Calls `fn` with a limited time, and fails if it runs past it. |
| `n()` | The 1-based attempt number, inside `repeat` or `retry`. |
| `load(path, name)` | Binds a name from another script. |
| `json` | `json.encode`, `json.decode`, and the rest of the module go.starlark.net ships. |
| `state` | `state.set`, `state.get`, `state.update` — see below. |
| `time`, `math` | go.starlark.net's own modules. |
| `jsonpath` | `patch_json`, `extract_json`, `match_json`, `len_json`, `find_key`. |
| `codec` | Twelve conversions between bytes, hex, bits and ints, plus `b64encode` / `b64decode`, `b64urlencode` / `b64urldecode`, `b32encode` / `b32decode` and `crc32`. See below. |
| `utils` | `utils.datetime()`, the local time to the microsecond, as a string. |

`spawn` takes a **named function that takes no arguments** — not a lambda, not a
call. A closure over what it needs is how a script passes data in. The rule
exists so every thread has a name to report.

### Sharing data between threads

Module scope is frozen before anything concurrent runs, so a script cannot share
data by assigning to a global. `state` is how threads pass anything to each
other:

```python
def writer():
    state.set("message", "written by one thread")

def reader():
    return state.get("message")

def main():
    join(spawn(writer))
    return join(spawn(reader))[0]
```

**A store belongs to one execution.** Two runs of the same artifact get two
stores, and every thread inside a run gets the same one. Reading a name nothing
has written gives `None`, because that is the ordinary case in a store threads
share.

**The store works like read-copy-update.** What is stored is frozen, so threads
reading one name at once can never be handed something another can change
underneath them. `state.get` therefore returns a **copy**: a script owns what it
read, changes it freely, and nothing it does is visible anywhere until it
publishes the result.

```python
mine = state.get("findings")   # read — a copy of the published value
mine.append("two")             # copy — nobody else can see this yet
state.set("findings", mine)    # update — now they can
```

A copy costs time proportional to the size of the value, on every read, so a
script walking a large structure should read it once rather than in a loop.

Copies preserve sharing and survive cycles: a list reachable by two paths stays
one list, and a list that contains itself copies without looping.

**Use `state.update` when other threads write the same name.** A `get` followed
by a `set` is two operations, and another thread writing between them loses one
of the updates — silently, and only under load:

```python
state.update("count", lambda n: n + 1)
```

Four threads incrementing one name two hundred times each:

| | Result |
| --- | --- |
| `state.set(n, state.get(n) + 1)` | 294, 575, 317, 644, 442 — a different number every run |
| `state.update(n, lambda v: v + 1)` | 800, every run |

The lock is Go's and covers the whole of read, call and write. A script never
sees it and cannot forget to release it. The function is called with `None` when
nothing is stored yet, and it must not start another update — two threads
updating two names in opposite orders would wait on each other forever, so
nesting is refused rather than risked.

Each of these is a plugin, and a host enables it by importing it:

```go
import (
    _ "github.com/thebagchi/lark/runtime/plugin/state"
    _ "github.com/thebagchi/lark/runtime/plugin/jsonpath"
    _ "github.com/thebagchi/lark/runtime/plugin/time"
    _ "github.com/thebagchi/lark/runtime/plugin/math"
    _ "github.com/thebagchi/lark/runtime/plugin/codec"
    _ "github.com/thebagchi/lark/runtime/plugin/utils"
)
```

`cmd/lark` imports all of them, so every sample can use them.

### Two naming styles in `codec`, and why

The twelve the brief asks for are `x2y`: `bytes2hex`, `hex2bits`, `int2bytes`.
The base encodings are `encode` and `decode`, because a name ending in a digit
cannot take the `2` infix without reading as a number - `base642bytes` is "base
642 bytes" to anyone who has not been told otherwise.

```python
hex = bytes2hex(data)              # the twelve, unchanged
token = b64urlencode(payload)      # unpadded, which is what a JWT carries
data = b64urldecode(token)         # padded or not, either way
sum = crc32(chunk, sum)            # continues a checksum already started
```

Anything conceptually bytes takes a `str` too, so `b64encode("foobar")` works.
Quoted-printable, uuencode and `crc_hqx` are deliberately absent: two are
email-era formats and the third serves one obsolete protocol.

### Repeating, retrying and bounding

Each calls straight away and gives back what the call produced.

```python
last = repeat(3, step)             # calls step three times
```

| | |
| --- | --- |
| `repeat(n, fn)` | exactly `n` calls, stopping at the first error, returning the last success |
| `retry(n, fn)` | up to `n` calls, returning the first success |
| `timeout(seconds, fn)` | one call, failing if it runs past the limit |

They do not compose: `retry(3, timeout(5, fn))` is refused, because a wrapper
takes a function and gives back a result, not another function.

**`retry` retries only assertions.** A `fail()`, a cancelled handle or a refused
builtin propagates at once. That is what the two failure kinds are for: `assert`
says a check did not hold, which may be timing; `fail` says this cannot work.

An assertion normally ends the whole run — inside a `retry` it ends only that
attempt, because the thread the attempt runs on is marked as catching. When
every attempt has failed, the last assertion propagates and ends the run exactly
as a bare assertion would.

**`n()` is a call, not a variable.** A plain name belongs to the *module*, and
every thread of a run shares one of those: a global is an index into a single
slice of values, a predeclared name a key in a single map. Neither is reachable
per thread, so a bare `n` would be one number for every attempt at once, and two
wrappers running side by side would read each other's. A builtin is handed the
calling thread, which is where the attempt is kept, so `n()` can answer. Outside
a `repeat` or `retry` it is an error rather than a guess.

**Each attempt runs on its own goroutine and interpreter thread**, so two
wrappers running in parallel never share an attempt count, and `timeout` can
stop waiting for one.

The count comes first and the function last, because the function is the subject
and a lambda is the argument most likely to grow. `n` must be at least 1.

**A lambda is accepted**, and it is what you write when a call needs arguments:
`repeat(3, lambda: greet("alice"))`. A named function reports its own name;
an anonymous one has none, so a status shows the thread with no function
against it rather than a name that is not true.

### `assert` or `fail`?

Both end an evaluation and neither can be caught — Starlark has no `try`, and
reserves `raise` as a keyword without implementing it. They differ in **how far
the failure reaches**, and that is the whole of the choice:

| | `assert(cond, msg)` | `fail(msg, …)` |
| --- | --- | --- |
| Supplied by | this runtime | Starlark itself |
| Takes a condition | yes | no — it always aborts |
| Ends | **the whole run**: spine, every spawned thread, anything they spawned | **the calling thread** |
| Siblings | cancelled where they stand, always | cancelled only if `join` reaches the failure first |
| At a `join` | supersedes the join — the run has one outcome | reported first **in argument order** |
| Joined or not | fails the run either way | only surfaces if something joins it |
| Retried by `retry` *(when it exists)* | yes | no |

Put plainly: **`assert` means this run is over. `fail` means this operation
cannot continue.**

The sibling row has a wrinkle worth knowing. `join` is fail-fast, so it cancels
whatever it has not reached once something fails — which means a `fail` *also*
stops siblings when the failing handle comes first in the join. Where they truly
differ is when it comes last:

```python
join(slow, dies)   # fail:   waits the slow one out, then reports
                   # assert: cancels it immediately
```

`samples/failkinds.star` is that script, and the two spellings differ by about
three orders of magnitude in wall time.

```python
def check(value):
    if value < 0:
        fail("negative value:", value)   # this call is wrong; siblings carry on
    return value

def main():
    assert(ready(), "the fixture never came up")   # nothing can be salvaged
```

`fail` takes any number of values and joins them with spaces, so
`fail("code", 42)` reads `fail: code 42`.

The last row matters even though `retry` is not built yet: `plan.md` §9.1
reserves the distinction, so a step meant to be retried should `assert`, and one
that must abort regardless should `fail`.

### The three shapes of `assert`

```python
assert(x == 1)                  # stops the run when x is not 1
assert(x == 1, "x was wrong")   # the same, with a message
assert(msg = "unreachable")     # always stops it
```

`assert("some text")` is **refused**, not run — and the refusal stops the run
too. It reads like the third form and would behave like the first, since a
non-empty string is true, so it would otherwise pass silently. A mistyped
assertion is still an assertion; a typo must not quietly reduce it to nothing.

### Failure

**A failed assertion stops the whole run** — the spine, every spawned thread,
and anything they spawned, whether or not anyone joins the failed handle. The
first assertion ends the script, as it would in a test runner.

```python
def broken():
    assert(False, "the reason")

def main():
    slow = spawn(counts_a_long_way)
    join(spawn(broken), slow)   # raises: "the reason": assertion failed
                                # and slow is cancelled, not waited for
```

**`join` is fail-fast.** It waits on handles in argument order, and the first
failure cancels the ones it has not reached — then waits for them, so no
evaluation is abandoned mid-flight.

There are two failure kinds, and they differ in exactly one way:

| | Ends | Reported |
| --- | --- | --- |
| `assert` | the whole run | the assertion, wherever it happened, joined or not |
| a refused `assert("text")` | the whole run | the refusal — a mistyped assertion is still an assertion |
| everything else — `fail()`, a cancelled handle, a panicking builtin | the thread it happened on | the first **in argument order** at the join |

So for ordinary failures the same script reports the same failure every run,
whichever thread lost the race. An assertion supersedes that: it stops the run,
and a run has one outcome — the first assertion recorded. A sibling that would
have failed later is cancelled before it can.

An assertion in a thread **nobody joins** still fails the run. A run does not
report success while it is being torn down.

## The Go surface

```go
compiler := runtime.NewCompiler(runtime.WithLoader(loader))

artifact, err := compiler.Compile("entry.star", src)   // entry + every module it loads
value, err := artifact.Run(ctx)                        // calls main
value, err := artifact.Invoke(ctx, "other")            // calls any top-level function
units, err := artifact.Save()                          // the compiled units, as bytes

ctx = runtime.WithPrinter(ctx, func(line string) { ... })   // where print goes
ctx = runtime.WithReporter(ctx, reporter)                   // starts, ends and prints
```

What a script prints goes to standard error unless the context says otherwise.
`WithPrinter` collects only the lines; `WithReporter` takes a `runtime.Reporter`,
which is told when each function starts and ends as well. One reporter per run:
the later of the two wins.

`Compile` builds the whole load graph **before anything executes**. A cycle or a
missing entry point is refused without a single top-level statement having run,
and a cycle names the ring it found:

```
cycle in the load graph: a.star -> b.star -> a.star
```

Every module is fetched once per compile, however many scripts load it.

### Failures a host can act on

Each is reachable with `errors.Is`, through whatever wrapping carried it.

| Sentinel | Raised when |
| --- | --- |
| `runtime.ErrNoMain` | No `main`, or one that takes arguments |
| `runtime.ErrCycle` | The load graph closes on itself |
| `runtime.ErrNoLoader` | A script loads and the compiler has no loader |
| `runtime.ErrNoGlobal` | `Invoke` names something that is not there |
| `runtime.ErrNotCallable` | `Invoke` names something that is not a function |
| `runtime.ErrAssert` | A script asserted false |
| `runtime.ErrNotACondition` | `assert` was given only a message |
| `runtime.ErrCancelled` | A joined handle was cancelled |
| `runtime.ErrNotAName` | `spawn` got something other than a zero-argument function |
| `runtime.ErrNotAHandle` | `join` or `cancel` got something other than a handle |
| `runtime.ErrInterrupted` | A `sleep` was cut short by the run ending |
| `runtime.ErrDuration` | `sleep` or `timeout` got something no timer can hold |
| `runtime.ErrNested` | `state.update` was called from inside an update, on any thread it started |
| `runtime.ErrConflict` | Two plugins supply one name |
| `runtime.ErrUnknown` | No run answers to that id |

Every sentinel of every package the facade wraps is here. A plugin you import
yourself - `flow`, `state`, `jsonpath` - keeps its own.

### Cancellation

Cancelling the context passed to `Run` or `Invoke` stops the script's own
evaluation and every thread it spawned. `cancel(h)` stops one thread and leaves
its siblings alone.

A cancelled evaluation stops at its next instruction. It does **not** interrupt
a Go function already running, so a builtin that blocks must take a context of
its own.

`Run` does not return until every thread it started has stopped. A handle nobody
joined is cancelled rather than waited for, so a forgotten `spawn` cannot hold a
call open.

## A bundle carries its own picture

`-b` compiles without running and writes one file: which unit is the entry,
every compiled unit in dependency order, and the graph a user interface draws
it as.

```
lark -s build.star -b build.bin
```

The graph in a bundle carries **no function bodies**. The runnable program is
already there, compiled, and a body beside it would be the same thing twice.
Nothing regenerates a script from a bundle, because a bundle has one.

It carries no status either, because a graph says what will happen. A reader
draws it as a workflow that has not started:

```go
snap := runtime.Pending(artifact.Graph())   // every node PENDING
```

A script that compiles may still have parts no graph draws as steps, and
`Unmodelled()` says which. That is not a loss: such a function is carried
whole, as the body it was written as, and a graph containing one still
regenerates its script exactly. When there is no graph at all, this is the
only answer, so a bundle nobody could describe is never mistaken for one
nobody described.

When the graph came first, the bundle keeps that graph rather than deriving
one from the script it generated:

```
lark -g graph.json -b build.bin
```

## Every run keeps its own transcript

What a script prints goes where the caller says. `-l` keeps it as well, one
file per run, each line behind the thread that wrote it:

```
lark -s build.star -l logs
```

```
thread_0  from the spine
thread_2  from beta
thread_1  from alpha
```

The lane matters because a concurrent script interleaves: the three lines above
arrived in that order on standard output too, and without the prefix the file
could not say which thread said what.

A host embedding the runtime asks a store for the same thing:

```go
id := store.Start(ctx, artifact, observe.WithLogs("/var/log/lark"))
```

**The file is the run's id with `.log` on the end, under the directory you
named.** That rule is the whole of what a poller needs, which is why `Workflow`
carries no path: you named the directory and you hold the id. Two runs of one
artifact are two files, because an id is what tells them apart. The directory
is created if it is not there, a run whose file cannot be opened fails before
its script is evaluated, and the sweep that forgets a run after its
time-to-live does not delete the file.

## Watching a run

`Run` and `Invoke` block until the script is over. A long-running host — a
service with a web interface rather than a command-line tool — wants the other
shape: start the script, get an id back at once, and ask about it afterwards.

```go
id := runtime.Start(ctx, artifact)         // returns immediately, a v7 UUID
snap, err := runtime.Status(id)            // how is it doing
value, err := runtime.Wait(ctx, id)        // block until it is over
err := runtime.Cancel(id)                  // stop it, without waiting
```

`Start` returns while the script is still running, whatever it goes on to do.
The id is a version 7 UUID, so it sorts by start time and can never be reissued
— a stale id from a previous process is always unknown rather than quietly
attaching to a different run.

### What a status says

`runtime.Status` returns a `*runtime.Workflow` — [`workflow.proto`](proto/workflow.proto)'s
own message, not a copy. A script whose `main` joins a failing `bad` and a
sleeping `patient` reports:

```json
{
  "status": "STATUS_FAILED",
  "threads": [
    { "nodes": [{ "function": "main", "status": "STATUS_FAILED",
                  "failure": "\"this one breaks\": assertion failed" }] },
    { "index": 1,
      "nodes": [{ "function": "bad", "status": "STATUS_FAILED",
                  "failure": "\"this one breaks\": assertion failed" }] },
    { "index": 2,
      "nodes": [{ "function": "patient", "status": "STATUS_CANCELLED" }] }
  ],
  "cause": { "thread": 1, "function": "bad",
             "failure": "\"this one breaks\": assertion failed" }
}
```

One node per function per thread, so the same function spawned twice is two
nodes with one name. A thread's number is the one `spawn` gave it; the entry
point is thread 0 and carries no `index`, because zero is the default.

`main` is failed here too, and that is not double-counting: the assertion
reached it through `join`, so its own evaluation failed. `cause` is what says
which of the two mattered.

| Status | Means |
| --- | --- |
| `STATUS_RUNNING` | still going |
| `STATUS_SUCCEEDED` | finished |
| `STATUS_FAILED` | something went wrong, and `failure` says what |
| `STATUS_CANCELLED` | stopped — **not** a failure |
| `STATUS_PENDING` | declared by a graph and not reached |

**Cancelled is not failed, and the difference is worth having.** This runtime is
fail-fast, so a failing run stops threads that were doing nothing wrong. Without
a separate value, one broken script reports as several.

`failure` is set **only** for `STATUS_FAILED`, so a non-empty `failure` means
there is something to show without first checking the status. A cancellation's
text would only say it was cancelled, which the status already says.

`cause` names the node whose failure ended the run — thread, function and text —
so a host follows it to a node instead of searching every thread for a failed
one. It is nil unless the run failed.

### Reading, waiting and forgetting

Asking costs nothing and changes nothing. Every caller that asks is told how a
run ended, as often as they like, and two interfaces watching one run are both
answered.

**A finished run is forgotten 24 hours after it ended**, read or not. Nothing
else forgets it: `Status` does not consume it and neither does `Wait`. So a host
that starts runs and never asks is bounded by time rather than by attention —
and a finished run holds what it produced for that day, which is the price of
answering everyone.

After that, its id is unknown, which is the same answer as an id that never
existed. A host polling a run a day later cannot tell those apart.

`Wait` takes a context of its own, and it is the **caller's** patience, not the
run's. Giving up on a wait leaves the run untouched; stopping the run is
`Cancel`, or the context passed to `Start`.

### Saying what has not happened yet

Without a graph, a report says what the run **did**: a function nothing called
never appears. Hand one over and every function it declares starts `PENDING`,
so a report can also say what was not reached.

```go
id := runtime.Start(ctx, artifact, runtime.WithGraph(graph))
```

A graph is a **floor, not a ceiling**. It may add functions and threads that
have not happened; it may never remove, rename or hide one that did. A run that
spawns something the graph never mentions is reported anyway, beside the pending
ones — when a graph and a run disagree, both are visible and the run wins.

A function left pending after the run ended is not an error. It means the run
finished without going there.

### A store of your own

`runtime.Start` and friends use one store the package owns, which is what makes
them plain calls. Two hosts in one process share it, and tests in one binary
cannot isolate from each other's runs. Neither is solved by hiding it:

```go
import "github.com/thebagchi/lark/runtime/observe"

store := observe.New()          // your own, with the same methods
id := store.Start(ctx, artifact)
```

This is the one place a host names a subpackage. Everything above is reachable
through `runtime` alone.

## Adding your own names

A plugin says what it is called and what it supplies. It registers itself, so
importing the package is the whole of enabling it:

```go
package clock

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
)

func init() { runtime.Register(&plugin{}) }

type plugin struct{}

func (p *plugin) Name() string { return "clock" }

func (p *plugin) Values() starlark.StringDict {
	return starlark.StringDict{
		"now": starlark.NewBuiltin("now", now),
	}
}
```

```go
import _ "your/module/clock"   // the import is the installation
```

Two plugins supplying one name is refused when a compile builds its environment,
naming both — a shadowed builtin is a defect that otherwise surfaces much later
as wrong behaviour somewhere else.

**Registration by import is process-wide.** A plugin that registers itself from
its `init` installs its names into the default registry, which every compiler
uses unless told otherwise. A compiler that should see a set of its own is given
one:

```go
mine := runtime.NewRegistry()
mine.Register(&plugin{})

compiler := runtime.NewCompiler(runtime.WithPlugins(mine))
```

## The dialect

Sets, `while` loops and recursion are enabled. Reassigning a top-level name is
not: a workflow freezes its globals, so a rebind would fail at run time, and
refusing it at compile time reports the same mistake with a position in it.

Module scope is frozen after it initialises, which is what makes it safe to
share across threads. A script that mutates a global from inside a function
fails loudly rather than corrupting memory.

## Threads

The entry point is thread 0. Spawns take 1 upwards, in the order the script made
them, so the same script numbers identically on every run. A handle reports its
own:

```go
handle.Name()     // the function it is running
handle.Thread()   // its thread number
```

## What this does not do

- **Nothing rebuilds a runnable artifact from a bundle.** A bundle is written
  to be shown: a reader decodes it, draws the graph it carries and renders
  status from that. Running comes from the script or the graph.
- **A runaway recursive script will exhaust the stack and take the process
  down.** Recursion is enabled and nothing bounds a run. A panicking builtin is
  recovered and becomes an error; a stack overflow is not recoverable in Go.
- **Nothing executes a graph directly.** A graph becomes a script and the
  script runs; an interpreter that walks the graph itself is the brief's
  largest unbuilt item.
- **Nothing persists across process restart**, and nothing talks to a remote.

## Licence

MIT. See [LICENSE](LICENSE).
