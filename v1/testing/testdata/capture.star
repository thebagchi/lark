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
