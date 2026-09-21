# A workflow.Graph using every step kind and every argument kind, and the
# Starlark that runtime/graph generates from it. The code below this comment is
# that generator's output, pasted unedited - a test reads the graph back out of
# this comment, generates from it, and fails if the two ever disagree.
#
# Every thread says what it runs in its first step, the spine included: its
# first step calls main, and the steps after it are main's own body. A thread
# with only that one step is a fork pointing at a leaf, and the leaf keeps its
# text.
#
# Read the JSON as two halves. functions is the code: a name, its parameters,
# and a body of statements. threads is the workflow: what runs, on which
# thread, in what order. A function no thread runs is a helper - greet is
# called by two spawned threads, and every other leaf by a step - and helpers
# never appear in a status report, because a report is of steps.
#
# Every step kind, in the order the spine performs them:
#
#   Fork      h1 = spawn(lambda: greet("alice"))
#   Join      join(h1, h2)
#   Call      record("ada", 36, True, ["x", 1.5], {"k": 1}, None)
#   Repeat    repeat(3, tick)
#   Retry     retry(5, flaky)
#   Sleep     sleep(0.02)
#   Timeout   timeout(2, settle)
#   If        if both_greeted(): ... else: ...
#   Match     _match = kind(); if _match == "alpha": ...
#
# Cancel is the tenth and is not here, because this script cancels nothing -
# see samples/cancel.star. Every builtin has a message of its own, so a step
# that names a thread carries a list of thread ids rather than values something
# has to read back as strings.
#
# All six argument kinds are in that one Call, and they keep their JSON kinds:
# a string, a number, a bool, a list, an object and null become "ada", 36,
# True, ["x", 1.5], {"k": 1} and None.
#
# Six things worth matching back to the JSON:
#
#   spawn(lambda: ...)   a Call that passes arguments becomes a lambda, because
#                        spawn takes none to pass on. A site without them is
#                        the bare name - see timeout(2, settle).
#
#   h1, h2               a spawn names a thread by id, and the handle is that
#                        id with its prefix swapped - thread_1 is h1. What runs
#                        there is that thread's own first step.
#
#   repeat(3, tick)      the count comes first and the callable last. It calls
#                        straight away: the wrappers are not factories.
#
#   sleep(0.02)          the schema counts milliseconds and the builtins take
#                        seconds, so 20 becomes 0.02 and 2000 becomes 2 - an
#                        integer, because Starlark has two number types where
#                        JSON has one. One unit throughout, so a reader never
#                        checks which a duration is in.
#
#   36, not 36.0         the same rule for an argument. A whole number is an
#                        integer, and // and % treat the two differently.
#
#   _match = kind()      a Match evaluates its expression once, into a local.
#                        Calling it per case would be a different program.
#
# It prints its results and returns None. The None is not an oversight: no step
# expresses a return. A generated main performs its steps and gives nothing
# back, so a graph describes what a workflow does and not what it answers -
# which is why this one ends with a step that prints.
#
# graph:
#   {
#     "functions": [
#       {
#         "body": "state.update(\"greeted\", lambda s: 1 if s == None else s + 1)\n\nreturn \"hello \" + who",
#         "name": "greet",
#         "params": [
#           "who"
#         ]
#       },
#       {
#         "body": "state.set(\"record\", [name, age, admin, tags, meta, note])",
#         "name": "record",
#         "params": [
#           "name",
#           "age",
#           "admin",
#           "tags",
#           "meta",
#           "note"
#         ]
#       },
#       {
#         "body": "state.update(\"ticks\", lambda t: 1 if t == None else t + 1)\n\nreturn n()",
#         "name": "tick"
#       },
#       {
#         "body": "assert(n() \u003e= 2, \"not ready on attempt \" + str(n()))\n\nreturn \"ready\"",
#         "name": "flaky"
#       },
#       {
#         "body": "sleep(0.01)\n\nreturn \"settled\"",
#         "name": "settle"
#       },
#       {
#         "body": "return state.get(\"greeted\") == 2",
#         "name": "both_greeted"
#       },
#       {
#         "body": "state.set(\"branch\", \"announced\")",
#         "name": "announce"
#       },
#       {
#         "body": "state.set(\"branch\", \"hushed\")",
#         "name": "hush"
#       },
#       {
#         "body": "return \"beta\"",
#         "name": "kind"
#       },
#       {
#         "body": "state.set(\"matched\", \"alpha\")",
#         "name": "on_alpha"
#       },
#       {
#         "body": "state.set(\"matched\", \"beta\")",
#         "name": "on_beta"
#       },
#       {
#         "body": "state.set(\"matched\", \"other\")",
#         "name": "on_other"
#       },
#       {
#         "body": "print(\"greeted\", state.get(\"greeted\"), \"| ticks\", state.get(\"ticks\"),\n      \"| branch\", state.get(\"branch\"), \"| matched\", state.get(\"matched\"),\n      \"| record\", state.get(\"record\"))",
#         "name": "report"
#       },
#       {
#         "name": "main"
#       }
#     ],
#     "threads": [
#       {
#         "id": "thread_0",
#         "static": {
#           "steps": [
#             {
#               "call": {
#                 "function": "main"
#               }
#             },
#             {
#               "fork": {
#                 "thread": "thread_1"
#               }
#             },
#             {
#               "fork": {
#                 "thread": "thread_2"
#               }
#             },
#             {
#               "join": {
#                 "threads": [
#                   "thread_1",
#                   "thread_2"
#                 ]
#               }
#             },
#             {
#               "call": {
#                 "args": [
#                   "ada",
#                   36,
#                   true,
#                   [
#                     "x",
#                     1.5
#                   ],
#                   {
#                     "k": 1
#                   },
#                   null
#                 ],
#                 "function": "record"
#               }
#             },
#             {
#               "repeat": {
#                 "call": {
#                   "function": "tick"
#                 },
#                 "count": 3
#               }
#             },
#             {
#               "retry": {
#                 "attempts": 5,
#                 "call": {
#                   "function": "flaky"
#                 }
#               }
#             },
#             {
#               "sleep": {
#                 "durationMs": 20
#               }
#             },
#             {
#               "timeout": {
#                 "call": {
#                   "function": "settle"
#                 },
#                 "timeoutMs": 2000
#               }
#             },
#             {
#               "if": {
#                 "condition": {
#                   "call": {
#                     "function": "both_greeted"
#                   }
#                 },
#                 "else": {
#                   "function": "hush"
#                 },
#                 "then": {
#                   "function": "announce"
#                 }
#               }
#             },
#             {
#               "match": {
#                 "cases": [
#                   {
#                     "call": {
#                       "function": "on_alpha"
#                     },
#                     "value": "alpha"
#                   },
#                   {
#                     "call": {
#                       "function": "on_beta"
#                     },
#                     "value": "beta"
#                   }
#                 ],
#                 "default": {
#                   "function": "on_other"
#                 },
#                 "expression": {
#                   "call": {
#                     "function": "kind"
#                   }
#                 }
#               }
#             },
#             {
#               "call": {
#                 "function": "report"
#               }
#             }
#           ]
#         }
#       },
#       {
#         "id": "thread_1",
#         "static": {
#           "steps": [
#             {
#               "call": {
#                 "args": [
#                   "alice"
#                 ],
#                 "function": "greet"
#               }
#             }
#           ]
#         }
#       },
#       {
#         "id": "thread_2",
#         "static": {
#           "steps": [
#             {
#               "call": {
#                 "args": [
#                   "bob"
#                 ],
#                 "function": "greet"
#               }
#             }
#           ]
#         }
#       }
#     ]
#   }

# Generated from the graph above:

def greet(who):
    state.update("greeted", lambda s: 1 if s == None else s + 1)

    return "hello " + who
    pass

def record(name, age, admin, tags, meta, note):
    state.set("record", [name, age, admin, tags, meta, note])
    pass

def tick():
    state.update("ticks", lambda t: 1 if t == None else t + 1)

    return n()
    pass

def flaky():
    assert(n() >= 2, "not ready on attempt " + str(n()))

    return "ready"
    pass

def settle():
    sleep(0.01)

    return "settled"
    pass

def both_greeted():
    return state.get("greeted") == 2
    pass

def announce():
    state.set("branch", "announced")
    pass

def hush():
    state.set("branch", "hushed")
    pass

def kind():
    return "beta"
    pass

def on_alpha():
    state.set("matched", "alpha")
    pass

def on_beta():
    state.set("matched", "beta")
    pass

def on_other():
    state.set("matched", "other")
    pass

def report():
    print("greeted", state.get("greeted"), "| ticks", state.get("ticks"),
          "| branch", state.get("branch"), "| matched", state.get("matched"),
          "| record", state.get("record"))
    pass

def main():
    h1 = spawn(lambda: greet("alice"))
    h2 = spawn(lambda: greet("bob"))
    join(h1, h2)
    record("ada", 36, True, ["x", 1.5], {"k": 1}, None)
    repeat(3, tick)
    retry(5, flaky)
    sleep(0.02)
    timeout(2, settle)
    if both_greeted():
        announce()
    else:
        hush()
    _match = kind()
    if _match == "alpha":
        on_alpha()
    elif _match == "beta":
        on_beta()
    else:
        on_other()
    report()
    pass
