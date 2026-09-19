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
```

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
| `repeat(fn, n)` | Returns a callable that calls `fn` exactly `n` times. |
| `retry(fn, n)` | Returns a callable that calls `fn` until one attempt succeeds. Retries **only** assertions. |
| `timeout(fn, seconds)` | Returns a callable that gives `fn` a limited time. |
| `n()` | The 1-based attempt number, inside `repeat` or `retry`. |
| `load(path, name)` | Binds a name from another script. |
| `json` | `json.encode`, `json.decode`, and the rest of the module go.starlark.net ships. |
| `state` | `state.set`, `state.get`, `state.update` — see below. |
| `time`, `math` | go.starlark.net's own modules. |
| `jsonpath` | `patch_json`, `extract_json`, `match_json`, `len_json`, `find_key`. |

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
)
```

`cmd/lark` imports all of them, so every sample can use them.

### Repeating, retrying and bounding

Each is a **factory**: it takes a function and returns a callable, and nothing
happens until that callable is called.

```python
three_times = repeat(step, 3)      # builds it
three_times()                      # runs it
```

| | |
| --- | --- |
| `repeat(fn, n)` | exactly `n` calls, stopping at the first error, returning the last success |
| `retry(fn, n)` | up to `n` calls, returning the first success |
| `timeout(fn, seconds)` | one call, failing if it runs past the limit |

**`retry` retries only assertions.** A `fail()`, a cancelled handle or a refused
builtin propagates at once. That is what the two failure kinds are for: `assert`
says a check did not hold, which may be timing; `fail` says this cannot work.

An assertion normally ends the whole run — inside a `retry` it ends only that
attempt, because the thread the attempt runs on is marked as catching. When
every attempt has failed, the last assertion propagates and ends the run exactly
as a bare assertion would.

**`n()` is a call, not a variable.** A predeclared name is one value shared by
every thread and resolved when the script is compiled, so it cannot know which
attempt is asking. A function can, because it is handed the calling thread.
Outside a `repeat` or `retry` it is an error rather than a guess.

**Each attempt runs on its own goroutine and interpreter thread**, so two
wrappers running in parallel never share an attempt count, and `timeout` can
stop waiting for one.

A lambda is refused by all three — they name the function they wrap, in an error
and in whatever a graph records — and `n` must be at least 1.

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
```

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
| `runtime.ErrNotAName` | `spawn` got something other than a named zero-argument function |
| `runtime.ErrConflict` | Two plugins supply one name |

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

**Registration is process-wide.** A plugin installs its names for every script
compiled in the binary, including by code that never asked for it.

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

- **Nothing reads a saved artifact back.** `Save` hands out what a container
  format needs — per unit a name, its compiled code, and what each `load`
  spelling resolved to — but there is no `Restore` yet.
- **A runaway recursive script will exhaust the stack and take the process
  down.** Recursion is enabled and nothing bounds a run. A panicking builtin is
  recovered and becomes an error; a stack overflow is not recoverable in Go.
- **No sleep, timeout or retry builtins.** A script cannot wait.
- **Nothing persists across process restart**, and nothing talks to a remote.

## Licence

MIT. See [LICENSE](LICENSE).
