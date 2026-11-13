# lark

lark embeds [Starlark](https://github.com/bazelbuild/starlark) in Go, with
concurrency.

A script and all the modules that it loads compile into one artifact. Each
`spawn` runs its function on its own goroutine and interpreter thread. A plain
call stays on the thread of its caller. The script starts this work, waits for
it and stops it:

```python
load("util.star", "shout")

def hello():
    return shout("hello")

def world():
    return shout("world")

def main():
    assert(1 + 1 == 2, "arithmetic still works")

    parts = join(spawn(hello), spawn(world))

    print(" ".join(parts))
```

`print` writes a log line to the transcript of the run. The line has the time,
the level, the thread and the function. It is the same on a terminal, in a pipe,
in a file, and with `-logs` or `-hold`:

```
time=2026-11-03T23:49:21.134+05:30 level=INFO thread=thread_0 function=main msg="HELLO WORLD"
```

## Try it

```
./build.sh binaries
./bin/lark.bin run samples/concurrent.star
./bin/lark.bin convert samples/concurrent.star   # the flow it describes, as JSON
```

A script and the flow that it describes are two forms of one workflow. `lark`
can run each form, and it can convert each form into the other. The first word
is the verb, and a verb is required:

```
lark run script.star                          run the script
lark run - < script.star                      read the script from standard input
lark convert script.star                      print the flow it describes, as JSON
lark run flow.json                            run the flow
lark convert flow.json                        print the Starlark it generates
lark run -logs dir script.star                keep a log in dir/script.star.log
lark compile -o out.bin script.star           compile into a bundle instead of running
lark run out.bin                              run a bundle
lark run -args '{"host": "db"}' script.star   supply the arguments it declares
lark run -memory script.star                  end the run at 95 percent memory in use (exit 5)
lark run -hold script.star                    print the lines at the end of the run, not as they come
lark run -graph - script.star                 write each change of the graph to standard output
lark version                                  print the versions of lark and Go
lark help run                                 print the help of one verb
```

`run`, `compile` and `convert` take one FILE. The extension of FILE tells its
kind:

- `.bin` is a bundle.
- `.json` is a flow.
- Any other name is a script.

Flags may come before FILE or after it. Each flag belongs to one verb:
`lark convert -hold x.star` exits 2 with `flag provided but not defined:
-hold`.

A first word that is no verb exits 2 with a hint. `lark -version` prints `lark:
unknown verb "-version" (did you mean version?)`, then `try lark help`.

A FILE of `-` reads standard input as a script. An error in it names
`<stdin>`, and its `-logs` file is `stdin.log`. To read standard input as
another kind, give `-as flow` or `-as bundle`: `lark run -as flow - <
flow.json`. `-as` goes with a FILE of `-` only. Check: `echo 'def main():
fail("x")' | lark run -` prints `script failed: <stdin>:1: fail: x`.

`-hold` keeps what the script prints until the run ends. The held lines wait in
a spool file in the temporary directory. lark says so on standard error: `lark:
lines are held until the run ends: /tmp/lark-N.log`. lark removes the file at
the end of the run.

`-plugins DIR` starts each `lark-*.bin` file in DIR as a plugin. The builtins of
a plugin join the names of the script. For example, `cmd/clock` built as
`plugins/lark-clock.bin` gives `clock.now`. A plugin that does not start gets
one line on standard error. The run goes on without it. A bundle needs the same
plugins when it runs. Compile it with `-plugins DIR`, and run it with
`-plugins DIR`. Without them the run exits 1. The error matches `ErrPlugin`:
`bundle failed: c.bin: plugin "clock": the bundle needs a plugin that was not given`.
The section A plugin in another process, in the package comment of `v1/runtime`,
has the details.

`-graph FILE` writes each change that the run makes to its graph. It writes one
JSON Patch on each line, as the run makes the change. The status of the run is
a patch like the rest. The first line makes it running. The last line gives it
the status that the run ended with. Apply the lines in order to `{}`, and you
get the graph that the run ended with.

`-graph -` writes the patches to standard output. What the script prints then
goes to standard error, `-hold` lines included. Thus standard output holds the
patches alone. With `-hold`, lark keeps the patches until the run ends. It then
writes them before the held lines.

To write the patches to a file that is named `-`, give its path as `./-`. lark
empties the file when it starts. If the directory of the path is not there,
lark exits 3 before the script runs. This check prints `ok`:

```
lark run -graph - script.star 2>/dev/null | python3 -c 'import sys, json; [json.loads(l) for l in sys.stdin]' && echo ok
```

When a run fails in a function other than `main`, lark can add a last line to
standard error. The line names the function that the graph gives as the cause:
`cause: deep`. lark adds it only when the failure text does not name that
function. Check: a script with `def deep(): fail("x")` and `def main(): deep()`
ends with that line.

`lark help` prints the verbs, the flags, examples and the exit codes on
standard output, and exits 0. So does `lark -h`. `lark help run` (or `lark run
-h`) prints the part that belongs to `run`. A mistake on the command line
prints one error line and `try lark help` on standard error, and exits 2.
Check: `lark help | head -1` prints `lark runs a Starlark script as a graph of
concurrent steps.`. Then `lark run -grph x` prints `flag provided but not
defined: -grph (did you mean -graph?)` and `try lark help`.

### Which scripts convert

For a script that converts, the script and its flow run the same program. This
command prints the same line as `lark run samples/concurrent.star`:

```
lark convert samples/concurrent.star > flow.json && lark run flow.json
```

The line is the log line. A terminal, a pipe and a file all get it:

```
time=2026-11-03T23:49:21.134+05:30 level=INFO thread=thread_0 function=main msg="HELLO WORLD"
```

Convert the flow back, and you get a script with the same statements. Blank
lines and a last `pass` in `main` can differ.

A script converts when a flow can carry all that the script holds. **A
module-level constant is the main exception.** The flow carries a constant when
its value is one of these:

- An int, a float, a string, bytes, `True`, `False` or `None`.
- A list of these.
- A dict of one pair with a string key that is not empty.
- A call of a function that the script declares.

An int must fit an `int64`, and `1` and `1.0` stay two values. A literal past
`int64` is not carried. A dict of more than one pair is not carried, because a
flow cannot keep the order of the pairs. A dict with a key that is not a string, such as `{1: "x"}`
or `{b"k": 1}`, stays as the text of its statement in the flow's `init`. A
tuple, a negative number and an expression are not carried. For these constants
`lark convert` exits 1 with `constant cannot be carried` (`ErrCarry`). The
script is not wrong: it runs and compiles.
`samples/patch.star` and `samples/pointers.star` are two such scripts. Check:
`lark convert samples/patch.star` prints `script failed: samples/patch.star:
BEFORE: constant cannot be carried` and exits 1. Then `lark run
samples/patch.star` runs it.

Only what `main` can reach counts. lark leaves out a constant or a `def` that
`main` cannot reach. Thus it never stops a conversion. Check: `A = (1, 2)` and
`def main(): print(1)` in one script convert. Add `print(A)` to `main`, and
`lark convert` exits 1.

The same error stops a conversion in these cases, which are rarer:

- An `arg()` call that is malformed.
- A `def` with a default value.
- `*args` or `**kwargs` in a signature.
- A top-level statement of a loaded module that is not a `def`, a `load` or an
  assignment.
- A `load` that gives a different name to what it binds.

They also count only when `main` can reach them.

A flow names no modules. When lark derives a flow, it puts the code of the
loaded modules into the flow. Thus the flow is complete. lark checks a flow
before it generates a script from it. If a flow cannot run, lark refuses it and
gives the reason.

### Flows and operands

A call in a flow passes its arguments as values when all of them are literals.
When one or more of them is a name, it passes them as operands:

```json
{"spawn": {"binding": "conn",
           "call": {"function": "connect", "operands": [{"name": "host"}]}}}
```

A call with literal arguments is written as `"args": [{"string": "db"}]` in the
flow. The script gets `connect("db")` back without a change. `{"name": "host"}` passes the
value that `host` has at the call. `host` can be one of these:

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
handle that it receives keeps its text when lark derives a flow. An example is
`def watch(h): join(h)`. The flow cannot tell which thread that handle is. A
flow also does not tell which thread id a spawn gets. The run gives the ids. A
reader matches `join(conn)` to its thread through the binding.

### Values in a flow

Every value in a flow is an object with one member, named for its kind. A
constant, the argument of a call, the literal of an operand and the default of
an `arg()` are values:

| Kind | JSON | Script |
|---|---|---|
| null | `{"null": null}` | `None` |
| bool | `{"bool": true}` | `True` |
| int | `{"int": "9223372036854775807"}` | `9223372036854775807` |
| float | `{"float": 1.5}` | `1.5` |
| string | `{"string": "db"}` | `"db"` |
| bytes | `{"bytes": "ZP8="}` | `b"d\xff"` |
| list | `{"list": {"items": [{"int": "1"}]}}` | `[1]` |
| map | `{"map": {"entries": {"k": {"int": "1"}}}}` | `{"k": 1}` |

An int is an `int64`, written as text (`"9223372036854775807"`) or as a number.
`{"float": 1}` is `1.0`, not `1`. Bytes are base64, so they can hold any byte. A
map has string keys. A tuple is not a kind, and a flow does not carry one.

A flow that holds a value that is not one is refused, and the check names it:
an object with no member, an int outside `int64` or that is not a whole number,
a float that is `NaN` or infinite, a `null` kind that holds anything but `null`,
and a value nested too deep. A bare `null` where a value stands is refused too,
with the hint `(a value is an object named for its kind, such as {"null": null})`.
Check: `{"args": {"a": {"name": "a", "default": null}}, "text": "pass"}` in
`n.json` stops `lark run n.json` with `flow.args{}.default: a null where a value
stands`. protojson reads that `null` as no default. A Go program reads a flow with
`runtime.Read`, which refuses that `null` too, as `lark run` does, and runs `Check`.

A text or a body in a flow that is not Starlark is refused too. Check: `{"text":
"!"}` in `b.json` stops `lark run b.json` with `flow failed: b.json: main: body is
not Starlark (line 1: unexpected input character '!'): no form`.

A flow from before values were tagged holds plain JSON, such as `"args":
["db"]`. It does not read now. Check: put `{"main": {"statement": [{"call":
{"function": "connect", "args": ["db"]}}]}}` in `old.json`. Then `lark run
old.json` exits 1 with `flow failed: old.json: proto: syntax error (line 1:67):
unexpected token "db" (a value is an object named for its kind, such as
{"string": "x"} or {"int": "3"})`. Write each value as an object named for its
kind.

### Wait for every thread with `outcomes`

`outcomes` is the form of `join` for a script that wants to continue after a
thread that failed. `join` raises the first failure and cancels the handles that
it has not reached. `outcomes` waits for all of them and returns a list with one
dict for each handle:

```python
def main():
    rs = outcomes(spawn(fetch), spawn(slow), spawn(fetch))
    for r in rs:
        if r["ok"]:
            print(r["value"])
        else:
            print(r["kind"], r["error"])      # for example: fail build.star:8: fail: not found
```

| Key | Value |
| --- | --- |
| `"ok"` | `True` when the thread returned a value. |
| `"value"` | The returned value, or `None` when the thread failed. |
| `"error"` | The whole error text, or `None` when the thread returned a value. |
| `"kind"` | `None` when the thread returned a value. Otherwise one of the four words below. |
| `kind = "fail"` | A `fail` call, or any thread error not named below. |
| `kind = "cancelled"` | The handle was cancelled. |
| `kind = "cycle"` | A value that holds itself. |
| `kind = "nested"` | A state operation inside an update. |

`outcomes` takes the same arguments as `join`. No handles gives an empty list. A
value that is not a handle is refused. It does not catch `assert`: a failed
`assert` in any thread still stops the whole run. A cancelled handle is a record
with kind `"cancelled"`, not a raise. When the thread that calls `outcomes` is
itself cancelled, `outcomes` does one of two things. It raises, or it returns a
`"cancelled"` record for each handle that the cancel ended. A handle that had
finished keeps its own record. `outcomes` never returns a false record. A script
may define its own global named `outcomes`, which hides the builtin. A function
that calls `outcomes` keeps its text when lark derives a flow. The flow has no
node for it.

### The samples

`samples/` has scripts that show the ideas of this guide. This table lists some
of them. A comment in each script tells its purpose:

| Script | Shows |
| --- | --- |
| `hello.star` | the smallest script that lark runs |
| `strings.star` | a library, with no `main` of its own |
| `modules.star` | `load`, which finds a module beside the file that loads it |
| `concurrent.star` | `spawn` and `join` |
| `queue.star` | `queue`: a producer, poppers that stop at `queue.CLOSED`, `queue.FULL`, and a `timeout()` around a pop |
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

To use the `lark` command, install it. This puts a binary named `lark` in
`$(go env GOBIN)`, or in `$(go env GOPATH)/bin` when GOBIN is not set.

```
go install github.com/thebagchi/lark/cmd/lark@latest
```

Check it: `lark version` prints `lark <version> <go version>`, for example
`lark v1.2.3 go1.26.10`. A build in a git checkout prints a pseudo-version such
as `v0.0.0-20261029193537-cc05da88e20f+dirty`. A build that has no version
information, such as `go run ./cmd/lark`, prints `(devel)`. Put the version in a
bug report.

To use the runtime as a library in your own program, add it to your module:

```
go get github.com/thebagchi/lark
```

The module needs Go 1.26.10, the `go` line of `go.mod`. A Go that is older
fetches this toolchain on first use, and prints `switching to go1.26.10`.

## Use it from Go

A Go program that embeds the runtime is a host. The guide for a host is the
package comment of `v1/runtime`. Read it on
[pkg.go.dev](https://pkg.go.dev/github.com/thebagchi/lark/v1/runtime), or in a
terminal, in a checkout of this repository:

```
go doc github.com/thebagchi/lark/v1/runtime
```

It tells which of `Run`, `Start` and `Eval` to call. It shows the loaders, the
plugins and the options of a run. The `Example` functions are in
`v1/runtime/tests/example_test.go`. `go test ./v1/runtime/tests` runs them.

## What a script gets

| Name | What it does |
| --- | --- |
| `spawn(fn)` | Runs `fn` on its own goroutine and interpreter thread, and returns a handle. `spawn(lambda: f(x))` passes the value that `x` has when `spawn` is called. |
| `join(h, …)` | Waits for each handle. Returns the result of each handle, in the given order. |
| `outcomes(h, …)` | Waits for each handle. Returns one dict for each handle, in the given order. It never raises for a failed thread and it does not cancel the other handles. |
| `cancel(h, …)` | Stops each handle. Does not wait. |
| `assert(cond, msg)` | Stops the **whole run** when `cond` is false. `assert(msg = "...")` always stops it. |
| `str(x)`, `repr(x)`, `print(x, …)` | The functions of Starlark itself, with a check: a value nested deeper than 10,000 levels gives `value nested too deep`. |
| `fail(msg, …)` | The function of Starlark itself, with the same depth check as `str`. Stops the **thread that calls it**. |
| `sleep(ms)` | Waits for that number of milliseconds. A cancel ends the wait early. |
| `repeat(n, fn, delay)` | Calls `fn` exactly `n` times and returns the last result. With `delay`, it waits that number of milliseconds between calls. |
| `retry(n, fn, delay)` | Calls `fn` until one attempt succeeds. With `delay`, it waits that number of milliseconds between attempts. Retries **only** assertions. |
| `timeout(ms, fn)` | Calls `fn` with a time limit, and fails if `fn` does not end in that time. |
| `n()` | The number of the attempt, from 1, in `repeat` or `retry`. |
| `load(path, name)` | Binds a name from another script. |
| `json` | `json.encode`, `json.decode`, and the other functions of the module that go.starlark.net supplies. |
| `state` | `state.set`, `state.get`, `state.update`. See below. |
| `event` | `event.post(name, value)` and `value, err = event.wait(name, ms)`: one thread waits for another. A subsequent section tells more. |
| `queue` | `q = queue.new()` makes a queue. `q.push(value)`, `value, err = q.pop()` and `q.close()`: one thread hands many values to others, in order. A subsequent section tells more. |
| `time` | The module of go.starlark.net. |
| `math` | The usual functions, and also `inf`, `nan`, `tau`, `trunc`, `isnan`, `isinf`, `log2`, `log10`, `gcd`. |
| `regexp` | `search`, `match`, `findall`, `sub`, `split`, `quote`. It uses RE2, so it does not backtrack. |
| `random` | `seed`, `int`, `float`, `choice`, `shuffle`, `bytes`. You can set the seed for each run. |
| `path` | `join`, `dir`, `base`, `ext`, `stem`, `split`, `parts`, `clean`, `isabs`. These work on text only. |
| `file` | `read`, `bytes`, `write`, `append`, `exists`, `remove`, `list`, `size`, `mkdir`, `stat`, `lines`, `writelines`, `appendlines`. It **can read and write the disk**, so a script has it only when its host gives it. `lark` gives it. |
| `patch_json`, `extract_json`, `match_json`, `len_json`, `find_key` | Functions with no module name: write `extract_json(doc, "/a/b")`, not `jsonpath.extract_json(...)`. The `jsonpath` plugin supplies them. |
| `hex` | `hex.from_bytes`, `to_bytes`, `from_int`, `to_int`, `from_binary` and `to_binary`. It writes upper case. It reads either case, with or without a `0x` prefix. See below. |
| `binary` | `binary.from_bytes`, `to_bytes`, `from_int` and `to_int`: text of 0 and 1, with the most significant bit first. It reads text with or without a `0b` prefix. See below. |
| `integer` | `integer.from_bytes` and `to_bytes`: an integer and its bytes. They are unsigned unless `signed = True` is given. They are big-endian unless `"little"` is given. See below. |
| `buf` | `buf.new()` makes a buffer to write bytes into, and `buf.reader(data)` makes a reader to take bytes apart. A subsequent section tells more. |
| `base64` | `base64.encode` and `decode`, and `urlencode` and `urldecode` for the alphabet that is safe in a URL. |
| `base32` | `base32.encode` and `decode`, as RFC 4648 sets them. |
| `hash` | `hash.md5`, `sha1`, `sha256`, `sha512` and `hash.hmac(algorithm, key, data)`, as upper-case hex. Also `hash.crc32(data, seed)`, as an integer. |
| `utils` | `utils.datetime()`: the local time, to the microsecond, as a string. |
| `arg(name, default)` | Declares an argument that the run supplies. Use it at module level only. |

All of the names in the table above but `file` come from plugins of the runtime
itself. Each script has all of them. An import of `runtime` imports each plugin,
and each plugin registers itself. These plugins are in every script:

- `state`, `event`, `queue`, `json` and `jsonpath`.
- `time`, `math`, `regexp` and `random`.
- `hex`, `binary`, `integer` and `buf`.
- `base64`, `base32` and `hash`.
- `path` and `utils`.
- `args`, `flow` and `core`.

**`file` is not one of them, because it can read and write the disk.** A script
has it only when a host gives it. See [Paths, and the
disk](#paths-and-the-disk).

**All durations are in milliseconds**: `sleep`, `timeout`, the delay of `repeat`
and `retry`, and `event.wait`. `sleep(1000)` is one second. A duration must be a
whole number that fits in an `int32`. Then a flow can carry it without a change.
lark refuses all other values with `ErrDuration`.

### Start threads from main

**`spawn` and `timeout` are refused while a module's top level runs.** Each runs
a function on a thread of its own. The top level is the top level of the script,
a function that the top level calls, and the top level of a module that the
script loads. lark returns `ErrInit`, and starts no thread:

```python
def work():
    return 1

handle = spawn(work)    # spawn: cannot start a thread while the module loads
                        # (call spawn from main or a function main calls)

def main():
    return join(handle)
```

A module is safe to share between threads only after its top level has run and
its globals are frozen. A thread that started earlier would read a global that
the top level still writes. Call `spawn` and `timeout` from `main`, or from a
function that `main` calls. The text names the builtin: `timeout: cannot start a
thread while the module loads (call timeout from main or a function main
calls)`. A flow whose `init` holds a `spawn` or a `timeout` is refused too, and
matches `ErrInit`.

### Give a thread what it needs

`spawn` calls its function with no arguments. To give arguments to a thread,
write a lambda in the call. **The lambda takes the values when `spawn` is
called**, not when the thread starts to run:

```python
def main():
    handles = []

    for word in ["hello", "world"]:
        handles.append(spawn(lambda: shout(word)))    # each thread gets its own word

    parts = join(*handles)                            # ["HELLO", "WORLD"]
```

The compiler changes each variable that such a lambda reads into a parameter,
with the variable as its default. Thus the loop above runs `spawn(lambda
word=word: shout(word))`. A default gets its value where the lambda is written.
Without this rule, each thread would read the one `word` that the loop continues
to write. Without it, the example prints `WORLD WORLD`, and the race detector
finds a race on the variable. A comment in `v1/runtime/dialect/compile.go`
explains the rule.

**The values that a thread gets are frozen.** They are frozen in the thread and
in the function that gave them. They stay frozen after `join`. A write from
either side fails with an error, and does not cause a race:

```python
def main():
    items = []
    spawn(lambda: count(items))
    items.append(1)        # cannot append to frozen list
```

You can bind a name to a new value. You cannot change the value that you gave to
the thread. A thread returns its results when it ends. Use `state` for all
other data that threads share.

**lark refuses, at the spawn, all other ways to carry a local variable into a
thread.** The error is `ErrClosure`. It names the variable:

| Refused | Why | Write this |
| --- | --- | --- |
| `spawn(worker)`, where `worker` is a nested function that reads a local variable | a function that is spawned by name reads the variable when it runs | a top-level function, through a lambda: `spawn(lambda: worker(x))` |
| `job = lambda: f(x)` and then `spawn(job)` | only a lambda that is written in the call takes its variables | `spawn(lambda: f(x))` |
| `spawn(lambda: call(read_y))`, where `read_y` reads `y` | the thread gets the closure, and the closure still reads `y` from the function that made it | give `y` itself to the thread |

A top-level function does not need this. It can capture only what its file
loaded, and that is bound one time and frozen. Thus `spawn(worker)` works. A
lambda that is not in `spawn(...)` has its usual meaning. This is also true for
`retry(3, lambda: fetch(url))`, because a wrapper waits for its call.

### Arguments a run supplies

A script declares its arguments at module level, with or without a default:

```python
server = arg("host", "localhost")
port = arg("port", 8080)
token = arg("token")

def main():
    print("%s:%d" % (server, port), token)
```

```
lark run -args '{"host": "db.internal", "token": "t-123"}' connect.star
```

A key that no `arg()` of the script declares is a typo, not a default. Suppose
`lark run` is given a script or a flow. Then lark writes one line on standard
error for each such key. It drops the key and runs on with exit 0. An example of
the line is
`lark: argument "hosts" is not declared by the script (did you mean "host"?)`.
Suppose it is given a bundle. Then lark has no source to read the names from, so
the library refuses the key before main starts. The exit is 1, and the error
matches `ErrArgs`. Check: `lark run -args '{"hosts": "db"}' samples/args.star`
prints the line above and then runs with the defaults.

The name that a caller supplies and the name that the script binds are
**independent**. The caller supplies `"host"`, and this script calls it
`server`. Values are JSON: a string, a number, a boolean, `null`, an array or an
object. lark reads a number by the text it is written in, as the rules of
protobuf JSON do. A number written as an integer (`5432`) arrives as an `int`,
and the int is exact. It must fit an `int64`: a number outside the range stops
the run with `out of range`. A number with a fraction or an exponent (`1.5`,
`1e3`, `1.0`) arrives as a `float`. A string stays a string, and lark does not
read it as base64 or as a number. Text that is not valid UTF-8 is refused. Check:
`lark run -args '{"retries": 2}' samples/args.star` prints `retries 2`, and
`-args '{"retries": 2.0}'` prints `retries 2.0`. Then `lark run -args '{"port":
9223372036854775808}' samples/args.star` exits 1 with `script failed: an
argument holds a value JSON cannot: argument "port": 9223372036854775808 is out
of range of an int64: out of range`. A Go host that gives a `[]byte` gives bytes.
A Go host that gives a `float64` gives a float, so it writes an integer type for
an int.

**You must supply an argument that has no default.** In the example above,
nothing supplies `token`. The run stops at the declaration:

```
script failed: initialise connect.star: connect.star:3: token: argument not supplied and has no default (supply it as -args '{"token": ...}', or give arg() a default)
```

**lark drops an `arg()` that `main` cannot reach.** It treats the declaration as
it treats a constant that no code reads. Suppose `main` above did not print
`token`. Then `lark run connect.star` would run with no error. Also,
`-args '{"token": "t"}'` would write
`argument "token" is not declared by the script`. Check: put `token` in `main`,
and the run stops at the declaration.

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
lark run -args '{"port": 5432}' serve.star
time=2026-11-10T23:25:04.544+05:30 level=INFO thread=thread_0 function=main msg="listening on 5432"
```

```
lark run -args '{"port": -1}' serve.star      script failed: serve.star:3: assertion failed: "port must be positive"
```

With `-1`, the run stops at the `assert`, before `main` starts. A `fail()` at
module level does the same. Such a failure is a failure of the run, not of the
compile. Check: `lark compile -o serve.bin serve.star` exits 0. Then `lark run
-args '{"port": -1}' serve.bin` exits 1 with `bundle failed: serve.star:3:
assertion failed: "port must be positive"`.

A flow keeps the declarations in a field of their own. Thus a host can find the
arguments of a workflow without its source:

```json
"args": {
  "server": {"name": "host", "default": {"string": "localhost"}},
  "token":  {"name": "token"}
}
```

When lark generates a script from a flow, it writes the declarations back. Thus
a script whose declarations the flow carries converts to a flow and back without
a change to them. See [Which scripts convert](#which-scripts-convert).

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
stores. All threads in a run get the same store. A read of a name that nothing
wrote gives `None`. That is the usual case in a store that threads share.

**The store works like read-copy-update.** The stored values are frozen. Thus
threads that read one name at the same time never get a value that another
thread can change.

Thus `state.get` returns a **copy**. The script owns the copy
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

The store is the cache of the script. A cache holds data: `None`, a bool, a
number, a string, bytes, and containers of these. **lark refuses a function or a
thread handle** at all depths. You cannot store a list that contains a function,
as you cannot store the function.

```python
state.set("k", helper)             # refused before the script runs

def main():
    state.set("k", [1, pick()])        # refused when it runs
    state.update("k", lambda v: helper)    # refused when it runs
    state.set("k", spawn(worker))          # refused when it runs

    state.set("k", [1, {"a": (2, 3)}, None, b"x"])   # fine
```

Neither is useful, and both look as if they worked. A stored function is the
same frozen code that the script already has. A handle names a thread, and
another thread cannot do anything with it. A wait on a handle in the lock of the
store would make the store useless.

**When the source shows the value, lark refuses it at compile time**, and gives
the position:

```
state.set at build.star:5:14 stores the function helper: not data a store can hold: value cannot be converted
```

A name that the file declares, or a lambda that is written in place, is known
before anything runs. lark refuses all other values when they run. Examples are
the result of a call, and the value that the function of an update returns.

**A store of a value that is not data stops the whole run**, as a failed
assertion does. It does not stop only the thread that tried it. A thread that
nothing joins fails without a message. Its error goes into the report, but it
never becomes the result of the run. Thus, without this rule, a spawned worker
that stores a function would leave the store empty. The script would end as if
it worked.

**lark refuses three things while a name is held.** All three give `ErrNested`.
Each would cause a wait that nothing in the script can end:

| Refused | Why |
| --- | --- |
| A second `update` | Two threads that update two names in opposite orders would wait for each other forever. A script cannot take locks in an order that it cannot see |
| A `set`, in an update | It uses the same lock, so the result is the same deadlock |
| A `join` | An update takes the name before it calls its function. A join of a thread that needs that name makes each thread wait for the other |

**A thread that is spawned in an update keeps the ban for all of its life.** It
cannot `set` or `update` any name, even after the update returns. The thread
copied the ban when it started. Nothing clears a copy.

This is intentional. If lark cleared the ban at the release, one script could
succeed on one run and fail on the next. The result would depend on when its
write occurred. If a thread must lock, spawn it before the update, not in it.

A join *after* the update returns is correct. lark refuses a join while a name
is held. It does not refuse the handle.

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
was posted. It is `"timed out"` when the time of the wait ended. The wait gives
a pair and does not raise an error. A timeout is an answer that a script acts
on, not a fault. The wait does not give only the value. Then
`event.post("k", None)` and a wait that reached its time limit would give the
same answer.

**An event is a latch: a script posts it one time, and it stays posted.** A
waiter that arrives after the post does not wait. All waiters see the post.

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
same reason. Another thread reads the value. Code has no meaning for a thread
that did not write it. lark refuses a function or a handle with
`not data an event can carry`.

**When the source shows the value, lark refuses it before the script runs.** It
gives the position, as it does for the store:

```
event.post at build.star:5:15 posts the function helper: not data an event can carry: value cannot be converted
```

A function that the file declares, or a lambda that is written in place, is
known before anything runs. lark refuses all other values when the post runs.
Examples are the result of a call, and a value that the script read.

**Both refusals stop the whole run.** A failed assertion does the same, and so
does a store of a value that is not data. They do not stop only the thread that
posted. A thread that nothing joins fails without a message. Its error goes into
the report, but never becomes the result of the run. Thus, without this rule, a
spawned worker that posts two times would tell nothing. The thread that waits
for it would see only a timeout.

**A wait obeys its caller.** `timeout(2000, lambda: event.wait("never",
300000))` returns after two seconds, not after three hundred. A cancel of the
run ends all waits in it. Nothing else limits the time of a wait. The timeout
that the script gives is the limit, and it is necessary.

**Both sides make the event.** A waiter can name an event that nobody posts.
That is how a waiter can arrive first. Nothing removes an event. The event keeps
its value until the run ends. No limit applies to the number of events or to the
size of a value. See [What a run can use](#what-a-run-can-use).

An event is not a queue. It has no delete, no second post and no count of the
waiters. If a script needs these, use the store and a loop.

### Many values from one thread to another

An event tells one thing, one time. A queue passes many values, in order. One
thread pushes. Other threads pop. A queue is an object: `queue.new()` makes it,
and a script passes it to the threads it spawns.

```python
def produce(q):
    for i in range(1, 6):
        q.push({"id": i})

    q.close()

def main():
    q = queue.new()
    held = spawn(lambda: produce(q))

    seen = []

    while True:
        job, err = q.pop()

        if err == queue.CLOSED:
            break

        seen.append(job["id"])

    join(held)

    return seen    # [1, 2, 3, 4, 5]
```

| Call | What it does |
| --- | --- |
| `queue.new()` | Makes a queue that holds 64 values |
| `queue.new(n)` | Makes a queue that holds `n` values. `queue.new(0)` is `queue.new()` |
| `q.push(value)` | Adds the value at the back. It never waits. It gives `None`, or `queue.FULL` when the queue holds as many values as it can |
| `q.pop()` | Takes the value at the front, and waits until there is one. It gives `(value, None)`, or `(None, queue.CLOSED)` when the queue is closed and empty |
| `q.close()` | Wakes every waiting `pop`. A second `close` does nothing |

`queue.FULL` is the text `"full"`, and `queue.CLOSED` is the text `"closed"`.
They are in the module so that a mistake in a spelling is an error, and not a
loop that never ends.

`print(q)` shows how many values the queue holds and how many it can hold, and
`closed` once it is closed: `<queue 3/64>`, `<queue 0/64 closed>`.

**`pop` gives a pair, `(value, err)`.** This is the same as `event.wait`. `None`
is a legal value, so the value alone cannot tell a pushed `None` from a closed
queue. `err` is `None` when a value came. It is `queue.CLOSED` when the queue is
closed and every value pushed before the close was already taken.

**A queue holds a fixed number of values.** `push` never waits. A full queue
gives `queue.FULL` at once, and the producer decides what that means: wait and
try again, drop the value, or count it. lark does not drop a value for you. A
capacity of `n` must be a whole number of 0 or more, and 0 means 64. A negative
number, or a value that is not an int, stops the whole run. The queue
allocates its space as the values arrive, so a large capacity costs nothing
until the queue uses it.

**A value goes to one popper, in the order of the pushes.** Several workers that
pop one queue share the work, and none sees the value of another. This is not a
broadcast. For that, make one queue for each reader. **The pop that has waited
longest gets the next value.** Waiting pops stand in a line. A `push` wakes only
the first of them, and a pop that arrives later does not pass those that wait. A
pop that a `timeout()` or a cancel ended leaves the line and takes no value, so
no value is lost or moved. After a `close`, the values that are still in the
queue go out first, in order. Then every `pop` gives `queue.CLOSED`.

```python
def main():
    q = queue.new()
    q.push("a")
    q.close()

    q.pop()     # ("a", None)
    q.pop()     # (None, "closed")
```

**The queue keeps the value you push, with no copy.** `push` freezes the value
in place, as `event.post` does, and `pop` gives out that same frozen value. A
list that a script pushed cannot grow afterwards, and a list that it popped
cannot grow either. To change one, build a new list: `list(item) + [x]`. A push
that gives `queue.FULL` freezes the value too, so a producer that tries again
pushes the same frozen value.

**A queue carries data, and only data.** This is the rule of the store and of
events. `push` refuses a function or a handle, and a queue, with `not data a
queue can carry`. For the same reason `event.post`, `state.set` and `q.push`
refuse a queue as a value. A queue is not data.

**A push after `close` stops the whole run** with `this queue is closed`
(`queue.ErrClosed`). A producer that keeps on pushing did not hear the close. A
thread that nothing joins would fail without a message, as for a second post, so
the failure stops the run. Close a queue from the thread that knows that no
producer is left to push: usually `main`, after it joined them.

| What occurs | What a script sees |
| --- | --- |
| A push to a queue with room | `None` |
| A push to a full queue | `queue.FULL`. The queue is not changed |
| A pop of a queue with values | The first value, immediately |
| A pop of an empty queue | The wait lasts until a push, a `close`, a `timeout()` or a cancel |
| A pop of a queue that is closed and empty | `(None, queue.CLOSED)`, immediately |
| A push to a closed queue | The whole run stops: `this queue is closed` |
| A value that is not data | The whole run stops: `not data a queue can carry` |
| A `timeout()` around the pop, or a cancel of the run | The wait ends early: `timeout()` fails as it does around any call, and a cancelled run ends |
| A `pop` inside `state.update` | The call fails with `ErrNested`, as `event.wait` does |

**A pop obeys its caller, and takes no time limit.** `timeout(2000, lambda:
q.pop())` returns after two seconds, and a cancel of the run ends all pops in it.
Nothing else limits the time of a pop. If no thread will push or close, the pop
waits for ever, so a script closes its queue, or puts a `timeout()` around the
pop. A pop that was cancelled takes no value.

**A queue works when it is frozen.** A queue in a global of the module, or one
that a `spawn` captured, still takes and gives values. Its state is behind a
mutex, so two threads never change it at the same time.

**A `pop` that nothing joined is cut off when the run ends.** `main` can return
while a worker still waits in `pop`. The run does not wait for it. The values
that are left in a queue go away with the run. No limit applies to the number of
queues or to the size of a value. See [What a run can use](#what-a-run-can-use).

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
and they will not be available. They need a backtracking engine. A program can
run scripts from other persons. In that program, a backtracking engine lets a
script stop the service. lark refuses a pattern that asks for one of them. It
does not silently change the meaning. Named groups, `(?P<name>...)`, work.

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
of one artifact get independent values. All threads in a run get values from the
same stream. If the script sets no seed, lark gets a seed from `crypto/rand`.

**A seed repeats less than it seems to.** A seeded source gives one sequence.
But the order of the requests decides which thread gets which number. The
scheduler sets that order, not the script. Thus a run with one thread and a seed
repeats exactly. A concurrent run repeats the values, but not the threads that
get them.

If a thread must repeat, give it its own seed, and let only that
thread take values.

`shuffle` returns a new list. It does not change the order of the list that it
gets. The module scope freezes before concurrent work starts. A function that
works at the top of a script and fails in a thread is worse. It is worse than a
function that never changes a list.

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

It uses the **separator of the host**, through `path/filepath`. These paths go
to `file` and then to the operating system. If the module used its own
separator, the host would have to translate each path. A translation can change
what a path means. As a result, the same script sees a different separator on
Windows. Make paths with `join`, not with text, and this does not occur.

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

```go fragment
artifact, err := runtime.Compile(source, runtime.WithPlugins(file.New()))
```

A host that runs scripts from other persons does not give it. `lark` gives it to
each script that it runs, because those scripts are yours, and they run with
your permissions.

`remove` removes one file or one empty directory, and never a tree. Thus an
incorrect path causes a refusal, not the loss of many hours of work.

**`read` reads files, not devices.** lark refuses all that is not a regular
file. Examples are a character device, a pipe and a directory. The error is
`ErrRegular`, which also matches `ErrFile`. Without this rule, an endless source
such as `/dev/zero` would grow until the process stopped.

**No size limit applies to a file, but the memory must exist.** `read` allocates
the bytes and copies them into a string. Thus it uses two times the size of the
file. Before it allocates, `read` and `bytes` compare the size of the file with
the free memory. If the file is larger, they raise `not enough free memory`
(`memwatch.ErrRoom`). The check skips a file of less than 1 MiB. [What a run can
use](#what-a-run-can-use) tells how to limit the memory of a process. Check:
make a sparse file of 100 GiB with `truncate -s 100G big.dat`. Then
`file.read("big.dat")` fails with
`file.read: 107374182400 bytes: not enough free memory`.

**`lines` reads a file one line at a time.** Thus the longest line sets its
cost, not the file. You can read a log that is much too large for `read`. The
memory of the run stays small, because only the current line is held. The
maximum length of a line is the largest size that the scanner of Go accepts. It
is not a number that lark chose. A file of one very long line needs memory for
that line.

You can read `file.lines(p)` **one time**, as a Python file object. A second
pass raises `ErrWalked`. It does not silently go back to the start of the file.
lark checks the file when you call `lines`. It refuses a missing file or a
directory there. It opens the file only when the loop starts. Thus, if you name
a file and do not read it, nothing stays open.

**`writelines` puts a newline between lines and none after the last line.** Thus
what you read back is what you wrote. When `appendlines` adds to a file that
does not end in a newline, it adds the newline first. Without this, the first
new line would join the end of the last old line.

**`stat` gives all that the syscall read**: `size`, `dir`, `mode` and
`modified`. `size` and `exists` each make that call and keep one fact. Suppose a
script needs two facts and asks two times. It can get two facts that were never
true at the same time. `mode` is the permission bits as a number, as
`stat.S_IMODE` of Python gives them. Thus `info.mode & 0o700` tests one bit and
`"%o" % info.mode` prints them. `modified` is a `time` value. The `time` module
can use it without a conversion.

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
asks for, the read fails with `buf.ErrShort`. A read that fails does not move
the reader.

A buffer and a reader are not data, so the store and events refuse them. A
`spawn` freezes them, because two threads must not change one buffer. After
that, a write or a read fails with `buf.ErrFrozen`. `b.bytes()`, `len()` and
an index still work.

### Repeat, retry and limit a call

Each of these calls its function immediately, and returns the result of the
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

You cannot put one of these in another. lark refuses
`retry(3, timeout(5000, fn))`. A wrapper takes a function and returns a result,
not another function.

**`retry` tries again only after an assertion.** A `fail()`, a cancelled handle
or a refused builtin propagates immediately. That is the purpose of the two types
of failure. `assert` tells that a check was not true, possibly because of the
timing. `fail` tells that the operation cannot work.

An assertion usually ends the whole run. In a `retry`, it ends only that
attempt. lark marks the thread of the attempt so that it catches assertions.
When all attempts fail, the last assertion propagates and ends the run, as an
assertion outside a `retry` does.

**`n()` is a call, not a variable.** A plain name belongs to the *module*, and
all threads of a run share the module. A global is an index into one slice of
values. A predeclared name is a key in one map. A thread cannot have its own
value for either.

Thus a plain `n` would be one number for all attempts. Also,
two wrappers that run at the same time would read the values of each other.

A builtin gets the thread that calls it, and that thread keeps the attempt. Thus
`n()` can give the correct number. Outside a `repeat` or a `retry`, `n()` gives
an error, not a guess.

**`repeat` and `retry` run each attempt on the thread that called them.** The
thread keeps the number of the attempt. Thus two wrappers that run in parallel
never share a count. `timeout` runs its call on a goroutine of its own, so it
can stop its wait for the call.

The count is first and the function is last, because a lambda is the argument
that most often becomes long. `n` must be 1 or more.

**You can give a lambda.** Write a lambda when a call needs arguments:
`repeat(3, lambda: greet("alice"))`. The compiler reads from the source which
function the lambda calls. The graph shows the attempts under that name: here,
`greet`.

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
it cancels the handles that it did not reach. Thus a `fail` *also* stops
siblings when the failed handle is first in the join. The two differ when the
failed handle is last:

```python
join(slow, dies)   # fail:   waits the slow one out, then reports
                   # assert: cancels it immediately
```

`samples/failkinds.star` is that script. With `fail`, the run waits for the
slow thread to end its whole count. With `assert` in its place, the run ends at
once. Change the one line and use `time lark run` to see the difference.

```python
def check(value):
    if value < 0:
        fail("negative value:", value)   # this call is wrong; siblings carry on
    return value

def main():
    assert(ready(), "the fixture never came up")   # nothing can be salvaged
```

`fail` takes any number of values and joins them with spaces. Thus `fail("code",
42)` on line 2 of `build.star` gives `build.star:2: fail: code 42`. The text
starts with the position of the call.

Some errors end with a hint in parentheses, after the failing text. An example
is `build.star:2:5: undefined: prnt (did you mean print?)`. A hint gives the
usual fix. A suggestion names the one nearest name, and no name when none is
close. A misspelled command flag or verb gets the same hint:

```
flag provided but not defined: -grph (did you mean -graph?)
lark: unknown verb "compil" (did you mean compile?)
```

A step that `retry` must try again must use `assert`. A step that must stop in
all cases must use `fail`.

### The three forms of `assert`

```python
assert(x == 1)                  # stops the run when x is not 1
assert(x == 1, "x was wrong")   # the same, with a message
assert(msg = "unreachable")     # always stops it
```

lark **refuses** `assert("some text")`, and does not run it. The refusal also
stops the run. This call looks like the third form. But it would work like the
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
    join(spawn(broken), slow)   # raises: build.star:2: assertion failed: "the reason"
                                # and slow is cancelled, not waited for
```

**`join` stops at the first failure.** It waits for handles in argument order.
The first failure cancels the handles that it did not reach. Then `join` waits
for these handles, so it does not leave an evaluation that still runs.

There are two types of failure, and they differ in only one way:

| | Ends | Reported |
| --- | --- | --- |
| `assert` | the whole run | the assertion, where it occurred, joined or not |
| a refused `assert("text")` | the whole run | the refusal: an assertion that has a mistake in it is still an assertion |
| all other failures: `fail()`, a cancelled handle, a builtin that panics | the thread where it occurred | the first failure **in argument order** at the join |

Thus, for usual failures, a script reports the same failure on each run. This
does not depend on which thread lost the race. An assertion is stronger. It
stops the run, and a run has one outcome: the first assertion that lark records.
lark cancels a sibling that would fail later, before it can fail.

An assertion in a thread that **nothing joins** also fails the run. A run does
not report success while lark stops its threads.

## Cancellation

This table shows each way to stop a run, or a part of a run. For each way, it
shows:

- the error that a host or a script gets
- the status that the graph gives each function
- what `lark` prints.

Each row comes from a run of a small script, through `Start` and `Status`, and
through `lark` itself.

| Who | How | What stops | What it shows |
| --- | --- | --- | --- |
| A host | cancels the context that it gave to `Start`, `Eval` or `Run` | the whole run | `Wait` returns `ErrCancelled`. The run, and each function that still runs, shows `STATUS_CANCELLED`, with no cause |
| A builtin of a host | calls `Stop` on a `Handle` that a script gave it | that thread | a `join` of the thread raises `ErrCancelled`. Its function shows `STATUS_CANCELLED` |
| A person who runs `lark` | an interrupt, `SIGTERM` or `SIGHUP` | the whole run | `script stopped` on standard error, with `flow` or `bundle` in place of `script` for those inputs, and exit status 4 |
| A reader of `lark` standard output | closes the pipe, with `-logs` or `-hold` given | the whole run | `script stopped` on standard error and exit status 4. The `-logs` file holds every line printed, and a held spool is removed. Without either flag the process ends by `SIGPIPE` |
| A script | `cancel(h)` | that thread | a `join` of the thread raises `ErrCancelled`. The text starts with the position of the `join`: `build.star:7: work: cancelled: context canceled (read a cancelled thread with outcomes())`. Its function shows `STATUS_CANCELLED` |
| A script | `timeout(ms, f)` that runs longer than `ms` | the call that it wraps | it raises `flow.ErrTimeout`, as `build.star:5: timeout(100, slow): timed out`, with the position of the call. The wrapped function shows `STATUS_FAILED` |
| A script | a `join` that gets a failure | each handle that the join did not get to | the join raises that failure. The functions of those handles show `STATUS_CANCELLED` |
| A script | A failed `assert`. A `retry` that used all its attempts. A store, update or post of a value that is not data. A second post of one event. A push of a value that is not data. A push to a queue that is closed. A `queue.new` with a capacity that is not an int, or is negative. A `file.lines` loop whose read failed part-way | the whole run, from each thread, joined or not | the error of the run is that failure, and `ErrAssert` for the first two. The run shows `STATUS_FAILED`, and each function that still runs shows `STATUS_CANCELLED`. In a `retry`, a failed assertion stops only that attempt. The retry raises each other failure at the first attempt, and does not try again |
| The runtime | `main` returns, or raises an error | each thread that nothing joined | their functions show `STATUS_CANCELLED`. The run shows the result of `main` |

A `fail()` and each other error that a thread raises stop only that thread. A
`join` of that thread raises the error again. A thread that nothing joins
fails, but the run does not fail: its function shows `STATUS_FAILED`, and the
run succeeds. `file.lines` is the only exception. It stops the whole run,
because a loop has no place to raise its error. *[`assert` or
`fail`?](#assert-or-fail)* tells which one a script must use, and why.

For a run that failed, `lark` writes `script failed:` and the error on standard
error, and exits with status 1. It exits with status 4 only for a run that
something stopped.

A cancelled evaluation stops at its next instruction. It does **not** interrupt
a Go function that already runs. Thus a builtin that blocks must take a context
of its own.

`Wait` does not return until each thread that the run started stops. lark
cancels a handle that nothing joined, and does not wait for it, as the last row
tells. Thus a `spawn` that a script did not join cannot keep a call open.

## What a run can use

**An opt-in memory watch ends a run before the machine is out of memory.** The
lark command takes `-memory`. A library host passes `runtime.WithMemoryWatch`
one time. A nil `abandon` is no call, and the watch is on. Both are off by
default. The watch reads the memory in use on the machine (`/proc/meminfo`) and
in the cgroup v2 of the process. It takes the tighter one. At 95 percent it ends
the run the way `assert` does, with the error `memory limit reached`
(`memwatch.ErrLimit`). A Go builtin can be busy and not look at its context.
Then the run has not ended 500 ms later, and lark abandons it. In both cases the
lark command exits 5, with one line on standard error. The `-graph` patches that
the run made are in the file already. Under `-hold` they are written first. A
system without `/proc/meminfo` or cgroup v2 refuses the watch, and names the
file it could not read. Memory that another process uses counts: the watch is
about the machine, not about the share of lark.

**Without the watch, nothing limits the memory of a run, with one exception.**
lark does not count a value that a script stores. It does not count an event, a
queue or the values in it, a thread that a script spawns, or the data that the script builds. Read this
section before you run a script that you did not write.

The exception is one size that a script names. Before lark allocates it, lark
compares it with the free memory. The free memory is the least of the machine,
the cgroup and `ulimit -v`. If the size is larger, the call raises `not enough
free memory` (`memwatch.ErrRoom`), and the process goes on. This check works
with the watch off or on. It skips a size of less than 1 MiB. It does not see a
loop of small allocations.

These calls have the check:

- `hex.from_int` and `binary.from_int`.
- `integer.to_bytes`.
- `write_uint` and `write_int` of a buffer.
- `random.bytes`.
- `file.read` and `file.bytes`.
- A line of `file.lines`.

Check: `hex.from_int(0, 1 << 40)` fails with `hex.from_int: 1099511627776 bytes:
not enough free memory`. The test is `TestSizeGuard_RefusesAtEverySite` in
`v1/testing/tests/sizeguard_test.go`.

**The interpreter allocates the memory of the script itself.** This script asks
the library for nothing:

```python
def main():
    held = []

    for i in range(400000):
        held.append("a string long enough to be worth counting, number %d" % i)

    return len(held)
```

The script holds 400,000 strings and succeeds. If you increase the loop count,
the process fails when the machine has no memory left. `go.starlark.net` does
not count its own memory, so lark cannot count it either.

`base64.encode` allocates a third more than the size of its input. This script
shows the cost of the interpreter:

```python
held = "x" * 200000000       # 200MB, the interpreter's

def main():
    base64.encode(held)      # 267MB more
```

The encoding needs its memory on top of the memory that the interpreter
already holds.

**Nothing limits the computation either.** There is no step limit and no
deadline. A script can loop forever. In a test, a script ran until a person
stopped it from outside, and nothing in the runtime stopped it.

A run ends when
its context is cancelled. That is why `lark` handles an interrupt, and why a
host must give a context that it can cancel.

Recursion is the largest risk, because it stops the **process**, and not only
the run. The dialect lets a script use recursion. A recursive function that does
not stop uses all of the stack. lark recovers a builtin that panics, and the
panic becomes an error (`guard.WithRecover`). But Go cannot recover from a stack
overflow, so no recover can help.

**lark checks the depth of a value, and some paths stay open.** A script can
build a list, a tuple or a dict nested millions of levels deep in a few lines. A
Go function that walks such a value in a recursion overflows the stack. Go
cannot recover from that. So every walk of lark itself stops at 10,000 levels
with the error `value nested too deep`. The run ends as `assert` ends it. These
are the walks:

- `find_key`.
- `state.set`, `state.get` and `state.update`.
- `event.post`.
- `q.push`.
- The value and the pointer of `patch_json`.
- `json.decode`.
- The default arguments of `spawn`.
- The values sent to a plugin in another process.

`str`, `repr`, `print` and `fail` do the same check before they write a value.
The text of `str(x)` for a value nested past the limit is
`build.star:5: str: value nested too deep`. It starts with the position of the
call. A value at or under the limit gives the text that go.starlark.net gives,
byte for byte. A list or dict that holds itself is not an error: the four still
write `[...]` for it.

These paths belong to go.starlark.net, and lark cannot check them. **Each
ends the process with `fatal error: stack overflow` when a loop has built a value
millions of levels deep:**

| Path | Example | Measured with a tuple of 12 million levels |
| --- | --- | --- |
| A dict key | `{x: 1}` | stack overflow (`Tuple.Hash`) |
| A set member | `set([x])` | stack overflow |
| A membership test | `x in d` | stack overflow |
| String formatting | `"%s" % x`, `"%r" % x`, `"{}".format(x)` | stack overflow |

`json.encode` of a very deep value does not crash, but it does not return soon.
It spends time quadratic in the depth, and lark does not check it. A comparison
`x == y` is safe: go.starlark.net stops it with
`comparison exceeded maximum recursion depth`. The builtin `hash` takes a string
or bytes only. A deep value gives an ordinary type error there. (In a lark
script `hash` is the hash module.)

One more hole is about memory, not the stack. Starlark's own `list * n` and
`tuple * n` refuse a result of 2^30 elements or more. But one element is 16
bytes. Thus `[0] * (2**30 - 1)` can ask for about 16 GiB in one step.
go.starlark.net caps `str * n` and `bytes * n` at 1 GiB, but not these. Nothing
in lark refuses it up front. The opt-in `-memory` watch can end the run. We read
this limit in the go.starlark.net source (`starlark/eval.go`: `maxAlloc` and
`tupleRepeat`). We did not run it, because it would use the memory of the
machine.

The other rows of the table were measured. Each is a script that builds a tuple
of 12 million levels in a loop and then runs the line. The limit was
`ulimit -v 12000000`. The process exited with `fatal error: stack overflow`
(exit 2).

Reader check: run `lark run deep.star` with this file, and see an error
and not a crash:

```python
def main():
    x = ()
    for i in range(20000):
        x = (x,)
    print(str(x))
```

The same loop with `12000000` levels and `d = {x: 1}` instead of the last line
ends the process.

**In summary: run a script that you do not trust in a process that you can lose,
under a cgroup.** Two limits seem to help, but they do not. Both were measured
with `lark run`:

- **`GOMEMLIMIT` has no effect here.** It is a *soft* limit: it makes the
  collector work more, and it never refuses an allocation. Thus live data
  continues to grow. A `GOMEMLIMIT` below the live data of the `base64.encode`
  script above does not change what the script uses.
- **`ulimit -v` caps the memory, but a run that reaches the cap ends with a
  crash, not an error.** The crash is a Go fatal error, and the exit status is
  2. Go reserves a large virtual address space at the start. Thus a low limit
  gives `fatal error: failed to reserve page summary memory` before `main` runs.
  An example is `ulimit -v 500000`. This also occurs for a script that only
  returns `1`. lark reads this limit for its size check, as the text above says.
  A loop of small allocations gets no error. `-memory` is different: it stops
  the run cleanly, with the error `memory limit reached` and exit 5.

A memory limit on a container, cgroup `memory.max`, is the limit that works.
When a process goes above that limit, the kernel stops the process. Go accepts
that, and this runtime cannot do it for itself.

**A counter in the runtime would not close this gap.** `go.starlark.net` has no
allocation hook: `SetMaxExecutionSteps` and `SetLocal` are the only setters on a
thread. Thus no part of this runtime knows when a script appends to a list. To
close the gap correctly, the interpreter must count memory.

There is no decision yet about a step limit or a deadline.
`SetMaxExecutionSteps` would limit the loop, but not the stack and not the
memory.

## A bundle is the program

`lark compile` compiles and does not run. It writes one file. The file holds the
name of the entry unit and all compiled units, in the order of their
dependencies. `-o` names the file, and is required. `lark run` runs a bundle,
which it knows by the extension `.bin`:

```
lark compile -o build.bin build.star
lark run build.bin
```

When the bundle is written, `lark` says so on standard error:
`lark: wrote "build.bin"`. If the bundle cannot be written, `lark` prints one
line and exits 3. An example is a directory that is not there:
`lark: bundle "x/build.bin": no such file or directory`. A bundle is not a
plugin. A plugin is `lark-*.bin` in the `-plugins` directory, so do not name a
bundle `lark-*.bin` there. `lark run` knows a bundle by `.bin`. A bundle that
has another name is read as a script. `lark compile` and `lark convert` do not
take a bundle, because it has no source.

The units are already compiled, so lark does not parse or check them again. The
run gets the plugins that `lark` compiles with. A bundle is a program. Thus lark
refuses to write a module, which is a script with no `main`. A host does the
same with `Save`, `Load` and `Start`. See the section Compile and Load in the
package comment of `v1/runtime` ([Use it from Go](#use-it-from-go)).

A bundle holds no picture of the workflow. The picture is two items, and neither
item goes in a bundle. A **flow** tells what a script will do: its functions,
the statements in them, and `main`. When you ask for a flow, lark derives it from
the source:

```
lark convert build.star > build.json
```

A **graph** tells what a run did, and comes from the run. See the section
Execution in the package comment of `v1/runtime` ([Use it from
Go](#use-it-from-go)). Nothing changes a flow into a graph of threads that are
not started yet. A flow does not know which threads a run will make.

A flow cannot state some lines, for example a library call, a `return`, or a
`for` over a list. A function that has such a line goes into the flow whole, as
the text that it was written in. A flow that holds such a function still
generates its script exactly. If the flow came first, `lark compile` compiles the
script that the flow generates:

```
lark compile -o build.bin build.json
```

## Keeping a run's output

A script prints to standard output. `-logs DIR` also keeps the lines in
`DIR/NAME.log`, where NAME is the file name of the script. A second run of the
same script replaces the file. Each line tells when the script printed it, on
which thread, from which function, and then the text:

```
lark run -logs logs build.star
```

```
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_0 function=main msg="from the spine"
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_2 function=beta msg="from beta"
time=2026-10-02T16:49:01.940+05:30 level=INFO thread=thread_1 function=alpha msg="from alpha"
```

The thread is important because the lines of a concurrent script mix. The three
lines above also came in that order on standard output. Without the thread, the
file could not tell which thread printed which line.

A host that embeds the runtime names the writer itself, with `WithLog`. See the
section Logs in the package comment of `v1/runtime` ([Use it from
Go](#use-it-from-go)).

## The dialect

The dialect lets a script use sets, `while` loops and recursion. It does not let
a script assign a new value to a top-level name. A workflow freezes its globals,
so a new assignment would fail at run time. lark refuses it at compile time.
Thus it reports the same mistake with its position.

lark freezes the module scope after the module initialises. Thus threads can
share the module scope safely. A script that changes a global from in a function
fails with an error, and does not corrupt memory.

One rule is not an option of the interpreter. lark compiles a lambda that is
written in `spawn(...)` with a parameter for each variable that the lambda
reads. The default of each parameter is that variable. Thus the lambda gets the
values at the spawn. See *[Give a thread what it
needs](#give-a-thread-what-it-needs)*.

lark does not freeze a local variable as
it freezes the module scope. Without this rule, a closure could give a local
variable to another goroutine.

## What this does not do

- **Nothing limits the steps or the time of a run, and the memory limit is off
  by default.** The `-memory` watch ends a run at 95 percent memory in use.
  Without it, lark checks only a size that a script names. A script that builds
  a very large list uses the memory of the machine. There is no step limit and
  no deadline. A recursive script that does not stop also stops the process. Go
  cannot recover from a stack overflow, as it can recover from a panic in a
  builtin. A very deep value as a dict key, a set member or an argument of
  string formatting does the same. See *[What a run can
  use](#what-a-run-can-use)*.
- **Nothing runs a flow directly, and this is a decision.** A flow becomes a
  script, and the script runs. There is one path, not two, so two paths cannot
  disagree about the meaning of a flow. A second interpreter would have that
  cost.
- **Nothing stays after the process restarts**, and lark opens no network
  connection of its own. Plugins under `-plugins` run as local processes. They
  talk to lark over a local socket.

## Licence

MIT. See [LICENSE](LICENSE).
