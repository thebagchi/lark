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
lark -s script.star -a '{"host": "db"}'  supply the arguments it declares
lark -s script.star -m 64  run it with 64MB of memory to use
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
| `args.star` | `arg` — what a run supplies, and what it defaults to |

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
| `time` | go.starlark.net's own module. |
| `math` | The usual functions, plus `inf`, `nan`, `tau`, `trunc`, `isnan`, `isinf`, `log2`, `log10`, `gcd`. |
| `regexp` | `search`, `match`, `findall`, `sub`, `split`, `quote`. RE2, so no backtracking. |
| `random` | `seed`, `int`, `float`, `choice`, `shuffle`, `bytes`. Seedable per run. |
| `path` | `join`, `dir`, `base`, `ext`, `stem`, `split`, `parts`, `clean`, `isabs`. Text only. |
| `file` | `read`, `bytes`, `write`, `append`, `exists`, `remove`, `list`, `size`, `mkdir`. **Reaches the disk.** |
| `jsonpath` | `patch_json`, `extract_json`, `match_json`, `len_json`, `find_key`. |
| `codec` | Twelve conversions between bytes, hex, bits and ints, plus `crc32`. See below. |
| `base64` | `base64.encode` / `decode`, and `urlencode` / `urldecode` for the URL-safe alphabet. |
| `base32` | `base32.encode` / `decode`, RFC 4648. |
| `hash` | `hash.md5`, `sha1`, `sha256`, `sha512` and `hash.hmac(algorithm, key, data)`, as hex. |
| `utils` | `utils.datetime()`, the local time to the microsecond, as a string. |
| `arg(name, default)` | Declares an argument this run supplies. Module level only. |

`spawn` takes a **named function that takes no arguments** — not a lambda, not a
call. A closure over what it needs is how a script passes data in. The rule
exists so every thread has a name to report.

### Arguments a run supplies

A script declares what it takes at module level, with a default or without one:

```python
server = arg("host", "localhost")
port = arg("port", 8080)
token = arg("token")

def main():
    print("%s:%d" % (server, port))
```

```
lark -s connect.star -a '{"host": "db.internal", "token": "t-123"}'
```

The name a caller supplies and the name the script binds are **independent** —
`"host"` arrives, `server` is what this script calls it. Values are JSON, so a
string, a number, a boolean, `null`, an array or an object; a whole number
arrives as an `int`, because Starlark has two number types where JSON has one.

**An argument with no default must be supplied.** Nothing supplies `token`
above and the run stops at the declaration:

```
script failed: initialise connect.star: token: argument not supplied and has no default
```

**A declaration is module level only.** `arg()` inside a function body is
refused, because the thread running a body is not the thread a run binds its
arguments on — such a call could only ever take its default, silently.

**Every run initialises the script itself**, which is what lets two runs of one
compiled artifact take different arguments. A module-level statement therefore
runs once per run, not once per compile — so a script can check what it was
given, at module level, before anything else happens:

```python
port = arg("port", 8080)

assert(port > 0, "port must be positive")

def main():
    print("listening on", port)
```

```
lark -s serve.star -a '{"port": 5432}'    listening on 5432
lark -s serve.star -a '{"port": -1}'      script failed: "port must be positive": assertion failed
```

One artifact, two runs, and the second does no work at all. `fail()` reads the
same way. A module-level failure is a run's failure, not the compile's.

A graph carries the declarations in a field of its own, so a host can ask what a
workflow takes without reading its source:

```json
"args": {
  "server": {"name": "host", "default": "localhost"},
  "token":  {"name": "token"}
}
```

Generating a script from a graph writes those declarations back, so the two
forms round-trip.

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
nothing is stored yet — `lambda n: 1 if n == None else n + 1` is the shape that
handles the first write.

`set` takes the same lock, so a write that lands while an update's function is
running is not overwritten by what that update read before it.

### A store holds data

It is the script's cache, and a cache holds data: `None`, a bool, a number, a
string, bytes, and the containers of those. **A function or a thread handle is
refused**, and refused all the way down — a list with a function in it is no
more storable than the function.

```python
state.set("k", helper)             # refused before the script runs
state.set("k", [1, pick()])        # refused when it runs
state.update("k", lambda v: helper)
state.set("k", spawn(worker))

state.set("k", [1, {"a": (2, 3)}, None, b"x"])   # fine
```

Neither is useful and both read as though they worked. A stored function is the
same frozen code the script already had. A handle names a thread, and there is
no way for another thread to do anything with it — waiting on one from inside
the store's own locking would defeat the point of the store.

**Where the source shows it, the refusal happens at compile time**, with the
position:

```
state.set at build.star:5:14 stores the function helper: not data a store can hold
```

A name the file declares, or a lambda written in place, is certain before
anything runs. Everything else — a call's result, what an update's function
returns — is refused when it runs.

**Storing something else stops the whole run**, the way a failed assertion
does, rather than only the thread that did it. A thread nobody joins fails
silently: its error reaches the report and never becomes the run's result. So a
spawned worker storing a function would otherwise leave the store empty, say
nothing, and let the script finish as though it had worked.

**Three things are refused while a name is held**, all with `ErrNested`, and all
because the alternative is a wait nothing in the script can end:

| Refused | Why |
| --- | --- |
| A second `update` | Two threads updating two names in opposite orders would wait on each other forever, and a script cannot be asked to take locks in an order it cannot see |
| A `set`, from inside an update | The same lock, so the same deadlock |
| A `join` | An update takes the name before calling its function, so joining a thread that needs that name leaves each waiting for the other |

**A thread spawned inside an update carries the ban for its whole life** — it
cannot `set` or `update` even after the update has returned, and not for any
name. It copied the fact at birth and nothing clears a copy. That is deliberate:
clearing it on release would make the same script succeed or fail depending on
which side of the release its write happened to land. A thread that needs to
lock is spawned before the update, not inside it.

Joining *after* the update has returned is fine. The refusal is about a name
being held, not about the handle.

Each of these is a plugin, and a host enables it by importing it:

```go
import (
    _ "github.com/thebagchi/lark/runtime/plugin/state"
    _ "github.com/thebagchi/lark/runtime/plugin/jsonpath"
    _ "github.com/thebagchi/lark/runtime/plugin/time"
    _ "github.com/thebagchi/lark/runtime/plugin/math"
    _ "github.com/thebagchi/lark/runtime/plugin/regexp"
    _ "github.com/thebagchi/lark/runtime/plugin/random"
    _ "github.com/thebagchi/lark/runtime/plugin/codec"
    _ "github.com/thebagchi/lark/runtime/plugin/base64"
    _ "github.com/thebagchi/lark/runtime/plugin/base32"
    _ "github.com/thebagchi/lark/runtime/plugin/hash"
    _ "github.com/thebagchi/lark/runtime/plugin/path"
    _ "github.com/thebagchi/lark/runtime/plugin/file"   // reaches the disk
    _ "github.com/thebagchi/lark/runtime/plugin/utils"
)
```

`cmd/lark` imports all of them, so every sample can use them. **A host that
runs scripts it did not write should think about `file` before importing it** —
see below.

### Regular expressions

```python
m = regexp.search(r"(?P<user>\w+)@(\w+)", "to bob@corp now")

m.text      # "bob@corp"
m.start     # 3, a byte offset
m.end       # 11
m.groups    # ["bob", "corp"]
m.named     # {"user": "bob"}
```

`search` looks anywhere, `match` only at the start, and both give `None` when
nothing matched — so `if regexp.search(...)` reads the way it looks.
`findall` gives a list of those same matches. A group that took part in no
match is `None`, not `""`, because they are different answers.

```python
regexp.sub(r"(\w+)@(\w+)", "$2/$1", "bob@corp")   # "corp/bob"
regexp.sub(r"a", "-", "banana", count = 2)        # "b-n-na"
regexp.split(r",\s*", "a, b,c")                   # ["a", "b", "c"]
regexp.quote("a.b*c")                             # escaped, matches itself
```

A replacement names a group **the engine's way** — `$1` and `${name}`, with
`$$` for a literal dollar — not Python's `\1`. One syntax, so nothing is
rewritten on the way through.

**The engine is RE2**, and that is a safety choice rather than a taste one: it
matches in time linear in the subject, so no pattern a script can write makes a
run hang. The price is that **lookahead, lookbehind and backreferences do not
exist** and never will — those are bought with backtracking, which is a denial
of service wearing a feature's clothes in anything that runs scripts it did not
write. A pattern asking for one is refused rather than quietly meaning
something else. Named groups, `(?P<name>...)`, do work.

### Randomness, and what a seed promises

```python
random.seed(42)
random.int(1, 6)         # both ends included
random.float()           # 0.0 up to but not including 1.0
random.choice(items)
random.shuffle(items)    # a new list; the original is untouched
random.bytes(16)
```

The source belongs to **one run**, as `state`'s store does, so two runs of one
artifact draw independently and every thread inside a run draws from the same
stream. Unseeded, it is seeded from `crypto/rand`.

**A seed repeats less than it looks like it does.** A seeded source hands out
one sequence, but which thread receives which number depends on the order the
threads ask — and that order is the scheduler's, not the script's. So a
single-threaded run with a seed repeats exactly; a concurrent one repeats in
the values drawn and not in who drew them. A thread that must repeat gets its
own seed and draws only there.

`shuffle` returns a new list rather than reordering the one it was given,
because module scope freezes before anything concurrent runs — a function that
worked at the top of a script and failed inside a thread would be worse than
one that never reorders in place.

### Paths, and the disk

`path` is text. Nothing in it touches a disk, so every answer is the same
whether the path exists or not.

```python
p = path.join("/srv", "work", "run.log")   # "/srv/work/run.log"
path.dir(p)                                 # "/srv/work"
path.base(p)                                # "run.log"
path.stem(p)                                # "run"
path.ext(p)                                 # ".log"
path.parts(p)                               # ["/", "srv", "work", "run.log"]
```

It uses the **host's own separator**, through `path/filepath`, because these
paths are handed to `file` and then to the operating system — a module that
spelled them its own way would build something the host has to translate, and
the translation is where a path stops meaning what it said. The cost is that
the same script reads a different separator on Windows, so build paths with
`join` rather than with text and it never arises.

`file` reaches the disk:

```python
file.write(p, "written by a script")   # makes the directory above it
file.read(p)                            # as text, regular files only
file.bytes(p)                           # as bytes, same rule
file.append(p, " and more")
file.exists(p)                          # False only when absent
file.list(dir)                          # names, sorted
file.size(p)
file.mkdir(dir)                         # making one twice is fine
file.remove(p)                          # one thing, never a tree

file.stat(p)                            # size, dir, mode, modified - one syscall
for line in file.lines(p):              # one line at a time, read once
    ...
file.writelines(p, ["a", "b"])          # newline between, none after the last
file.appendlines(p, ["c"])              # joins what was already there
```

**Importing `file` is the whole of enabling it, and that is the warning.**
Every other plugin here works on what a script was handed; this one reaches
whatever the host process can reach, with the host's permissions, and a script
may name any path it likes. A host that runs scripts it did not write should
not import it — and because importing is what enables a plugin, leaving the
import out is the whole of leaving it out.

`remove` deletes one file or one empty directory and never a tree, so a
mistyped path costs a refusal rather than an afternoon's work.

**`read` reads files, not devices.** Anything that is not a regular file — a
character device, a pipe, a directory — is refused with `ErrNotAFile`, which
also matches `ErrFile`. Something endless like `/dev/zero` would otherwise grow
until the process died.

**A large file is bounded by what the run may hold, not by a size limit.**
`read` allocates the bytes and copies them into a string, so it costs twice the
file: measured, 512MB of file peaked at 1057MB of memory. It now charges that
against the run's budget *from the stat*, before allocating, so a file the run
cannot afford costs one syscall rather than the process. Nothing here says how
big a file may be — only how much memory a run may use, which is a question the
host already knows the answer to.

The default is 256MB. `lark -m 64` sets it in megabytes; a host embedding the
runtime calls `scheduler.Allowing(ctx, ceiling)` on the context it passes to
`Run`. What a read reserves is given back as the value is handed over, so a
loop reading a thousand files is bounded by the largest of them rather than by
their sum — measured, a hundred reads of a 1MB file run under a ceiling of 8MB.

**`lines` walks a file a line at a time**, so what it costs is set by the
longest line rather than by the file, and a log far too big to `read` is
ordinary to walk. Measured: 27MB of file walked with the heap never passing
17MB. How long a line may be is whatever the run has left to spend, so a file
of one enormous line is refused for the memory it wanted rather than for
tripping a number nobody chose.

`file.lines(p)` is read **once**, like a Python file object: a second pass
raises `ErrWalked` rather than quietly starting again from the beginning. The
file is named when you call it — a missing file or a directory is refused right
there — but not opened until the loop starts, so naming one and never reading
it leaves nothing open.

**`writelines` puts a newline between lines and none after the last**, so what
comes back out is what went in. `appendlines` repairs that seam when it adds to
a file that does not end in one, because otherwise the first new line would run
onto the end of the old one.

**`stat` answers with everything the syscall read** — `size`, `dir`, `mode`,
`modified` — because `size` and `exists` were each making that call and
discarding the rest, so a script wanting two facts asked twice and could be
told two things that were never true together. `mode` is the permission bits as
a number, the way Python's `stat.S_IMODE` gives them, so `info.mode & 0o700`
tests one and `"%o" % info.mode` prints it. `modified` is a `time` value, ready
for the `time` module without conversion.

**`exists` is `False` only when the path is absent.** A directory the process
may not search answers with an error rather than `False`, because "not there"
and "I cannot tell" are different answers and a script deciding whether to write
would otherwise overwrite something it could not see.

### Two naming styles, and why

`codec`'s twelve conversions are flat and spelled `x2y`: `bytes2hex`,
`hex2bits`, `int2bytes`. The encodings are modules, because `encode` and
`decode` are words a script uses for several things and read better behind the
encoding they belong to — and because a name ending in a digit cannot take the
`2` infix without reading as a number: `base642bytes` is "base 642 bytes" to
anyone who has not been told otherwise.

```python
hex = bytes2hex(data)                   # codec's twelve, flat
token = base64.urlencode(payload)       # unpadded, which is what a JWT carries
data = base64.urldecode(token)          # padded or not, either way
sum = crc32(chunk, sum)                 # continues a checksum already started
mac = hash.hmac("sha256", key, body)    # hex, like every digest here
```

Anything conceptually bytes takes a `str` too, so `base64.encode("foobar")` and
`hash.sha256("abc")` both work. Quoted-printable, uuencode and `crc_hqx` are
deliberately absent: two are email-era formats and the third serves one
obsolete protocol.

`md5` and `sha1` are present and are not for anything a reader must not forge.
A script meeting an old checksum still has to read it, and refusing would only
send its author somewhere worse.

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

supplied, err := runtime.Parsed([]byte(`{"host": "db.internal"}`))
ctx = runtime.WithArgs(ctx, supplied)                       // what this run's arguments are
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
| `runtime.ErrNested` | `update`, `set` or `join` was called while a name was held, on any thread the update started |
| `runtime.ErrConflict` | Two plugins supply one name |
| `runtime.ErrNotObject` | What a run was given as arguments is not a JSON object |
| `state.ErrNotData` | A store was given a function, a handle, or a container holding one |

Every sentinel of every package the facade wraps is here. A plugin you import
yourself keeps its own, and there are more of them now:

| Sentinel | Raised when |
| --- | --- |
| `args.ErrNotSupplied` | A declaration nothing supplied has no default |
| `args.ErrNotDeclaring` | `arg()` was called outside module level |
| `file.ErrFile` | The filesystem refused, wrapping what it said |
| `file.ErrNotAFile` | `read` was given a device, a pipe or a directory. Also matches `ErrFile` |
| `regexp.ErrPattern` | RE2 cannot read that pattern — lookahead, lookbehind, a backreference |
| `random.ErrRange` | A range whose end is below its start, or a negative count |
| `random.ErrEmpty` | A choice from a sequence with nothing in it |
| `hash.ErrAlgorithm` | A keyed digest was asked for under a name this does not know |
| `math.ErrNumber` / `math.ErrBase` | Something that is not a number; a logarithm in base one |
| `base64.ErrEncoded` / `base32.ErrEncoded` | Text that is not that encoding |
| `path.ErrNotAPath` | `join` was given something that is not text |
| `unpack.ErrData` | Something that should be bytes or a string is neither |
| `graph.ErrNotCarried` | Deriving met an `arg()` call it cannot carry |

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

A host embedding the runtime names the file:

```go
run := runtime.Start(ctx, artifact, runtime.WithLog("/var/log/lark/nightly.log"))
```

**You name it, because you know what this run is and the runtime does not.**
Two runs of one artifact are two files when you give them two paths. The
directory is created if it is not there, and a run whose file cannot be opened
fails before its script is evaluated rather than running with its output going
nowhere you asked for.

## Watching a run

`Run` and `Invoke` block until the script is over. A long-running host — a
service with a web interface rather than a command-line tool — wants the other
shape: start the script, get the run back at once, and hold it.

```go
run := runtime.Start(ctx, artifact)   // returns immediately
snap := run.Status()                  // how is it doing
run.Stop()                            // stop it, without waiting
value, err := run.Wait()              // block until it is over
<-run.Done()                          // or select on it beside your own work
```

`Start` returns while the script is still running, whatever it goes on to do.
`Stop` reaches a script that is spinning as readily as one that is asleep: the
interpreter is told between instructions. Both `Stop` and cancelling the
context you passed in are the same stop, and `Wait` then returns
`runtime.ErrCancelled`.

**Nothing holds the run for you.** There is no store, no id to look one up by,
and nothing that forgets on a schedule of its own — what is worth keeping about
a finished run, and for how long, is your decision. A run answers `Status` for
as long as you hold it, and is collected when you let go.

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

**A finished run is kept for exactly as long as you hold it.** Nothing forgets
it on a schedule: `Status` does not consume it and neither does `Wait`. Let go
of it and it is collected like anything else. A host that wants runs to outlive
its hold on them keeps what it was told — see below.

Giving up on a run leaves it untouched. `Wait` blocks until the run is over and
takes no context of its own; a caller that would rather not block selects on
`Done()` instead, and neither stops anything. Stopping is `Stop`, or the
context passed to `Start`.

### Saying what has not happened yet

Without a graph, a report says what the run **did**: a function nothing called
never appears. Hand one over and every function it declares starts `PENDING`,
so a report can also say what was not reached.

```go
run := runtime.Start(ctx, artifact, runtime.WithGraph(graph))
```

A graph is a **floor, not a ceiling**. It may add functions and threads that
have not happened; it may never remove, rename or hide one that did. A run that
spawns something the graph never mentions is reported anyway, beside the pending
ones — when a graph and a run disagree, both are visible and the run wins.

A function left pending after the run ended is not an error. It means the run
finished without going there.

### Hearing about every change

`Status` is a question you ask. A `Watcher` is told, each time a function
changes status, without being asked:

```go
type Watcher interface {
    Changed(change *runtime.Change, whole func() *runtime.Workflow)
}

ctx = runtime.WithWatcher(ctx, mine)
value, err := artifact.Run(ctx)
```

A `Change` carries the thread and the node as it now stands, and costs nothing
to deliver. The **whole run arrives as a function, not a value**, because
assembling it copies every node of every thread: on a run of 1,218 changes,
ignoring it costs 21.6 ms and calling it every time costs 200 ms. A host that
logs changes pays nothing; a host drawing a user interface pays where it
chooses to.

Two things are yours to handle. `Changed` runs on the goroutine of the thread
that changed, so a watcher that blocks holds up the script — and a run with
several threads calls it from several goroutines at once, so a watcher that
keeps anything needs a lock of its own.

This is how a host keeps what it wants: the runtime reports, and you decide
what to remember.

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
