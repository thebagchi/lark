# lark

lark embeds [Starlark](https://github.com/bazelbuild/starlark) in Go, with
concurrency.

A script and all the modules that it loads compile into one artifact. Each
function runs on its own goroutine and interpreter thread. The script starts
this work, waits for it and stops it:

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
./bin/lark.bin -s samples/concurrent.star -t   # the flow it describes, as JSON
```

A script and the flow that it describes are two forms of one workflow. `lark`
can run each form, and it can translate each form into the other:

```
lark -s script.star        run the script
lark -s script.star -t     print the flow it describes, as JSON
lark -g flow.json          run the flow
lark -g flow.json -t       print the Starlark it generates
lark -s script.star -l dir keep a log in dir/script.star.log
lark -s script.star -b out.bin  compile into a bundle instead of running
lark -s script.star -a '{"host": "db"}'  supply the arguments it declares
lark -s script.star -m 64  run it with 64MB of memory to use
```

The two translations are inverses. Thus this command prints the same output as
the first command in the example above:

```
lark -s samples/concurrent.star -t > flow.json && lark -g flow.json
```

A flow names no modules. When lark derives a flow, it puts the code of the
loaded modules into the flow, so the flow is complete. lark checks a flow before
it generates a script from it. If a flow cannot run, lark refuses it and gives
the reason.

```
"HELLO WORLD"
```

A call in a flow passes its arguments as values when all of them are literals.
When one or more of them is a name, it passes them as operands:

```json
{"spawn": {"binding": "conn",
           "call": {"function": "connect", "operands": [{"name": "host"}]}}}
```

lark writes `"args": ["db"]` into the script without a change. `{"name":
"host"}` passes the value that `host` has at the call. `host` can be one of
these:

- A parameter of the function that makes the call.
- The result or the binding of an earlier statement in the same list.
- A function, a constant or an argument that the flow declares.

The check refuses a name that the call cannot see. The spawn above generates
`conn = spawn(lambda: connect(host))`. lark adds the lambda because the call has
arguments.

A spawn binds the thread that it starts to a name. A join or a cancel names a
binding from an earlier statement in its own list. The check refuses all other
names.

You can pass a binding to a call as an operand. But a function that joins a
handle that it receives, for example `def watch(h): join(h)`, keeps its text
when lark derives a flow. The flow cannot tell which thread that handle is. A
flow also does not tell which thread id a spawn gets. The run gives the ids, and
a reader matches `join(conn)` to its thread through the binding.

`samples/` has one script for each idea. A comment in each script tells its
purpose:

| Script | Shows |
| --- | --- |
| `hello.star` | the smallest script that lark runs |
| `strings.star` | a library, with no `main` of its own |
| `modules.star` | `load`, which finds a module beside the file that loads it |
| `concurrent.star` | `spawn` and `join` |
| `failfast.star` | a failed `assert` that stops the whole run, and a `join` that stops early |
| `failkinds.star` | `assert` against `fail`: change one line and measure the time |
| `state.star` | `state`, which passes data between threads |
| `rcu.star` | read-copy-update: read a copy, change it and publish it |
| `flow.star` | `repeat`, `retry`, `timeout` and `n()` |
| `graph.star` | a `workflow.Flow` as JSON, and the Starlark that lark generates from it |
| `pointers.star` | RFC 6901: `extract_json`, `match_json`, `len_json`, `find_key` |
| `patch.star` | RFC 6902: `patch_json`, which does not change its input |
| `clock.star` | `time`: durations and instants |
| `numbers.star` | `math`, and the values that it returns |
| `cancel.star` | `cancel`, and the result of a join of a cancelled handle |
| `encode.star` | the `json` plugin |
| `args.star` | `arg`: the values that a run supplies, and the defaults |

Four scripts exit with a status that is not zero, on purpose: `cancel.star`,
`failfast.star`, `failkinds.star` and `strings.star`. They show how a failure
looks, because that is part of the interface.

## Install

```
go get github.com/thebagchi/lark
```

You must have Go 1.26 or newer.

## Run a script

A host imports one package and writes a loader. The runtime uses the loader to
get each module that a script asks for. A loader can get modules from a
directory, an archive, a database or a map in memory. The runtime works the same
with each of them.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/thebagchi/lark/v1/runtime"
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

	value, err := runtime.Run(context.Background(), &runtime.Source{
		Entry:  "greet.star",
		Text:   src,
		Loader: loader,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
}
```

If a `Source` has no `Loader`, a module is a file beside the file that loads it.
For example, `load("util.star", …)` in `scripts/greet.star` reads
`scripts/util.star`.

## What a script gets

| Name | What it does |
| --- | --- |
| `spawn(fn)` | Runs `fn` on its own goroutine and interpreter thread, and returns a handle. `spawn(lambda: f(x))` passes the value that `x` has when `spawn` is called. |
| `join(h, …)` | Waits for each handle. Returns the result of each handle, in the given order. |
| `cancel(h, …)` | Stops each handle. Does not wait. |
| `assert(cond, msg)` | Stops the **whole run** when `cond` is false. `assert(msg = "...")` always stops it. |
| `fail(msg, …)` | The function of Starlark itself. Stops the **thread that calls it**. |
| `sleep(ms)` | Waits for that number of milliseconds. A cancel ends the wait early. |
| `repeat(n, fn, delay)` | Calls `fn` exactly `n` times and returns the last result. With `delay`, it waits that number of milliseconds between calls. |
| `retry(n, fn, delay)` | Calls `fn` until one attempt succeeds. With `delay`, it waits that number of milliseconds between attempts. Retries **only** assertions. |
| `timeout(ms, fn)` | Calls `fn` with a time limit, and fails if `fn` does not end in that time. |
| `n()` | The number of the attempt, from 1, in `repeat` or `retry`. |
| `load(path, name)` | Binds a name from another script. |
| `json` | `json.encode`, `json.decode`, and the other functions of the module that go.starlark.net supplies. |
| `state` | `state.set`, `state.get`, `state.update`. See below. |
| `event` | `event.post(name, value)` and `value, err = event.wait(name, ms)`: one thread waits for another. See below. |
| `time` | The module of go.starlark.net. |
| `math` | The usual functions, and also `inf`, `nan`, `tau`, `trunc`, `isnan`, `isinf`, `log2`, `log10`, `gcd`. |
| `regexp` | `search`, `match`, `findall`, `sub`, `split`, `quote`. It uses RE2, so it does not backtrack. |
| `random` | `seed`, `int`, `float`, `choice`, `shuffle`, `bytes`. You can set the seed for each run. |
| `path` | `join`, `dir`, `base`, `ext`, `stem`, `split`, `parts`, `clean`, `isabs`. These work on text only. |
| `file` | `read`, `bytes`, `write`, `append`, `exists`, `remove`, `list`, `size`, `mkdir`. It **can read and write the disk**, so a script has it only when its host gives it. `lark` gives it. |
| `jsonpath` | `patch_json`, `extract_json`, `match_json`, `len_json`, `find_key`. |
| `hex` | `hex.from_bytes`, `to_bytes`, `from_int`, `to_int`, `from_binary` and `to_binary`. It writes upper case. It reads either case, with or without a `0x` prefix. See below. |
| `binary` | `binary.from_bytes`, `to_bytes`, `from_int` and `to_int`: text of 0 and 1, with the most significant bit first. It reads text with or without a `0b` prefix. See below. |
| `integer` | `integer.from_bytes` and `to_bytes`: an integer and its bytes, unsigned unless `signed = True` is given, and big-endian unless `"little"` is given. See below. |
| `buf` | `buf.new()` makes a buffer to write bytes into, and `buf.reader(data)` makes a reader to take bytes apart. See below. |
| `base64` | `base64.encode` and `decode`, and `urlencode` and `urldecode` for the alphabet that is safe in a URL. |
| `base32` | `base32.encode` and `decode`, as RFC 4648 sets them. |
| `hash` | `hash.md5`, `sha1`, `sha256`, `sha512` and `hash.hmac(algorithm, key, data)`, as upper-case hex. Also `hash.crc32(data, seed)`, as an integer. |
| `utils` | `utils.datetime()`: the local time, to the microsecond, as a string. |
| `arg(name, default)` | Declares an argument that the run supplies. Use it at module level only. |

**All durations are in milliseconds**: `sleep`, `timeout`, the delay of `repeat`
and `retry`, and `event.wait`. `sleep(1000)` is one second. A duration must be a
whole number that fits in an `int32`, so that a flow can carry it without a
change. lark refuses all other values with `ERR_DURATION`.

### Give a thread what it needs

`spawn` calls its function with no arguments. To give arguments to a thread,
write a lambda in the call. **The lambda takes the values when `spawn` is
called**, not when the thread starts to run:

```python
handles = []

for word in ["hello", "world"]:
    handles.append(spawn(lambda: shout(word)))    # each thread gets its own word

parts = join(*handles)                            # ["HELLO", "WORLD"]
```

The compiler changes each variable that such a lambda reads into a parameter,
with the variable as its default. Thus the loop above runs `spawn(lambda
word=word: shout(word))`. A default gets its value where the lambda is written.
Before this change, each thread read the one `word` that the loop continued to
write. Before 2026-09-29, this example printed `WORLD WORLD` in 20 of 20 runs.
The race detector also found two goroutines in a race on the variable.

**The values that a thread gets are frozen.** They are frozen in the thread and
in the function that gave them, and they stay frozen after `join`. A write from
either side fails with an error, and does not cause a race:

```python
items = []
spawn(lambda: count(items))
items.append(1)            # cannot append to frozen list
```

You can bind a name to a new value. You cannot change the value that you gave to
the thread. A thread gives back its results when it returns. Use `state` for all
other data that threads share.

**lark refuses, at the spawn, all other ways to carry a local variable into a
thread.** The error is `ERR_CAPTURES`, and it names the variable:

| Refused | Why | Write this |
| --- | --- | --- |
| `spawn(worker)`, where `worker` is a nested function that reads a local variable | a function that is spawned by name reads the variable when it runs | a top-level function, through a lambda: `spawn(lambda: worker(x))` |
| `job = lambda: f(x)` and then `spawn(job)` | only a lambda that is written in the call takes its variables | `spawn(lambda: f(x))` |
| `spawn(lambda: call(read_y))`, where `read_y` reads `y` | the thread gets the closure, and the closure still reads `y` from the function that made it | give `y` itself to the thread |

A top-level function does not need this. It can capture only what its file
loaded, and that is bound one time and frozen. Thus `spawn(worker)` works as
before. A lambda that is not in `spawn(...)` has its usual meaning. This is also
true for `retry(3, lambda: fetch(url))`, because a wrapper waits for its call.

### Arguments a run supplies

A script declares its arguments at module level, with or without a default:

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

The name that a caller supplies and the name that the script binds are
**independent**. The caller supplies `"host"`, and this script calls it
`server`. Values are JSON: a string, a number, a boolean, `null`, an array or an
object. A whole number arrives as an `int`, because Starlark has two number
types and JSON has one.

**You must supply an argument that has no default.** In the example above,
nothing supplies `token`, so the run stops at the declaration:

```
script failed: initialise connect.star: token: argument not supplied and has no default
```

**Declare arguments at module level only.** lark refuses `arg()` in a function
body. The thread that runs a body is not the thread on which a run binds its
arguments. Such a call could only take its default, and nothing would tell you.

**Each run initialises the script itself.** Thus two runs of one compiled
artifact can take different arguments. A module-level statement runs one time
for each run, not one time for each compile. Thus a script can check its
arguments at module level, before anything else occurs:

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

One artifact gives two runs, and the second run does no work. `fail()` works the
same way. A failure at module level is a failure of the run, not of the compile.

A flow keeps the declarations in a field of their own. Thus a host can find the
arguments of a workflow without its source:

```json
"args": {
  "server": {"name": "host", "default": "localhost"},
  "token":  {"name": "token"}
}
```

When lark generates a script from a flow, it writes the declarations back. Thus
each form translates to the other and back without a change.

### Share data between threads

The module scope is frozen before any concurrent work starts. Thus a script
cannot share data through an assignment to a global. Threads use `state` to pass
data to each other:

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
stores, and all threads in a run get the same store. A read of a name that
nothing wrote gives `None`, because that is the usual case in a store that
threads share.

**The store works like read-copy-update.** The stored values are frozen. Thus
threads that read one name at the same time never get a value that another
thread can change. Thus `state.get` returns a **copy**. The script owns the copy
and can change it. Other threads cannot see these changes until the script
publishes the result.

```python
mine = state.get("findings")   # read — a copy of the published value
mine.append("two")             # copy — nobody else can see this yet
state.set("findings", mine)    # update — now they can
```

A copy takes time in proportion to the size of the value, on each read. Thus
read a large structure one time, not in a loop.

A copy keeps shared values shared, and it is safe with cycles. A list that you
can reach by two paths stays one list. A list that contains itself copies
without an endless loop.

**Use `state.update` when other threads write the same name.** A `get` and then
a `set` are two operations. If another thread writes between them, one of the
updates is lost. This occurs without an error, and only under load:

```python
state.update("count", lambda n: n + 1)
```

Four threads add one to one name, two hundred times each:

| | Result |
| --- | --- |
| `state.set(n, state.get(n) + 1)` | 294, 575, 317, 644, 442: a different number on each run |
| `state.update(n, lambda v: v + 1)` | 800, on each run |

The lock is a Go lock, and it covers the read, the call and the write. A script
never sees the lock and cannot forget to release it. When nothing is stored yet,
lark calls the function with `None`. Use `lambda n: 1 if n == None else n + 1`
to handle the first write.

`set` takes the same lock. Thus a `set` that occurs while the function of an
update runs waits for the update. The update cannot write over it with an old
value.

### A store holds data

The store is the cache of the script, and a cache holds data: `None`, a bool, a
number, a string, bytes, and containers of these. **lark refuses a function or a
thread handle** at all depths. You cannot store a list that contains a function,
as you cannot store the function.

```python
state.set("k", helper)             # refused before the script runs
state.set("k", [1, pick()])        # refused when it runs
state.update("k", lambda v: helper)
state.set("k", spawn(worker))

state.set("k", [1, {"a": (2, 3)}, None, b"x"])   # fine
```

Neither is useful, and both look as if they worked. A stored function is the
same frozen code that the script already has. A handle names a thread, and
another thread cannot do anything with it. A wait on a handle in the lock of the
store would make the store useless.

**When the source shows the value, lark refuses it at compile time**, and gives
the position:

```
state.set at build.star:5:14 stores the function helper: not data a store can hold
```

A name that the file declares, or a lambda that is written in place, is known
before anything runs. lark refuses all other values when they run, for example
the result of a call, or the value that the function of an update returns.

**A store of a value that is not data stops the whole run**, as a failed
assertion does. It does not stop only the thread that tried it. A thread that
nothing joins fails without a message: its error goes into the report, but it
never becomes the result of the run. Thus, without this rule, a spawned worker
that stores a function would leave the store empty. The script would end as if
it worked.

**lark refuses three things while a name is held.** All three give `ERR_NESTED`,
because each would cause a wait that nothing in the script can end:

| Refused | Why |
| --- | --- |
| A second `update` | Two threads that update two names in opposite orders would wait for each other forever. A script cannot take locks in an order that it cannot see |
| A `set`, in an update | It uses the same lock, so the result is the same deadlock |
| A `join` | An update takes the name before it calls its function. A join of a thread that needs that name makes each thread wait for the other |

**A thread that is spawned in an update keeps the ban for all of its life.** It
cannot `set` or `update` any name, even after the update returns. The thread
copied the ban when it started, and nothing clears a copy.

This is intentional. If lark cleared the ban at the release, one script could
succeed on one run and fail on the next. The result would depend on when its
write occurred. If a thread must lock, spawn it before the update, not in it.

A join *after* the update returns is correct. lark refuses a join while a name
is held, not because of the handle.

### One thread waits for another

A store lets a thread leave a value where another thread can find it. But the
other thread cannot know *when* the value is there, so it must poll. An event
does the other half of the work. One thread tells that something occurred, and
all threads that wait for it wake.

```python
def loader():
    rows = fetch()
    state.set("rows", rows)
    event.post("loaded", len(rows))

def main():
    held = spawn(lambda: loader())

    count, err = event.wait("loaded", 5000)
    if err:
        fail("the loader never finished: %s" % err)

    join(held)

    return count
```

**`event.wait` gives a pair, `(value, err)`.** `err` is `None` when the event
was posted, and `"timed out"` when the time of the wait ended. It gives a pair
and does not raise an error, because a timeout is an answer that a script acts
on, not a fault. It does not give only the value, because then `event.post("k",
None)` and a wait that timed out would give the same answer.

**An event is a latch: a script posts it one time, and it stays posted.** A
waiter that arrives after the post does not wait, and all waiters see the post.

```python
def main():
    event.post("ready", "done")

    value, err = event.wait("ready", 10000)  # returns immediately

    return value
```

That is why an event is a latch and not a signal. A signal wakes only the
threads that wait at that time. It would lose the post above, and the script
would wait forever for something that already occurred. Such a deadlock depends
on which thread the scheduler ran first, and that makes it very hard to find.

| What occurs | What a script sees |
| --- | --- |
| A post before the wait | The value, immediately |
| A post during the wait | The value, at the time of the post |
| No post | `(None, "timed out")` when the timeout ends |
| Two posts | The whole run stops: `this event has already been posted` |
| A `timeout()` around the wait, or a cancel of the run | The wait ends early: `timeout()` fails as it does around any call, and a cancelled run ends |

**lark refuses a second post.** A latch tells that a thing occurred, and a thing
occurs one time. Two posts are two things with one name, or one thing told two
times. Both are mistakes, and you must know about them.

**An event carries data, and only data.** This is the rule of the store, for the
same reason. Another thread reads the value, and code has no meaning for a
thread that did not write it. lark refuses a function or a handle with `not data
an event can carry`.

**When the source shows the value, lark refuses it before the script runs**, and
gives the position, as it does for the store:

```
event.post at build.star:5:15 posts the function helper: not data an event can carry
```

A function that the file declares, or a lambda that is written in place, is
known before anything runs. lark refuses all other values when the post runs,
for example the result of a call, or a value that the script read.

**Both refusals stop the whole run**, as a failed assertion does and as a
refusal of the store does. They do not stop only the thread that posted. A
thread that nothing joins fails without a message: its error goes into the
report, but never becomes the result of the run. Thus, without this rule, a
spawned worker that posts two times would tell nothing. The thread that waits
for it would see only a timeout.

**A wait obeys its caller.** `timeout(2000, lambda: event.wait("never",
300000))` returns after two seconds, not after three hundred. A cancel of the
run ends all waits in it. Nothing else limits the time of a wait. The timeout
that the script gives is the limit, and it is necessary.

**Both sides use the budget of `-m`.** A post uses what the value holds, because
nothing deletes an event, and the event keeps its value until the run ends. *A
name* of an event also uses 256 bytes and the length of the name. A waiter can
name an event that nobody posts, and that is how a waiter can arrive first.
Without this charge, a script could also make a million channels at no cost.

An event is not a queue. It has no delete, no second post and no count of the
waiters. If a script needs these, use the store and a loop.

All of these but one are plugins of the runtime itself, and each script has all
of them. An import of `runtime` imports each one, and each one registers itself:
`state`, `event`, `jsonpath`, `time`, `math`, `regexp`, `random`, `hex`,
`binary`, `integer`, `buf`, `base64`, `base32`, `hash`, `path`, `utils`, `args`,
`flow` and `core`. **`file` is not one of them, because it can read and write
the disk.** A script has it only when a host gives it. See below.

### Regular expressions

```python
m = regexp.search(r"(?P<user>\w+)@(\w+)", "to bob@corp now")

m.text      # "bob@corp"
m.start     # 3, a byte offset
m.end       # 11
m.groups    # ["bob", "corp"]
m.named     # {"user": "bob"}
```

`search` looks in all of the text, and `match` looks only at the start. Both
give `None` when nothing matches, so `if regexp.search(...)` means what it
shows. `findall` gives a list of the same matches. A group that is not in a
match is `None`, not `""`, because these are different answers.

```python
regexp.sub(r"(\w+)@(\w+)", "$2/$1", "bob@corp")   # "corp/bob"
regexp.sub(r"a", "-", "banana", count = 2)        # "b-n-na"
regexp.split(r",\s*", "a, b,c")                   # ["a", "b", "c"]
regexp.quote("a.b*c")                             # escaped, matches itself
```

A replacement names a group **as the engine does**: `$1` and `${name}`, and `$$`
for a dollar sign. It does not use `\1` as Python does. There is one syntax, so
lark does not change a replacement before the engine gets it.

**The engine is RE2**, and lark uses it for safety. It matches in a time that is
linear in the length of the text. Thus no pattern that a script can write can
make a run hang.

Because of this, **lookahead, lookbehind and backreferences are not available**,
and they will not be available. They need backtracking. In a program that runs
scripts from other persons, backtracking lets a script stop the service. lark
refuses a pattern that asks for one of them, and does not silently change its
meaning. Named groups, `(?P<name>...)`, work.

### Random values, and what a seed promises

```python
random.seed(42)
random.int(1, 6)         # both ends included
random.float()           # 0.0 up to but not including 1.0
random.choice(items)
random.shuffle(items)    # a new list; the original is untouched
random.bytes(16)
```

The source belongs to **one run**, as the store of `state` does. Thus two runs
of one artifact get independent values, and all threads in a run get values from
the same stream. If the script sets no seed, lark gets a seed from
`crypto/rand`.

**A seed repeats less than it seems to.** A seeded source gives one sequence.
But which thread gets which number depends on the order of the requests, and the
scheduler sets that order, not the script. Thus a run with one thread and a seed
repeats exactly. A concurrent run repeats the values, but not the threads that
get them. If a thread must repeat, give it its own seed, and let only that
thread take values.

`shuffle` returns a new list. It does not change the order of the list that it
gets, because the module scope freezes before concurrent work starts. A function
that works at the top of a script and fails in a thread is worse than a function
that never changes a list.

### Paths, and the disk

`path` works on text. Nothing in it uses a disk, so each answer is the same if
the path exists or not.

```python
p = path.join("/srv", "work", "run.log")   # "/srv/work/run.log"
path.dir(p)                                 # "/srv/work"
path.base(p)                                # "run.log"
path.stem(p)                                # "run"
path.ext(p)                                 # ".log"
path.parts(p)                               # ["/", "srv", "work", "run.log"]
```

It uses the **separator of the host**, through `path/filepath`, because these
paths go to `file` and then to the operating system. If the module used its own
separator, the host would have to translate each path, and a translation can
change what a path means. As a result, the same script sees a different
separator on Windows. Make paths with `join`, not with text, and this does not
occur.

`file` reads and writes the disk:

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

**`file` is the one plugin that a host gives, and that is the warning.** All
other plugins work on what a script got. This plugin can get to all that the
host process can get to, with the permissions of the host. A script can name any
path. Thus it is not a plugin of the runtime itself. A script has it only when
its compile got it from `github.com/thebagchi/lark/v1/plugin/file`:

```go
artifact, err := runtime.Compile(source, runtime.WithPlugins(&file.Plugin{}))
```

A host that runs scripts from other persons does not give it. `lark` gives it to
each script that it runs, because those scripts are yours, and they run with
your permissions.

`remove` deletes one file or one empty directory, and never a tree. Thus an
incorrect path causes a refusal, not the loss of many hours of work.

**`read` reads files, not devices.** lark refuses all that is not a regular
file, for example a character device, a pipe or a directory. The error is
`ERR_NOT_A_FILE`, which also matches `ERR_FILE`. Without this rule, an endless
source such as `/dev/zero` would grow until the process stopped.

**The memory that a run can hold limits a large file. No size limit does.**
`read` allocates the bytes and copies them into a string, so it uses two times
the size of the file. It charges that to the budget of the run *from the stat*,
before it allocates. Thus a file that the run cannot pay for costs one syscall,
not the process.

Nothing here sets the maximum size of a file. Only the memory of a run has a
limit, and the host knows that limit. [What a run can use](#what-a-run-can-use)
gives the ceiling and tells how to set it.

**`lines` reads a file one line at a time.** Thus the longest line sets its
cost, not the file, and you can read a log that is much too large for `read`. In
a measurement, `lines` read a 27MB file, and the heap stayed below 17MB. The
maximum length of a line is the budget that the run has left. Thus lark refuses
a file of one very long line for the memory that it needs, not because of a
number that nobody chose.

You can read `file.lines(p)` **one time**, as a Python file object. A second
pass raises `ERR_WALKED`. It does not silently go back to the start of the file.
lark checks the file when you call `lines`, and refuses a missing file or a
directory there. It opens the file only when the loop starts. Thus, if you name
a file and do not read it, nothing stays open.

**`writelines` puts a newline between lines and none after the last line.** Thus
what you read back is what you wrote. When `appendlines` adds to a file that
does not end in a newline, it adds the newline first. Without this, the first
new line would join the end of the last old line.

**`stat` gives all that the syscall read**: `size`, `dir`, `mode` and
`modified`. Before, `size` and `exists` each made that call and discarded the
rest. A script that needed two facts asked two times, and could get two facts
that were never true at the same time. `mode` is the permission bits as a
number, as `stat.S_IMODE` of Python gives them. Thus `info.mode & 0o700` tests
one bit and `"%o" % info.mode` prints them. `modified` is a `time` value, and
the `time` module can use it without a conversion.

**`exists` is `False` only when the path is absent.** For a directory that the
process cannot search, it gives an error, not `False`. "Not there" and "I cannot
tell" are different answers. Without this rule, a script that decides to write
could write over something that it could not see.

### The names of the conversions, and why

Each conversion is in a module. The name of the module is the form that the
conversion writes or reads: `hex.from_bytes`, `binary.to_int`,
`integer.to_bytes`, `base64.encode`. A script uses `encode`, `from_bytes` and
`to_int` for many things. After the name of their form, these words are clear.
The module for integers is `integer`, because Starlark uses `int` and `bytes` as
the names of its own builtins.

```python
text = hex.from_bytes(data)             # upper case, two digits for each byte
data = hex.to_bytes("00ff")             # either case
bits = binary.from_int(5, 8)            # "00000101": the width counts bits
data = integer.to_bytes(256, 4)         # big-endian: the width counts bytes
token = base64.urlencode(payload)       # unpadded, which is what a JWT carries
data = base64.urldecode(token)          # padded or not, either way
sum = hash.crc32(chunk, sum)            # continues a checksum already started
mac = hash.hmac("sha256", key, body)    # upper-case hex, like every digest here
```

Each function that writes hex writes upper case: the `hex` module and the
digests of `hash`. Each function that reads hex reads either case. It also
accepts a `0x` prefix, in either case. Each function that reads bits accepts a
`0b` prefix, in either case: `binary.to_bytes`, `binary.to_int` and
`hex.from_binary`. To compare a digest with text in lower case, change the text
to upper case first, for example with `text.upper()`.

The width of `hex.from_int` counts hex digits. The width of `binary.from_int`
counts bits. These two functions do not cut a number that is wider than the
width. The width of `integer.to_bytes` counts bytes. `integer.to_bytes` refuses
a number that is wider than its width.

Each function that takes bytes also takes a `str`. Thus
`base64.encode("foobar")` and `hash.sha256("abc")` both work. Quoted-printable,
uuencode and `crc_hqx` are not available, on purpose. Two are formats from the
early years of email, and the third is for one obsolete protocol.

`md5` and `sha1` are available, but do not use them for anything that a reader
must not forge. A script that finds an old checksum must still read it. If lark
refused them, the author would use a worse tool.

### Buffers

Starlark cannot join bytes, and `+=` copies all of the bytes on each write. A
buffer keeps its bytes and adds to them. `buf.new()` makes a buffer. A reader
takes bytes apart in the same order. `buf.reader(data)` makes a reader. The
methods have the names that Go uses: each write starts with `write_`, and each
read starts with `read_`.

| Call | What it does |
| --- | --- |
| `b.write_bytes(data)` | Adds bytes, or the UTF-8 bytes of a `str` |
| `b.write_string(text)` | Adds the UTF-8 bytes of a `str`. It does not take bytes |
| `b.write_uint8(n)` to `b.write_uint64(n)` | Adds `n` as an unsigned integer of 8, 16, 32 or 64 bits |
| `b.write_int8(n)` to `b.write_int64(n)` | Adds `n` in two's complement, so `n` can be negative |
| `b.write_uint(n, width)` | Adds `n` as an unsigned integer of `width` bytes |
| `b.write_int(n, width)` | Adds `n` in two's complement, in `width` bytes |
| `b.bytes()` | Gives all of the bytes in the buffer |
| `len(b)` | Gives the number of bytes in the buffer |
| `r.read_bytes(count)` | Gives the next `count` bytes |
| `r.read_string(count)` | Gives the next `count` bytes as a `str` |
| `r.read_uint8()` to `r.read_uint64()` | Gives the next 1, 2, 4 or 8 bytes as an unsigned integer |
| `r.read_int8()` to `r.read_int64()` | Gives the next 1, 2, 4 or 8 bytes as an integer in two's complement |
| `r.read_uint(width)` | Gives the next `width` bytes as an unsigned integer |
| `r.read_int(width)` | Gives the next `width` bytes as an integer in two's complement |
| `len(r)` | Gives the number of bytes that are left |

```python
b = buf.new()
b.write_bytes(hex.to_bytes("CAFE"))     # a magic number
b.write_uint16(len(body))               # a length, big-endian
b.write_string(body)
b.write_int16(-1, "little")             # little-endian
r = buf.reader(b.bytes())
magic = r.read_bytes(2)
text = r.read_string(r.read_uint16())
```

An integer is big-endian. To use little-endian, give `"little"` as the last
argument. `b.write_uint16(n)` writes the same bytes as `b.write_uint(n, 2)` and
as `integer.to_bytes(n, 2)`. For two's complement, `integer.to_bytes` and
`integer.from_bytes` take `signed = True`.

A read moves the reader past the bytes that it gives. `r[0]` gives the next
byte and does not move the reader. If the reader has fewer bytes than a read
asks for, the read fails with `buf.ERR_SHORT`. A read that fails does not move
the reader.

A buffer and a reader are not data, so the store and events refuse them. A
`spawn` freezes them, because two threads must not change one buffer. After
that, a write or a read fails with `buf.ERR_FROZEN`. `b.bytes()`, `len()` and
an index still work.

### Repeat, retry and limit a call

Each of these calls its function immediately, and gives back the result of the
call.

```python
last = repeat(3, step)             # calls step three times
```

| | |
| --- | --- |
| `repeat(n, fn, delay)` | Exactly `n` calls. It stops at the first error, and returns the last success |
| `retry(n, fn, delay)` | Up to `n` calls. It returns the first success |
| `timeout(ms, fn)` | One call. It fails if the call does not end in the time limit |

The delay is optional. Give it as the third argument, or as `delay = 250`. The
wait occurs only between calls: not before the first call, and not after the
last call. A cancel ends the wait early, and the next call does not start.

You cannot put one of these in another. lark refuses `retry(3, timeout(5000,
fn))`, because a wrapper takes a function and gives back a result, not another
function.

**`retry` tries again only after an assertion.** A `fail()`, a cancelled handle
or a refused builtin goes up immediately. That is the purpose of the two types
of failure. `assert` tells that a check was not true, possibly because of the
timing. `fail` tells that the operation cannot work.

An assertion usually ends the whole run. In a `retry`, it ends only that
attempt, because lark marks the thread of the attempt so that it catches
assertions. When all attempts fail, the last assertion goes up and ends the run,
as an assertion outside a `retry` does.

**`n()` is a call, not a variable.** A plain name belongs to the *module*, and
all threads of a run share the module. A global is an index into one slice of
values, and a predeclared name is a key in one map. A thread cannot have its own
value for either. Thus a plain `n` would be one number for all attempts. Also,
two wrappers that run at the same time would read the values of each other.

A builtin gets the thread that calls it, and that thread keeps the attempt. Thus
`n()` can give the correct number. Outside a `repeat` or a `retry`, `n()` gives
an error, not a guess.

**Each attempt runs on its own goroutine and interpreter thread.** Thus two
wrappers that run in parallel never share a count of attempts, and `timeout` can
stop its wait for one.

The count is first and the function is last, because a lambda is the argument
that most often becomes long. `n` must be 1 or more.

**You can give a lambda.** Write a lambda when a call needs arguments:
`repeat(3, lambda: greet("alice"))`. The compiler reads from the source which
function the lambda calls, and the graph shows the attempts under that name:
here, `greet`.

### `assert` or `fail`?

Both end an evaluation, and a script cannot catch either. Starlark has no `try`,
and it keeps `raise` as a keyword but does not implement it. The two differ in
**how far the failure goes**, and that is all of the choice:

| | `assert(cond, msg)` | `fail(msg, …)` |
| --- | --- | --- |
| Supplied by | this runtime | Starlark itself |
| Takes a condition | yes | no: it always stops |
| Ends | **the whole run**: the spine, each spawned thread, and all that they spawned | **the thread that calls it** |
| Siblings | always cancelled where they are | cancelled only if `join` finds the failure first |
| At a `join` | replaces the result of the join: the run has one outcome | reported first **in argument order** |
| Joined or not | fails the run in both cases | seen only if something joins it |
| Tried again by `retry` | yes | no |

In short: **`assert` means that this run is over. `fail` means that this
operation cannot continue.**

The row for siblings has one more detail. `join` stops at the first failure, and
it cancels the handles that it has not reached. Thus a `fail` *also* stops
siblings when the failed handle is first in the join. The two differ when the
failed handle is last:

```python
join(slow, dies)   # fail:   waits the slow one out, then reports
                   # assert: cancels it immediately
```

`samples/failkinds.star` is that script. The two versions differ by
approximately three orders of magnitude in elapsed time.

```python
def check(value):
    if value < 0:
        fail("negative value:", value)   # this call is wrong; siblings carry on
    return value

def main():
    assert(ready(), "the fixture never came up")   # nothing can be salvaged
```

`fail` takes any number of values and joins them with spaces. Thus `fail("code",
42)` gives `fail: code 42`.

A step that `retry` must try again must use `assert`. A step that must stop in
all cases must use `fail`.

### The three forms of `assert`

```python
assert(x == 1)                  # stops the run when x is not 1
assert(x == 1, "x was wrong")   # the same, with a message
assert(msg = "unreachable")     # always stops it
```

lark **refuses** `assert("some text")`, and does not run it. The refusal also
stops the run. This call looks like the third form, but it would work like the
first, because a string that is not empty is true. Without the refusal, it would
pass without a message. An assertion that has a mistake in it is still an
assertion. A mistake must not silently remove an assertion.

### Failure

**A failed assertion stops the whole run**: the spine, each spawned thread, and
all that they spawned. This occurs if a thread joins the failed handle or not.
The first assertion ends the script, as in a test runner.

```python
def broken():
    assert(False, "the reason")

def main():
    slow = spawn(counts_a_long_way)
    join(spawn(broken), slow)   # raises: "the reason": assertion failed
                                # and slow is cancelled, not waited for
```

**`join` stops at the first failure.** It waits for handles in argument order.
The first failure cancels the handles that it has not reached. Then `join` waits
for these handles, so it does not leave an evaluation that still runs.

There are two types of failure, and they differ in only one way:

| | Ends | Reported |
| --- | --- | --- |
| `assert` | the whole run | the assertion, where it occurred, joined or not |
| a refused `assert("text")` | the whole run | the refusal: an assertion that has a mistake in it is still an assertion |
| all other failures: `fail()`, a cancelled handle, a builtin that panics | the thread where it occurred | the first failure **in argument order** at the join |

Thus, for usual failures, a script reports the same failure on each run, and
this does not depend on which thread lost the race. An assertion is stronger. It
stops the run, and a run has one outcome: the first assertion that lark records.
lark cancels a sibling that would fail later, before it can fail.

An assertion in a thread that **nothing joins** also fails the run. A run does
not report success while lark stops its threads.

## The Go surface

A host imports one package, `github.com/thebagchi/lark/v1/runtime`. The types
that it gives are aliases of the types of its subpackages, so a value goes
through this boundary without a conversion. The import also gives each script
all the plugins of the runtime itself, and these plugins are always there. They
include `spawn`, `join`, `cancel`, `assert`, `sleep`, `args`, `flow`, `state`,
`json`, `math` and `time`.

A host adds its own plugins with `WithPlugins`. That is also how a script gets
the file system: `file` is a plugin that a host gives, and never a plugin of the
runtime itself. A plugin is a separate package. A host imports it to give it to
a script, or to name its errors.

A run tells its host two things, and it pays for nothing else:

- Its graph. `Status` returns all of the graph, and `WithChanges` sends each
  change as a JSON Patch, from `{}`.
- What its script printed. This goes to the logger that the run gets.

```go
value, err := runtime.Run(ctx, &runtime.Source{Entry: "build.star", Text: src})
if err != nil {
	return err
}

log.Printf("main returned %s", value)
```

`Run` compiles and runs in one call. `Compile` and `Start` are its two halves.
Use them when a host gives a script its own plugins, reads the graph of a run,
or stops a run.

### How to read the examples

After each table below, examples use each row of the table. They are written as
a host writes them, to the conventions of this repository, with two changes to
save space:

- A fragment is code from the body of a function that has a `ctx` and returns an
  `error`. A type or a plugin is shown complete.
- Doc comments have no revision history.

The fragments share `built`, the artifact that lark compiled from `build.star`,
and these constants:

```go
const (
	// SCRIPT is the script the examples compile, and STEPS the module it loads.
	SCRIPT = "build.star"
	STEPS  = "steps.star"

	// LOG_FILE is the file a run's printed lines are logged to, BUNDLE the file
	// a saved artifact is written to, and BUNDLE_MODE that file's permissions.
	LOG_FILE    = "build.log"
	BUNDLE      = "build.bundle"
	BUNDLE_MODE = 0o640

	// LIMIT is how long a started run may take before the host stops it, and
	// EVERY how often the host reports on a run that is still going.
	LIMIT = 30 * time.Second
	EVERY = time.Second
)
```

`build.star` loads one function from `steps.star`:

```python
load("steps.star", "fetch")

def main():
    fetch("lark")
    print("built")
    return "ok"
```

```python
def fetch(name):
    print("fetching " + name)
    return name
```

The output beside an example is what it printed when it ran.

### Functions

| Signature | Purpose |
| --- | --- |
| `func Run(ctx context.Context, source *Source, opts ...RunOption) (starlark.Value, error)` | Compiles a script with the plugins of the runtime, runs its `main` and returns what `main` returned. It does `Compile`, `Start` and `Wait` in one call. It takes the options of the run, as `Start` does. |
| `func Compile(source *Source, opts ...CompileOption) (*Artifact, error)` | Compiles a script and all the modules that it loads into one artifact, before any top-level statement runs. The modules come from the `Loader` of the source. If the source names no loader, they are files beside the script. A script sees all the plugins of the runtime itself. |
| `func Load(bundle []byte, opts ...CompileOption) (*Artifact, error)` | Reads back a bundle that `Save` wrote, so a host can compile one time and run in a different place. It takes the options that the compile took, because compiled code names the plugins that it calls. |
| `func WithPlugins(plugins ...Plugin) CompileOption` | Gives a script plugins of the host, for example `file`. Their names are added to the plugins of the runtime itself. It takes all the plugins at one time, and a second `WithPlugins` replaces the first. Two compiles in one process can see different plugins of the host. When a compile builds its environment, it refuses a name that another plugin also supplies. |
| `func Derive(source *Source) (*Flow, error)` | Reads a script, and all the modules that it loads, into the flow that it describes. It is the reverse of `Emit`. A function that it cannot model goes into the flow as the text of its body, so a flow is the whole script. It refuses what it cannot carry. |
| `func Check(flow *Flow) error` | Refuses a flow that cannot become the script that it describes. For example, a flow declares a name two times, or names something that does not exist. A call passes arguments that the function does not take, or a flow joins a thread where it did not spawn it. `Emit` runs it first. A host calls it to refuse a flow when the flow arrives. |
| `func Emit(flow *Flow) ([]byte, error)` | Gives the Starlark script that a flow describes, ready for `Compile`. It checks the flow first, and refuses it with what `Check` returns. It does not compile what it writes. |
| `func Start(ctx context.Context, art *Artifact, opts ...RunOption) *Execution` | Runs the `main` of an artifact on its own goroutine, and returns the run immediately. The `Status` of the run is its graph, and `Wait` gives what `main` returned. It is the one way to run, and a cancel of `ctx` is the one way for a host to stop the run. Nothing keeps the run for the host. |
| `func WithMemory(bytes int64) RunOption` | An option of a run: the maximum memory that this runtime allocates for its script, for what it reads, stores, copies and spawns. The default is 256MB. It does not limit the allocations of the interpreter itself. |
| `func WithArgs(args map[string]any) RunOption` | An option of a run: the values that the `arg()` declarations of its script take. Each entry of `args` is the value of the argument with that name. Each value must be a value that JSON can hold. If not, the run fails with `ERR_NOT_JSON` when it starts. |
| `func WithChanges(watch func(change *Change)) RunOption` | An option of a run: the function that gets each change to the graph of the run, as a JSON Patch. The first change makes an empty graph, `{}`, into a graph whose status is `STATUS_RUNNING`. lark calls it for one change at a time, in order, on the goroutine of the thread whose step made the change. |
| `func WithLog(w io.Writer) RunOption` | An option of a run: the writer that gets what its script prints, one line for each print. It writes each line as the `lark` command does: the time, the level, the thread and the function, and then the message. |
| `func WithLogger(logger *slog.Logger) RunOption` | An option of a run: the logger that gets what its script prints, one record for each line. The line is the message, and the record has `thread` and `function` attributes. The handler writes the time and the layout. Without a logger, the default logger of slog gets the lines. |

A host gives the options of a run to `Start`, for one run at a time. Each option
has the same meaning on each run. The context carries only the cancellation.

#### `Compile`, `Load`

A script names its modules itself. If there is no `Loader`, a module is a file
beside the script that loads it. Thus lark reads `steps.star` from beside
`build.star`:

```go
src, err := os.ReadFile(SCRIPT)
if err != nil {
	return err
}

built, err := runtime.Compile(&runtime.Source{Entry: SCRIPT, Text: src})
if err != nil {
	return err
}
```

With a loader, the modules come from where the loader keeps them. `_Memory`, the
loader in [Loader](#loader), keeps them in a map. `build` and `steps` hold the
source of the two scripts:

```go
modules := &_Memory{
	files: map[string][]byte{
		SCRIPT: build,
		STEPS:  steps,
	},
}

built, err := runtime.Compile(&runtime.Source{
	Entry:  SCRIPT,
	Text:   build,
	Loader: modules,
})

// A ring of loads, or a call with the wrong arguments, is the script's to fix,
// and the error says where.
wrong := errors.Is(err, runtime.ERR_CYCLE) || errors.Is(err, runtime.ERR_ARITY)
if wrong {
	return fmt.Errorf("fix %s: %w", SCRIPT, err)
}

if err != nil {
	return err
}
```

| Refused | Error |
| --- | --- |
| `steps.star` loads `build.star` again | `cycle in the load graph: build.star -> steps.star -> build.star` |
| `fetch("lark", "now")` | `build.star:4:5: call of fetch: a call passes arguments the function does not take` |
| a `steps.star` that the loader cannot find | `cannot load "steps.star" from build.star: module steps.star: file does not exist` |

`Compile` builds the whole load graph **before anything runs**. It refuses a
ring of loads before one top-level statement runs. It gets each module one time
for each compile, however many scripts load it.

A compile does not refuse a script without an entry point, because a library has
none, and a library compiles. A run refuses it: `Wait` returns `ERR_NO_MAIN`.
`Save` and `Derive` also refuse it, each before anything runs.

`Save` writes an artifact as one bundle, and `Load` reads it back with no loader
and no source. Thus a host can compile one time and run in a different place.
`Load` takes the options that the compile took. Here there are none, so it uses
the plugins of the runtime itself:

```go
bundle, err := built.Save()
if err != nil {
	return err
}

err = os.WriteFile(BUNDLE, bundle, BUNDLE_MODE)
if err != nil {
	return err
}

// Elsewhere, later.
saved, err := os.ReadFile(BUNDLE)
if err != nil {
	return err
}

loaded, err := runtime.Load(saved)
if err != nil {
	return err
}

value, err := runtime.Start(ctx, loaded).Wait()
```

`value` is `"ok"`, as it is for `built`. `Save` refuses an artifact that a run
cannot start, because a host reads a bundle back to run it. `steps.star` is a
module, and it gives `steps.star: no entry point`, which matches `ERR_NO_MAIN`.
`Load` refuses bytes that are not a bundle, with `unmarshal bundle: proto:
cannot parse invalid wire-format data`. It refuses a bundle that does not hold a
module that one of its units loads, with `ERR_NO_UNIT`.

#### `WithPlugins`

The plugins of the runtime itself are always there, and `WithPlugins` adds
plugins of the host to them, for one compile. This script sees `setting()`, and
also `spawn`, `join`, `json` and the others. `_Settings` is the plugin in
[Plugin](#plugin).

```go
settings := &_Settings{
	values: map[string]string{"region": "eu-west-1"},
}

source := &runtime.Source{Entry: SCRIPT, Text: src}

built, err := runtime.Compile(source, runtime.WithPlugins(settings))
```

`WithPlugins` takes all the plugins of the host at one time, and a second
`WithPlugins` replaces the first. The file system is one of them. `file`, from
`github.com/thebagchi/lark/v1/plugin/file`, can get to all that the host process
can get to. Thus a script has it only when its host gives it. A host that runs
scripts from other persons does not give it.

```go
built, err := runtime.Compile(source, runtime.WithPlugins(settings, &file.Plugin{}))
```

Without it, a compile refuses a script that names it, before anything runs. For
example, `reads.star`:

```python
def main():
    return file.read("notes.txt")
```

```text
reads.star:2:12: undefined: file
```

A plugin cannot take a name that another plugin supplies, and this includes the
plugins of the runtime itself. A hidden builtin is a defect, and it would show
much later, as incorrect behaviour in a different place. The compile that builds
the environment refuses such a plugin, and names both plugins. The error matches
`ERR_CONFLICT`. For example, a plugin with the name `mine` that supplies its own
`sleep`:

```text
compile build.star: mine supplies "sleep", which core already supplies: two plugins supply one name
```

#### `Derive`, `Check`, `Emit`

`Derive` reads a script into the flow that a user interface shows. It gets the
loaded modules through the loader of the host, here `modules`:

```go
flow, err := runtime.Derive(&runtime.Source{
	Entry:  SCRIPT,
	Text:   build,
	Loader: modules,
})
if err != nil {
	return err
}
```

For `build.star`, with `fetch` from `steps.star`, the flow is:

```json
{
  "functions": [
    {"name": "fetch", "params": ["name"], "body": "print(\"fetching \" + name)\nreturn name"}
  ],
  "text": "fetch(\"lark\")\nprint(\"built\")\nreturn \"ok\""
}
```

If the walk cannot model a function as statements, the function goes into the
flow as the text of its body. Both functions do so here. Thus a flow always
holds all that the script does. lark refuses what it cannot carry: modules that
load each other give `ERR_CYCLE`, as they do for a compile.

A user interface draws a flow before a run. What a run did is its graph, in
[Execution](#execution). A graph shows a function when the run first calls it,
so a function that nothing called is not in the graph. Draw the flow beside the
graph to show what did not occur yet. A node has the name of its function, so
the function that a statement calls matches the statement to its node.

A user interface that edits a workflow sends it back as a `Flow`, the message of
`workflow.proto`, which protojson reads. The host writes its script with `Emit`,
which checks the flow first. Then the host compiles the script as it compiles
all other scripts:

```go
flow := new(runtime.Flow)

err := protojson.Unmarshal(sent, flow)
if err != nil {
	return err
}

src, err := runtime.Emit(flow)
if err != nil {
	return err
}

authored, err := runtime.Compile(&runtime.Source{Entry: "authored.star", Text: src})
if err != nil {
	return err
}
```

For this `sent`:

```json
{
  "functions": [{"name": "fetch", "params": ["name"], "body": "print(\"fetching \" + name)\nreturn name"}],
  "main": {"statement": [{"call": {"function": "fetch", "args": ["lark"]}}]}
}
```

`Emit` writes:

```python
def fetch(name):
    print("fetching " + name)
    return name

def main():
    fetch("lark")
    pass
```

`Check` refuses a flow whose `main` passes two arguments to `fetch`, and thus
`Emit` also refuses it:

```text
fetch takes 1 and is passed 2: a call passes arguments the function does not take
```

`Compile` refuses the same call in `build.star`, at the line and the column of
the call, as the table in [Compile, Load](#compile-load) shows:

```text
build.star:4:5: call of fetch: a call passes arguments the function does not take
```

Both errors match `ERR_ARITY`. One mistake has one value, in a flow or in a
script.

#### `Start`

```go
run := runtime.Start(ctx, built)

// Start returned at once. The host goes on with its own work, and collects
// the run when it wants it.
value, err := run.Wait()
if err != nil {
	return err
}

log.Printf("main returned %s", value)
```

If a host needs only the result, it writes `value, err := runtime.Start(ctx,
built).Wait()`, which blocks until the run ends. [Execution](#execution) does
more with a started run: its `Status` is the graph that a user interface draws.

`Start` returns while the script still runs, whatever the script does next. A
cancel of the context that the host gave is the one way for a host to stop the
run. It stops a script that is busy as easily as a script that waits, because
the interpreter gets the cancel between instructions. `Wait` then returns
`ERR_CANCELLED`. [Cancellation](#cancellation) lists all other ways that a run,
or a part of a run, stops.

**Nothing keeps the run for you.** There is no store, no id to find a run, and
nothing that removes runs on a schedule. You decide what to keep of a finished
run, and for how long. A run gives `Status` while you hold it, and the garbage
collector removes it when you release it.

#### `WithArgs`

`arg()` comes from the args plugin, which is a plugin of the runtime itself. A
script declares its arguments at module level. Each has a default for a run that
supplies nothing:

```python
host = arg("host", "localhost")
port = arg("port", 8080)
```

Arguments are a map of Go values. A host that has them gives the map:

```go
run := runtime.Start(ctx, built, runtime.WithArgs(map[string]any{"host": "build.internal"}))
```

A host reads JSON from a flag or a request into the map with `encoding/json`. If
the JSON is not an object, the read fails:

```go
// body is a JSON object from a flag or a request: {"host": "build.internal"}.
var args map[string]any

err := json.Unmarshal(body, &args)
if err != nil {
	return fmt.Errorf("bad arguments: %w", err)
}

value, err := runtime.Start(ctx, built, runtime.WithArgs(args)).Wait()
if err != nil {
	return err
}

log.Printf("main returned %s", value)
```

`[1, 2]` fails at `json.Unmarshal`, with `json: cannot unmarshal array into Go
value of type map[string]interface {}`, before the host makes a run. A value
that JSON cannot hold, for example a function or a channel, fails the run when
it starts. The error is `an argument holds a value JSON cannot: proto: invalid
type: func()`, which matches `ERR_NOT_JSON`.

Each run initialises the script again, so each run of one artifact can get
different values. With `{"host": "build.internal", "port": 9090}`, the script
sees `build.internal` and the int `9090`. With nothing, it sees `localhost` and
`8080`.

#### `WithMemory`

The runtime limits what it allocates for a run. Examples are a file that it
reads, a value that it stores, and a thread that it spawns. A run that asks for
more fails with the amount that it asked for, before the allocation. The default
is 256MB. A host that runs scripts from other persons sets less:

```go
run := runtime.Start(ctx, built, runtime.WithMemory(64<<20))
```

The host sets it before the run starts, and never after. A ceiling that a script
can raise during the run is not a ceiling. It limits the allocations of this
runtime, not those of the interpreter. It does not stop a script that builds a
large list in a loop.

[What a run can use](#what-a-run-can-use), below, tells what this covers and
what it does not cover. Read it before you run a script from another person.

#### `WithLog`, `WithLogger`

`WithLog` writes each printed line to a writer, as the `lark` command writes it:
first where the line came from, and then what it said.

```go
value, err := runtime.Run(ctx, source, runtime.WithLog(os.Stdout))
```

```text
time=2026-10-03T16:33:34.425+05:30 level=INFO thread=thread_0 function=fetch msg="fetching lark"
time=2026-10-03T16:33:34.425+05:30 level=INFO thread=thread_0 function=main msg=built
```

`WithLogger` takes a slog logger, for a host that has its own handler. Each
printed line is one record. The line is the message, and the thread and the
function that printed it are the `thread` and `function` attributes. The handler
adds its own time and layout. Here, the text handler of slog writes each line
when the script prints it, to standard output and to a file at the same time:

```go
file, err := os.Create(LOG_FILE)
if err != nil {
	return err
}

logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, file), nil))

_, err = runtime.Start(ctx, built, runtime.WithLogger(logger)).Wait()

// Closed however the run ended.
return errors.Join(err, file.Close())
```

```text
time=2026-10-02T16:27:43.656+05:30 level=INFO msg="fetching lark" thread=thread_0 function=fetch
time=2026-10-02T16:27:43.656+05:30 level=INFO msg=built thread=thread_0 function=main
```

Or the host keeps the lines as JSON, and reads them when the run is over:

```go
var out bytes.Buffer

logger := slog.New(slog.NewJSONHandler(&out, nil))

run := runtime.Start(ctx, built, runtime.WithLogger(logger))

_, err := run.Wait()
if err != nil {
	return err
}

log.Print(out.String())
```

```text
{"time":"2026-10-02T16:27:43.656550764+05:30","level":"INFO","msg":"fetching lark","thread":"thread_0","function":"fetch"}
{"time":"2026-10-02T16:27:43.656553584+05:30","level":"INFO","msg":"built","thread":"thread_0","function":"main"}
```

The threads of a run print at the same time, and a slog handler writes one
record at a time, so lines never mix. The function is the one that called
`print`: `<toplevel>` for the top level of a module, and `lambda` for a lambda.

The handlers of slog write `msg` before the attributes, as above. `WithLog`
writes it after them.

**The host names the file, because the host knows what this run is, and the
runtime does not.** When two runs of one artifact get two loggers, they write
two files. If the handler cannot write a line, lark discards the line and does
not stop the run. A full disk must not end a workflow that otherwise works.

#### `WithChanges`

A user interface keeps its own copy of the graph, from `{}`. It patches the copy
with each change when the run makes it, so it shows which function each thread
runs at that time. Here, each change goes to the interface as JSON, through a
channel of the host, `updates`:

```go
watch := func(change *runtime.Change) {
	raw, err := protojson.Marshal(change)
	if err != nil {
		log.Printf("change: %v", err)

		return
	}

	updates <- raw
}

run := runtime.Start(ctx, built, runtime.WithChanges(watch))
```

These are the changes that a run of `build.star` sends, one line for each
operation, and a blank line between changes:

```text
add /status "STATUS_RUNNING"

add /functions [{"name":"main","status":"STATUS_RUNNING"}]
add /functions/0/threads ["thread_0"]

add /functions/0 {"name":"fetch","status":"STATUS_RUNNING"}
remove /functions/1/threads
add /functions/0/threads ["thread_0"]
add /calls [{"callee":"fetch","caller":"main"}]

remove /functions/0/threads
add /functions/1/threads ["thread_0"]
add /functions/0/status "STATUS_SUCCEEDED"

remove /functions/1/threads
add /functions/1/status "STATUS_SUCCEEDED"

add /status "STATUS_SUCCEEDED"
```

In order:

1. The run starts.
2. `main` starts on `thread_0`.
3. `fetch` starts, and goes before `main` in the list. `thread_0` moves to it,
   and the call from `main` is added.
4. `fetch` ends, and `thread_0` moves back.
5. `main` ends.
6. The run succeeds.

Applied in this order, the changes give exactly the graph that `Status` returns.

`watch` runs on the thread whose step made the change, and no other step occurs
while it runs. Thus a `watch` that blocks, for example on a full channel, stops
the run. Changes arrive one at a time, in order.

In `watch`, `Status` is exactly the graph after the current change. Thus a host
can give the whole graph to an interface that joins late, and then patch it from
the next change. This is also true for the last change: from then, `Status`
tells that the run ended, a short time before `Done` closes.

The changes carry only the graph, and the log goes beside them. A run that
nobody watches builds no changes, so it pays only for its graph.

### Methods

These are the types that the functions above return, and the methods that a host
calls on them. The signatures are as the code declares them: `workflowpb.Graph`
is `runtime.Graph`.

#### Artifact

| Signature | Purpose |
| --- | --- |
| `func (a *Artifact) Save() ([]byte, error)` | Encodes the artifact as one bundle that `Load` can read back. The bundle holds the name of the entry unit, and all compiled units, with dependencies first. Refuses an artifact that has no `main` that a run can call, with `ERR_NO_MAIN`. |

Use [Start](#start) to run an artifact. [Compile, Load](#compile-load) saves an
artifact and reads it back.

#### Execution

| Signature | Purpose |
| --- | --- |
| `func (e *Execution) Wait() (starlark.Value, error)` | Blocks until the run is over, and returns what `main` returned, or why the run failed. |
| `func (e *Execution) Done() <-chan struct{}` | Closes when the run is over, for a host that selects on it beside its own work. |
| `func (e *Execution) Status() *workflowpb.Graph` | The graph of the run, as it is now or as it ended. It shows each function that the run called, with its status and its threads. It also shows the calls between the functions, and what ended the run if it failed. |

`Status` is what a user interface draws: a call graph, as a profiler draws one.
It has one node for each function that the run called. A node shows the status
of its function and the threads that run it. The graph also has an edge for each
call between two functions. The interface reads it when it asks, during the run
or after it. Or it keeps it current with [WithChanges](#withchanges).

A function is one node, however many times the run called it. The node shows the
status of its newest call. Thus a function that a retry calls until it succeeds
shows succeeded, and a function whose last call still runs shows running.

A call is one edge from caller to callee, however many times the run made it. A
spawn is an edge from the function that spawned. A line that calls no function
is not a node, for example a join, a sleep or a cancel. A library call is also
not a node.

```go
ctx, stop := context.WithCancel(ctx)
defer stop()

run := runtime.Start(ctx, built)

_Supervise(run, stop)

value, err := run.Wait()
if errors.Is(err, runtime.ERR_CANCELLED) {
	return fmt.Errorf("stopped after %v: %w", LIMIT, err)
}

if err != nil {
	return err
}

log.Printf("main returned %s", value)
```

```go
// _Supervise holds a started run to LIMIT, stopping it with stop, and reports
// on it every EVERY until it is over.
func _Supervise(run *runtime.Execution, stop context.CancelFunc) {
	ticker := time.NewTicker(EVERY)
	defer ticker.Stop()

	expired := time.After(LIMIT)

	for {
		select {
		case <-run.Done():
			return
		case <-expired:
			// Cancelling does not wait: Done closes once the run has unwound.
			stop()
		case <-ticker.C:
			log.Printf("%d functions so far", len(run.Status().GetFunctions()))
		}
	}
}
```

| Status | Meaning |
| --- | --- |
| `STATUS_RUNNING` | still runs |
| `STATUS_SUCCEEDED` | finished |
| `STATUS_FAILED` | something went wrong, and `cause` tells what |
| `STATUS_CANCELLED` | stopped: **not** a failure |

**Cancelled is not failed, and the difference is important.** This runtime stops
at the first failure, so a failed run stops threads that did nothing wrong.
Without a separate value, one broken script would show many failures.

When `main` returns, lark cancels a function on a thread that nothing joined, if
it still runs. Its status is succeeded or cancelled, and this depends on how
fast it was. The status of a stopped run is `STATUS_CANCELLED`, and the node of
`main` is also cancelled. A stopped run has no cause, because nothing went wrong
in it.

The cause of a failed run names the function whose failure ended it, and what
that failure said. Thus a host goes to that node, and does not search for a
failed node:

```go
graph := run.Status()

log.Printf("%s with %d functions", graph.GetStatus(), len(graph.GetFunctions()))

cause := graph.GetCause()
if cause != nil {
	log.Printf("ended by %s: %s", cause.GetFunction(), cause.GetFailure())
}
```

For this script:

```python
def work():
    print("working")

def boom():
    fail("boom")

def main():
    first = spawn(work)
    second = spawn(boom)
    join(first, second)
```

```text
STATUS_FAILED with 3 functions
ended by boom: fail: boom
```

`main` also failed in that run, and that is not a second count of one failure.
The failure got to `main` through `join`, so the evaluation of `main` failed.
`cause` tells which call caused the failure.

A question costs nothing and changes nothing. Each caller that asks gets an
answer, as often as it asks. Two interfaces that watch one run both get answers.

`Status` does not use up a run, and `Wait` does not either. A host that must
keep a record of a run after it releases the run keeps the last `Status` and the
logs of that run.

If a host stops its wait for a run, the run continues. `Wait` blocks until the
run is over, and takes no context of its own. A caller that must not block
selects on `Done()`. Neither stops the run. To stop a run, cancel the context
that you gave to `Start`.

##### The graph as JSON

A user interface takes the graph as JSON. `Status` returns the message of
`workflow.proto`, so protojson, from
`google.golang.org/protobuf/encoding/protojson`, writes it:

```go
graph, err := protojson.Marshal(run.Status())
if err != nil {
	return err
}
```

For a run of `build.star`:

```json
{
  "status": "STATUS_SUCCEEDED",
  "functions": [
    {"name": "fetch", "status": "STATUS_SUCCEEDED"},
    {"name": "main", "status": "STATUS_SUCCEEDED"}
  ],
  "calls": [
    {"caller": "main", "callee": "fetch"}
  ]
}
```

Functions are in the order of their names, and calls are in the order of caller
and then callee. Thus two reads of one run differ only where the run changed.
While the run continues, a node also lists the threads that run it, as in
`{"name": "fetch", "status": "STATUS_RUNNING", "threads": ["thread_0"]}`. When
the run ends, a node lists no threads.

A change to a graph is a JSON Patch, as RFC 6902 sets it. Thus a user interface
that holds the graph as JSON can apply a change with any JSON Patch library.
`Change`, in `workflow.proto`, holds the operations. `Operation`, in
`patch.proto`, is one operation:

- `op` is spelled as the RFC spells it.
- `path` and `from` are JSON Pointers.
- `value` is any JSON.

Each field is absent for an op that does not use it. Paths point into the JSON
that protojson writes. The protojson package does not write a field that has its
default value. Thus a change sets a field with `add`, and removes a list that
becomes empty. [WithChanges](#withchanges) shows the changes of a run.

#### Handle

A handle is a spawned thread, as a script holds it and a host sees it.

| Signature | Purpose |
| --- | --- |
| `func (h *Handle) Name() string` | The function that the thread runs. |
| `func (h *Handle) Thread() string` | The id of the thread: `thread_1`, or `thread_1_2` for the second thread that `thread_1` started. |
| `func (h *Handle) Done() <-chan struct{}` | Closes when the evaluation of the thread is over, however it ended. |
| `func (h *Handle) Stop()` | Cancels the thread, and does not wait for it. |

A handle is also a Starlark value, so it has the five methods that each value
has. The interpreter calls them, not a host: `String`, `Type`, `Freeze`, `Truth`
and `Hash`.

The entry point is `thread_0`, and it is not a handle. The children of a thread
get numbers from 1 after its own id, in the order that it made them:
`thread_1_1` is the first child of `thread_1`. Thus a script numbers its threads
the same on each run.

A script gets a handle from `spawn`. A host gets a handle as an argument to its
own builtin. `deadline(worker, ms)` stops a thread that still runs `ms`
milliseconds later. `_Deadlines` gives it to a script that is compiled with
`runtime.Compile(source, runtime.WithPlugins(&_Deadlines{}))`.

```go
const (
	// DEADLINES is what this plugin is called, and DEADLINE the builtin it
	// gives a script.
	DEADLINES = "deadlines"
	DEADLINE  = "deadline"
)

// _Deadlines gives a script deadline(worker, ms). Empty: a deadline belongs to
// the call that set it, so the plugin has nothing to hold.
type _Deadlines struct{}

// Name is what a clash calls this plugin.
func (d *_Deadlines) Name() string {
	return DEADLINES
}

// Values is the one name this plugin gives a script.
func (d *_Deadlines) Values() starlark.StringDict {
	return starlark.StringDict{
		DEADLINE: starlark.NewBuiltin(DEADLINE, _Deadline),
	}
}

// _Deadline is deadline(worker, ms): it stops worker if it is still running ms
// milliseconds from now. It returns at once, so the script goes on meanwhile.
func _Deadline(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		worker *runtime.Handle
		limit  int
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &worker, &limit)
	if err != nil {
		return nil, err
	}

	go func() {
		select {
		case <-worker.Done():
			// Finished in time: nothing to stop.
		case <-time.After(time.Duration(limit) * time.Millisecond):
			log.Printf("%s on %s is overdue", worker.Name(), worker.Thread())

			worker.Stop()
		}
	}()

	return starlark.None, nil
}
```

The interpreter calls those five methods, and a script sees their results:

```python
def work():
    sleep(5000)

# Frozen once the module has initialised, as every module-level name is,
# which calls Freeze on the handle. It holds nothing to protect.
warm = spawn(work)

def main():
    worker = spawn(work)

    # The host's builtin: stops worker if it is still running in 100 ms.
    deadline(worker, 100)

    # String: <handle work #thread_2>, since warm is thread_1.
    print(worker)

    # Type: handle
    print(type(worker))

    # Truth: a handle is always true.
    if worker:
        print("spawned")

    # Hash: refused, so this fails the run with "handle is unhashable".
    index = {worker: 1}
```

A script that joins the handle sees the effect of the deadline:

```python
def work():
    sleep(5000)

def main():
    worker = spawn(work)
    deadline(worker, 100)
    join(worker)
```

After 100 ms, the host logs `work on thread_1 is overdue`, and the join fails
with `work: cancelled: context canceled`, which matches `ERR_CANCELLED`.

### Interfaces a host implements

| Interface | Method | Purpose |
| --- | --- | --- |
| `Loader` | `Resolve(from string, target string) (string, error)` | The identity of the module that a script in `from` spells `target`. Two spellings of one module resolve to one name, so lark builds the module one time. A host gives a loader as the `Loader` of a `Source`. |
| `Loader` | `Load(name string) ([]byte, error)` | The source of the module that `Resolve` named. |
| `Plugin` | `Name() string` | The name of the plugin, so that an error about two plugins that supply one name can name it. |
| `Plugin` | `Values() starlark.StringDict` | The names that the plugin gives a script. |

#### Loader

```go
// _Memory serves modules from a map, keyed by the name a script loads each
// one by, as a test or a host keeping its scripts in a database would.
type _Memory struct {
	files map[string][]byte
}

// Resolve names a module by its spelling alone. There are no directories, so
// "steps.star" is one module whichever script loads it, and it is built once.
func (m *_Memory) Resolve(from string, path string) (string, error) {
	return path, nil
}

// Load is the source of the module Resolve named.
func (m *_Memory) Load(name string) ([]byte, error) {
	src, found := m.files[name]
	if !found {
		return nil, fmt.Errorf("module %s: %w", name, fs.ErrNotExist)
	}

	return src, nil
}
```

A loader that has directories resolves `target` relative to `from`, as the
default loader does. Thus `lib/steps.star`, loaded from `ci/build.star`, is
`ci/lib/steps.star`.

#### Plugin

```go
const (
	// SETTINGS is what this plugin is called, and SETTING the builtin it
	// gives a script.
	SETTINGS = "settings"
	SETTING  = "setting"
)

// _Settings gives a script setting(name): a value from the host's own
// configuration, which a script has no other way to reach.
type _Settings struct {
	values map[string]string
}

// Name is what a clash calls this plugin.
func (s *_Settings) Name() string {
	return SETTINGS
}

// Values is the one name this plugin gives a script.
func (s *_Settings) Values() starlark.StringDict {
	return starlark.StringDict{
		SETTING: starlark.NewBuiltin(SETTING, s._Setting),
	}
}

// _Setting is setting(name): what the host holds under name, or None when it
// holds nothing there.
func (s *_Settings) _Setting(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &name)
	if err != nil {
		return nil, err
	}

	value, found := s.values[name]
	if !found {
		return starlark.None, nil
	}

	return starlark.String(value), nil
}
```

In a script that is compiled with this plugin, `region = setting("region")` is
`"eu-west-1"`.

#### A plugin in another process

A plugin does not have to be a package that this binary imports. The host can
listen, and a plugin in another process can connect and tell what it supplies.

Build the plugin into a directory, and give that directory to lark with `-p`:

```
go build -o plugins/lark-clock.bin ./cmd/clock
lark -s build.star -p plugins
```

lark searches the directory for `lark-*.bin`. The extension tells that a file is
compiled, and the prefix tells that the file is for lark. Thus the directory can
hold other binaries, and lark does not start them. lark starts each plugin,
waits until the plugin tells what it supplies, and keeps the plugin until the
run ends. A host that embeds the runtime does these steps in its own code. The
host gives `Listen` a socket path and a secret token:

```go
// _Host runs source with the plugins in PLUGINS, each in a process of its own.
// The plugins answer calls until the run ends, so the listener closes after it.
func _Host(ctx context.Context, source *runtime.Source, socket string, token string) error {
	listener, err := remote.Listen(socket, token)
	if err != nil {
		return err
	}

	err = _Run(ctx, source, listener)

	return errors.Join(err, listener.Close())
}

// _Run runs source with the plugins that listener starts. lark names a plugin
// that does not start, and runs without it. This host refuses the run.
func _Run(ctx context.Context, source *runtime.Source, listener *remote.Listener) error {
	loading, err := listener.Load(PLUGINS, "")
	if err != nil {
		return err
	}

	if len(loading.Skipped) > 0 {
		return errors.Join(loading.Skipped...)
	}

	built, err := runtime.Compile(source, runtime.WithPlugins(listener.Plugins()...))
	if err != nil {
		return err
	}

	_, err = runtime.Start(ctx, built).Wait()

	return err
}
```

**If a plugin does not start, lark skips it and names it on standard error.**
The run continues without that plugin. The search reads the name of a file, not
its execute bit. Thus a file whose name matches and that is not a program is a
usual result.

A host that must refuse such a run examines `Loading`, as `_Run` does. `Loaded`
holds the path of each plugin that started. `Skipped` holds an error for each
plugin that did not start.

**lark starts the plugins, and the plugins connect back to lark.** This is not
unnecessary work. The host listens, so nothing must connect to a plugin. Thus a
plugin needs no port, no address that other processes must know, and no way for
other processes to get in. Also, lark does not parse a handshake line from the
standard output of a child process. lark decides which code runs, and that is
why the host must be the parent.

**lark makes a new token for each run, and sends it in the environment.** lark
never sends the token in an argument, because the process table shows the
arguments to all users of the machine. A plugin reads `LARK_PLUGIN_SOCKET` and
`LARK_PLUGIN_TOKEN`. lark sets both when it starts a plugin.

The runtime sees a usual plugin, with `Name` and `Values`. Neither method
mentions a socket. Thus the code that compiles and runs a script did not change
for a plugin in another process. A script also cannot see the difference: it
writes `clock.now()`, as it does for all modules.

`cmd/clock` is the example. The important part is what it does not do. It
imports the schema that is generated from `proto/plugin.proto`, and nothing
else: no `starlark.StringDict`, and no part of the runtime. Thus you can write a
plugin in a language that this repository does not use.

**Only data goes between the processes.** A function, a thread handle or a
module has no meaning in another process, so lark refuses to send one. It
refuses at compile time when the source shows the value clearly, and at run time
if not. A plugin never gets such a value, so it never must decide what to do
with one.

Numbers go as protobuf numbers, which are float64. An integer comes back as a
float. lark refuses an integer larger than 2^53, and does not round it silently.

**`Listen` returns an error, and that is intentional.** A blank import cannot
report a failed connection. It cannot wait for a registration, and it cannot
close a socket when the host stops. The plugins of the runtime still register by
import. All other plugins go to a compile through `WithPlugins`, and a plugin in
another process does too.

**lark uses only Unix sockets.** The names of an attached plugin go into each
script that is compiled with the plugins of the listener. Such a script can also
have `file`, if the host gives it. `file` can get to all files that the host
process can get to. Thus an open port would give the filesystem to each process
that can connect to the port. There is no TCP option.

lark checks the token before it accepts a name. Thus a process that only found
the socket cannot install a name.

**When a plugin stops, its names fail, and they stay.** A call to one of its
names fails, and does not wait on a stream that nothing reads. The host
continues to run. The names also stay, because the run has already built its
environment. If lark removed the names during the run, one failure would become
a failure that is more difficult to understand.

### Errors

The runtime declares these errors. Each error is the same value as the error of
its subpackage. Thus `errors.Is` matches it, however many times something
wrapped it, and through each of its two names. The examples in [Compile,
Load](#compile-load) and [Execution](#execution) test errors in this way. The
error of a run comes from `Wait`, or from `Run`.

| Error | Returned by | When |
| --- | --- | --- |
| `ERR_ARITY` | `Compile`, `Check` | A call gives a function arguments that it does not take: too many, too few, one that has no parameter, or one two times. This applies to a script and to a flow. |
| `ERR_CYCLE` | `Compile`, `Derive` | Modules load each other in a ring. The error of a compile names the ring. |
| `ERR_CONFLICT` | `Compile`, `Load` | Two plugins supply one name. The plugins of the runtime are included. |
| `ERR_NO_MAIN` | `Wait`, `Save`, `Derive` | The script defines no `main`, or a `main` that a run cannot call with no arguments. |
| `ERR_NOT_JSON` | `Wait` | An argument that `WithArgs` got holds a value that JSON cannot hold: a function, a channel, a struct. |
| `ERR_NO_UNIT` | `Load` | The bundle does not hold its entry, or a module that one of its units loads. |
| `ERR_ASSERT` | `Wait` | The script asserted a false condition, or asserted with only `msg =`. |
| `ERR_NOT_A_CONDITION` | `Wait` | `assert` got only a message. That looks like a failure, but it would pass. |
| `ERR_NOT_A_NAME` | `Wait` | `spawn` got nothing, a value that is not a function, or a function that takes an argument that `spawn` cannot supply. |
| `ERR_CAPTURES` | `Wait` | A value that goes to a new thread captures a local variable. |
| `ERR_NOT_A_HANDLE` | `Wait` | `join` or `cancel` got a value that is not a handle, or a keyword argument. |
| `ERR_CANCELLED` | `Wait` | Something cancelled a joined handle, or the context of the run stopped the run. |
| `ERR_INTERRUPTED` | `Wait` | The end of the run stopped a `sleep` before its time. |
| `ERR_DURATION` | `Wait` | A duration is not a whole number of milliseconds that an `int32` can hold. |
| `ERR_MEMORY` | `Wait` | The script asked this runtime for more memory than `WithMemory`, or the default ceiling, lets it have. |
| `ERR_NESTED` | `Wait` | A script called `update`, `set` or `join` while it held a name. This also applies on each thread that the update started. |
| `ERR_DUPLICATE` | `Check` | A flow declares one name two times. |
| `ERR_ARGUMENTS` | `Check` | A call in a flow has both args and operands. |
| `ERR_CONSTANT` | `Derive`, `Check` | A module-level value that a flow cannot carry, or constants in a flow that name each other. |
| `ERR_NOT_FORKED` | `Check` | A flow joins or cancels a thread where it did not spawn the thread, or spawns two threads under one binding. |
| `ERR_UNRESOLVED` | `Check` | A name in a flow resolves to nothing. |
| `ERR_NO_BODY` | `Check`, `Emit` | A function or `main` in a flow has nothing to write. |
| `ERR_FORM` | `Emit` | A statement in a flow has no form that lark can write in a script. |
| `ERR_NOT_CARRIED` | `Derive` | An `arg()` declaration that a flow cannot carry. Its name is not a string literal, no value describes its default, or it has the wrong number of arguments. |
| `ERR_SIGNATURE` | `Derive` | A `def` whose signature a flow cannot carry: a default, a `*args` or a `**kwargs`. |
| `ERR_COLLISION` | `Derive` | Two modules that the script loads declare one name. |
| `ERR_ALIAS` | `Derive` | A load gives a different name to what it binds. |

A plugin keeps its own errors. This applies to the plugins of the runtime, and
to a plugin that a host gives. The error of a run holds them, wrapped as it
holds all errors. A host names a plugin error through the package of the plugin.

Five `remote` errors do not go to a run. Their rows tell where they go. The
errors of `core` are in the table above, with their runtime names.

| Error | When |
| --- | --- |
| `args.ERR_NOT_SUPPLIED` | Nothing supplies a declared argument, and the declaration has no default |
| `args.ERR_NOT_DECLARING` | A script calls `arg()` outside module level |
| `args.ERR_NOT_VALUE` | A supplied argument is a `google.protobuf.Value` that has no kind |
| `base64.ERR_ENCODED` / `base32.ERR_ENCODED` | Text is not in that encoding |
| `binary.ERR_BITS` | Text is not only 0 and 1, or its length is not a multiple of eight where whole bytes are necessary |
| `buf.ERR_FROZEN` | A script writes to a buffer, or reads from a reader, after a `spawn` froze it. A read changes a reader |
| `buf.ERR_SHORT` | A read asks a reader for more bytes than it has left |
| `event.ERR_NOT_DATA` | A post gets a function, a handle, a buffer, a reader, or a container that holds one |
| `event.ERR_POSTED` | A script posts one event a second time |
| `file.ERR_FILE` | A call of the `file` plugin fails. Each refusal of the plugin matches it. The three errors below also match it |
| `file.ERR_NOT_A_FILE` | `read` gets a device, a pipe or a directory |
| `file.ERR_LINE` | A value in a list of lines is not a line |
| `file.ERR_WALKED` | A script reads the lines of one `file.lines` call a second time |
| `flow.ERR_COUNT` | A count, or a limit of attempts, is less than one |
| `flow.ERR_ATTEMPT` | A script calls `n()` outside `repeat` or `retry` |
| `flow.ERR_TIMEOUT` | The call that `timeout` wraps does not finish in time |
| `hash.ERR_ALGORITHM` | A keyed digest asks for an algorithm that this plugin does not know |
| `hex.ERR_HEX` | Text is not hexadecimal, it has no digits where a number is necessary, or it has an odd number of digits where whole bytes are necessary |
| `integer.ERR_ORDER` | A byte order is not `"big"` or `"little"` |
| `jsonpath.ERR_POINTER` | A pointer is not a JSON Pointer, as RFC 6901 sets it |
| `jsonpath.ERR_MISSING` | A pointer names a value that is not there |
| `jsonpath.ERR_KIND` | A step asks for a member of a value that has no members, or for an index of a value that is not a list |
| `jsonpath.ERR_OPERATION` | A patch for `patch_json` has an op that this plugin does not know |
| `jsonpath.ERR_FIELD` | An op of a patch does not have a field that the op needs |
| `jsonpath.ERR_TEST` | A `test` op of a patch does not match |
| `jsonpath.ERR_INTO` | A `move` op of a patch moves a value into itself |
| `math.ERR_NUMBER` / `math.ERR_BASE` | A value is not a number; a logarithm has base one |
| `path.ERR_NOT_A_PATH` | `join` gets a value that is not text |
| `random.ERR_RANGE` | A range has an end less than its start, or a count is negative |
| `random.ERR_EMPTY` | A choice is from an empty sequence |
| `regexp.ERR_PATTERN` | RE2 cannot read the pattern, for example a lookahead, a lookbehind or a backreference |
| `remote.ERR_ARGUMENT` | A call to a plugin in another process gives a keyword argument, or a value that is not data. A compile refuses it when the source shows a function |
| `remote.ERR_VALUE` | A value cannot go through the socket, for example an integer larger than 2^53 |
| `remote.ERR_REMOTE` | The plugin reports a failure for a call |
| `remote.ERR_GONE` | A call goes to a plugin that stopped, or the plugin stops before it answers |
| `remote.ERR_QUIET` | A plugin does not tell what it supplies in the 10 seconds after it starts. `Load` puts the error in `Loading.Skipped`, not in a run |
| `remote.ERR_TOKEN` | A plugin gives the wrong token. The listener closes the stream. The plugin gets the error |
| `remote.ERR_FIRST` | The first message of a plugin is not a registration. The plugin gets the error |
| `remote.ERR_ATTACHED` | A plugin announces the name of a plugin that is still attached. The plugin gets the error |
| `remote.ERR_RENAMED` | A plugin that connects again supplies names that differ from its first names. The plugin gets the error |
| `state.ERR_NOT_DATA` | A store gets a function, a handle, a buffer, a reader, or a container that holds one |
| `unpack.ERR_DATA` | A value that must be bytes or a string is not bytes and not a string |
| `unpack.ERR_RANGE` | An integer is negative, or it does not fit its width or the 32 bits of a checksum, or a width is less than one |

### Also exported

Types: `Artifact`, `CompileOption`, `RunOption`, `Loader`, `Handle`, `Plugin`,
`Execution`, `Flow`, `Graph`, `Change` and `Source`. The other messages of the
schema, for example `Operation`, are in `proto/gen/workflow` and
`proto/gen/patch`.

### Cancellation

This table shows each way to stop a run, or a part of a run. For each way, it
shows:

- the error that a host or a script gets;
- the status that the graph gives each function;
- what `lark` prints.

Each row comes from a run of a small script, through `Start` and `Status`, and
through `lark` itself.

| Who | How | What stops | What it shows |
| --- | --- | --- | --- |
| A host | cancels the context that it gave to `Start` or `Run` | the whole run | `Wait` returns `ERR_CANCELLED`. The run, and each function that still runs, shows `STATUS_CANCELLED`, with no cause |
| A builtin of a host | calls `Stop` on a [Handle](#handle) that a script gave it | that thread | a `join` of the thread raises `ERR_CANCELLED`. Its function shows `STATUS_CANCELLED` |
| A person who runs `lark` | an interrupt or `SIGTERM` | the whole run | `script stopped` on standard error, with `flow` or `bundle` in place of `script` for those inputs, and exit status 4 |
| A script | `cancel(h)` | that thread | a `join` of the thread raises `ERR_CANCELLED`, as `work: cancelled: context canceled`. Its function shows `STATUS_CANCELLED` |
| A script | `timeout(ms, f)` that runs longer than `ms` | the call that it wraps | it raises `flow.ERR_TIMEOUT`, as `timeout(100, slow): timed out`. The wrapped function shows `STATUS_FAILED` |
| A script | a `join` that gets a failure | each handle that the join did not get to | the join raises that failure. The functions of those handles show `STATUS_CANCELLED` |
| A script | a failed `assert`; a `retry` that used all its attempts; a store, update or post of a value that is not data; a second post of one event; a `file.lines` loop that was cut short | the whole run, from each thread, joined or not | the error of the run is that failure, and `ERR_ASSERT` for the first two. The run shows `STATUS_FAILED`, and each function that still runs shows `STATUS_CANCELLED`. In a `retry`, a failed assertion stops only that attempt. The retry raises each other failure at the first attempt, and does not try again |
| The runtime | `main` returns, or raises an error | each thread that nothing joined | their functions show `STATUS_CANCELLED`. The run shows the result of `main` |

A `fail()`, a refusal by the memory budget, and each other error that a thread
raises stop only that thread. A `join` of that thread raises the error again. A
thread that nothing joins fails, but the run does not fail: its function shows
`STATUS_FAILED`, and the run succeeds. `file.lines` is the only exception. It
stops the whole run, because a loop has no place to raise its error. *[`assert`
or `fail`?](#assert-or-fail)* tells which one a script must use, and why.

For a run that failed, `lark` writes `script failed:` and the error on standard
error, and exits with status 1. It exits with status 4 only for a run that
something stopped.

A cancelled evaluation stops at its next instruction. It does **not** interrupt
a Go function that already runs. Thus a builtin that blocks must take a context
of its own.

`Wait` does not return until each thread that the run started has stopped. lark
cancels a handle that nothing joined, and does not wait for it, as the last row
tells. Thus a `spawn` that a script did not join cannot keep a call open.

## What a run can use

**Nothing limits a run.** `-m` limits one thing: the memory that *this library*
allocates for a script. That is less than it seems. Read this section before you
run a script that you did not write.

A run has a budget for the memory that the library allocates for it. The default
is 256MB:

```
lark -s build.star -m 64
```

```go
runtime.Start(ctx, built, runtime.WithMemory(64<<20))
```

`file.read` charges the budget *from the stat*, before it allocates. Thus a file
that the run cannot afford costs one syscall, and not the process. In a
measurement, a file of 512MB used a peak of 1057MB of memory. The cause is that
the bytes and the string copied from them are both live. `file.lines` charges
one line at a time. lark gives back what a value reserves when it gives the
value to the script. Thus a loop that reads a thousand files needs a budget for
the largest file, not for the sum of all the files.

**What the budget charges** is each allocation of the library whose size the
script did not already pay for:

- a file read;
- a spawned thread;
- a name and a value that a script puts in the store;
- an event that a script names or posts;
- the text or bytes of an integer that `integer.to_bytes`, `hex.from_int`,
  `binary.from_int` or a write to a buffer makes, because the script names the
  width.

A thread costs 13KB: a goroutine, an interpreter thread, its locals and a
handle. Measurements gave 11.6KB to 12.1KB. Before lark charged for threads,
twenty thousand threads used 287MB.

lark charges for the store and for events because nothing deletes from them.
After its first use, lark keeps a name until the run ends. Thus a name costs its
length and a fixed amount at its first use, whatever it holds. The fixed amount
is 320 bytes in the store, for its entry and the lock beside it. It is 256 bytes
for an event, which keeps one map where the store keeps two. Both figures come
from measurements, rounded up.

A value costs what it holds. Under a budget of 64KB, lark refuses a thousand
names that hold `None`, because of the names alone.

A read of a name with `state.get` makes nothing and costs nothing. lark never
gives back the cost of the value of an event, because a script cannot post the
event again. lark gives back the other costs at these times:

- the cost of a thread, when the thread ends;
- the cost of a stored value, when a script replaces the value;
- the cost of a read value, when lark gives the value to the script.

**The budget does not cover the memory of the script itself**, and the
difference is large. This script asks the library for nothing:

```python
def main():
    held = []

    for i in range(400000):
        held.append("a string long enough to be worth counting, number %d" % i)

    return len(held)
```

Under `-m 1`, a ceiling of one megabyte, the script uses **72MB** and succeeds.
lark never examines the budget, because the interpreter allocated each byte, and
this library did not. If you increase the loop count, the process fails,
whatever the value of `-m`.

Thus `-m` is a guard on the calls that charge the budget. It is not a sandbox.
`starlark-go` does not count its own memory, so the budget counts only what lark
can count correctly.

This is also why the encoders do not charge the budget. `base64.encode`
allocates a third more than the size of its input. But its input is a string
that the script made, and nothing charged that string. Thus this script runs to
completion under `-m 1`:

```python
held = "x" * 200000000        # 200MB, uncharged: the interpreter's
base64.encode(held)          # 267MB more, and the process is at 926MB
```

A charge on the encode would refuse the last third after the process already
held the 200MB. To close this gap, the interpreter must count memory, and this
runtime cannot add that from outside.

**Nothing limits the computation either.** There is no step limit and no
deadline. A script can loop forever. In a test, a script ran until a person
stopped it from outside, and nothing in the runtime stopped it. A run ends when
its context is cancelled. That is why `lark` handles an interrupt, and why a
host must give a context that it can cancel.

Recursion is the largest risk, because it stops the **process**, and not only
the run. The dialect lets a script use recursion, and a recursive function that
does not stop uses all of the stack. lark recovers a builtin that panics, and
the panic becomes an error. But Go cannot recover from a stack overflow, so
`WithRecover` cannot help.

**In summary: run a script that you do not trust in a process that you can lose,
under a cgroup.** Two limits seem to help, but they do not. Both were measured
on the script above:

- **`GOMEMLIMIT` has no effect here.** It is a *soft* limit: it makes the
  collector work more, and it never refuses an allocation. Thus live data
  continues to grow. With a limit of 300MiB, the script still got to 926MB.
- **`ulimit -v` stops the program when it starts**, whatever the script. Go
  reserves a large virtual address space at the start. Thus a limit on the
  address space gives `fatal error: failed to reserve page summary memory`
  before `main` runs. This also occurs for a script that only returns `1`.

A memory limit on a container, cgroup `memory.max`, is the limit that works.
When a process goes above that limit, the kernel stops the process. Go accepts
that, and this runtime cannot do it for itself.

**An allocator in the runtime would not close this gap.** `starlark-go` has no
allocation hook: `SetMaxExecutionSteps` and `SetLocal` are the only setters on a
thread. Thus no part of this runtime knows when a script appends to a list. If
lark put the charges that it *can* see in one place, the records would be
better, but the charges would not cover more. To close the gap correctly, the
interpreter must count memory.

There is no decision yet about a step limit or a deadline.
`SetMaxExecutionSteps` would limit the loop, but not the stack and not the
memory.

## A bundle is the program

`-b` compiles and does not run. It writes one file, which holds the name of the
entry unit and all compiled units, in the order of their dependencies. `-r` runs
a bundle:

```
lark -s build.star -b build.bin
lark -r build.bin
```

The units are already compiled, so lark does not parse or check them again. The
run gets the plugins that `lark` compiles with. A bundle is a program. Thus lark
refuses to write a module, which is a script with no `main`. A host does the
same with `Save`, `Load` and `Start`, as [Compile, Load](#compile-load) shows.

A bundle holds no picture of the workflow. The picture is two items, and neither
item goes in a bundle. A **flow** tells what a script will do: its functions,
the statements in them, and `main`. lark derives a flow from the source when you
ask for it:

```
lark -s build.star -t > build.json
```

A **graph** tells what a run did, and comes from the run: see
[Execution](#execution). Nothing changes a flow into a graph of threads that are
not started yet. A flow does not know which threads a run will make.

A flow cannot state some lines, for example a library call, a `return`, or a
`for` over a list. A function that has such a line goes into the flow whole, as
the text that it was written in. A flow that holds such a function still
generates its script exactly. If the flow came first, `-g` compiles the script
that the flow generates:

```
lark -g build.json -b build.bin
```

## Every run keeps its own log

The output of a script goes where the caller tells. `-l` also keeps the output,
in one file for each run. Each line tells when the script printed it, on which
thread, from which function, and then the text:

```
lark -s build.star -l logs
```

```
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_0 function=main msg="from the spine"
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_2 function=beta msg="from beta"
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_1 function=alpha msg="from alpha"
```

The thread is important because the lines of a concurrent script mix. The three
lines above also came in that order on standard output. Without the thread, the
file could not tell which thread printed which line.

A host that embeds the runtime names the file and selects the handler itself,
with [WithLog, WithLogger](#withlog-withlogger).

## The dialect

The dialect lets a script use sets, `while` loops and recursion. It does not let
a script assign a new value to a top-level name. A workflow freezes its globals,
so a new assignment would fail at run time. lark refuses it at compile time, and
thus reports the same mistake with its position.

lark freezes the module scope after the module initialises. Thus threads can
share the module scope safely. A script that changes a global from in a function
fails with an error, and does not corrupt memory.

One rule is not an option of the interpreter. lark compiles a lambda that is
written in `spawn(...)` with a parameter for each variable that the lambda
reads. The default of each parameter is that variable, so the lambda gets the
values at the spawn. See *[Give a thread what it
needs](#give-a-thread-what-it-needs)*. lark does not freeze a local variable as
it freezes the module scope. Before this rule, a closure could give a local
variable to another goroutine.

## What this does not do

- **Nothing limits a run.** `-m` limits what this library allocates for a
  script, not what a script allocates itself. In a measurement, a script that
  made a list used 72MB under a ceiling of 1MB. There is also no step limit and
  no deadline. A recursive script that does not stop also stops the process. Go
  cannot recover from a stack overflow, as it can recover from a panic in a
  builtin. See *[What a run can use](#what-a-run-can-use)*.
- **Nothing runs a flow directly, and this is a decision.** A flow becomes a
  script, and the script runs. There is one path, not two, so two paths cannot
  disagree about the meaning of a flow. A second interpreter would have that
  cost.
- **Nothing stays after the process restarts**, and nothing connects to a
  remote.

## Licence

MIT. See [LICENSE](LICENSE).
