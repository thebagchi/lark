# A workflow.Flow using every statement kind but cancel, and every kind of
# argument, and the Starlark that runtime/graph generates from it. The code
# below this comment is that generator's output, pasted unedited - a test reads
# the flow back out of this comment, generates from it, and fails if the two
# ever disagree.
#
# Read the JSON as three parts. functions is the code: a name, its parameters,
# and either a body of text or a list of statements. constants is what the
# module binds before main runs. main is the spine: the statements main
# performs, in order. Every function here is text, because each of its lines
# is a library call or a return, and a flow states neither. main is
# statements, because every line of it is one.
#
# Every statement kind, in the order main performs them:
#
#   Spawn     h1 = spawn(lambda: greet("alice"))
#             h2 = spawn(lambda: greet(GREETING))
#   Join      join(h1, h2)
#   Call      record("ada", 36, True, ["x", 1.5], {"k": 1}, None)
#   Repeat    repeat(3, tick, 5)
#   Retry     retry(5, flaky)
#   Loop      for _ in range(2): wave()
#   Sleep     sleep(20)
#   Timeout   timeout(2000, settle)
#   If        if both_greeted(): ... else: ...
#   Match     _match = kind(); if _match == "alpha": ...
#
# Cancel is the eleventh and is not here, because this script cancels nothing -
# see samples/cancel.star.
#
# A call's arguments are values under args when every one is a literal, and
# operands when any is a name: h2 greets GREETING, a constant the flow
# declares, and passes the name rather than its value - greet(GREETING). The
# six kinds of value keep their JSON kinds: a string, a number, a bool, a list,
# an object and null become "ada", 36, True, ["x", 1.5], {"k": 1} and None.
#
# Things worth matching back to the JSON:
#
#   h1 = spawn(...)      a spawn's binding is the name a join waits on. A flow
#                        does not predict thread ids - a run assigns them - and
#                        the graph of a run matches join(h1) to its thread by
#                        that binding.
#
#   spawn(lambda: ...)   a call that passes arguments becomes a lambda, because
#                        spawn takes none to pass on. A call without them is the
#                        bare name - see timeout(2000, settle).
#
#   repeat(3, tick, 5)   the count first, the callable second, and the delay in
#                        milliseconds third, waited between calls. It calls
#                        straight away: the wrappers are not factories.
#
#   sleep(20)            a script and the schema both count milliseconds, so
#                        durationMs 20 is sleep(20) and nothing converts.
#
#   36, not 36.0         a whole number is an integer, and // and % treat the
#                        two differently.
#
#   _match = kind()      a match evaluates its expression once, into a local.
#                        Calling it per case would be a different program.
#
#   pass                 every suite that does not end in return closes with
#                        one, so where a block ends is not left to indentation
#                        alone. The flow does not store it.
#
# It prints its results and returns None: main is statements, and no statement
# returns.
#
# flow:
#   {
#     "constants": {
#       "GREETING": {
#         "value": "bob"
#       }
#     },
#     "functions": [
#       {
#         "body": "state.update(\"greeted\", lambda s: 1 if s == None else s + 1)\nreturn \"hello \" + who",
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
#         "body": "state.update(\"ticks\", lambda t: 1 if t == None else t + 1)\nreturn n()",
#         "name": "tick"
#       },
#       {
#         "body": "assert(n() >= 2, \"not ready on attempt \" + str(n()))\nreturn \"ready\"",
#         "name": "flaky"
#       },
#       {
#         "body": "state.update(\"waves\", lambda w: 1 if w == None else w + 1)",
#         "name": "wave"
#       },
#       {
#         "body": "sleep(10)\nreturn \"settled\"",
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
#         "body": "print(\"greeted\", state.get(\"greeted\"), \"| ticks\", state.get(\"ticks\"),\n      \"| waves\", state.get(\"waves\"), \"| branch\", state.get(\"branch\"),\n      \"| matched\", state.get(\"matched\"), \"| record\", state.get(\"record\"))",
#         "name": "report"
#       }
#     ],
#     "main": {
#       "statement": [
#         {
#           "spawn": {
#             "binding": "h1",
#             "call": {
#               "args": [
#                 "alice"
#               ],
#               "function": "greet"
#             }
#           }
#         },
#         {
#           "spawn": {
#             "binding": "h2",
#             "call": {
#               "function": "greet",
#               "operands": [
#                 {
#                   "name": "GREETING"
#                 }
#               ]
#             }
#           }
#         },
#         {
#           "join": {
#             "bindings": [
#               "h1",
#               "h2"
#             ]
#           }
#         },
#         {
#           "call": {
#             "args": [
#               "ada",
#               36,
#               true,
#               [
#                 "x",
#                 1.5
#               ],
#               {
#                 "k": 1
#               },
#               null
#             ],
#             "function": "record"
#           }
#         },
#         {
#           "repeat": {
#             "call": {
#               "function": "tick"
#             },
#             "count": 3,
#             "delayMs": 5
#           }
#         },
#         {
#           "retry": {
#             "attempts": 5,
#             "call": {
#               "function": "flaky"
#             }
#           }
#         },
#         {
#           "loop": {
#             "body": {
#               "call": {
#                 "function": "wave"
#               }
#             },
#             "times": {
#               "literal": 2
#             }
#           }
#         },
#         {
#           "sleep": {
#             "durationMs": 20
#           }
#         },
#         {
#           "timeout": {
#             "call": {
#               "function": "settle"
#             },
#             "timeoutMs": 2000
#           }
#         },
#         {
#           "if": {
#             "condition": {
#               "call": {
#                 "function": "both_greeted"
#               }
#             },
#             "else": {
#               "call": {
#                 "function": "hush"
#               }
#             },
#             "then": {
#               "call": {
#                 "function": "announce"
#               }
#             }
#           }
#         },
#         {
#           "match": {
#             "cases": [
#               {
#                 "statement": {
#                   "call": {
#                     "function": "on_alpha"
#                   }
#                 },
#                 "value": "alpha"
#               },
#               {
#                 "statement": {
#                   "call": {
#                     "function": "on_beta"
#                   }
#                 },
#                 "value": "beta"
#               }
#             ],
#             "default": {
#               "call": {
#                 "function": "on_other"
#               }
#             },
#             "expression": {
#               "call": {
#                 "function": "kind"
#               }
#             }
#           }
#         },
#         {
#           "call": {
#             "function": "report"
#           }
#         }
#       ]
#     }
#   }

# Generated from the flow above:

def greet(who):
    state.update("greeted", lambda s: 1 if s == None else s + 1)
    return "hello " + who

def record(name, age, admin, tags, meta, note):
    state.set("record", [name, age, admin, tags, meta, note])
    pass

def tick():
    state.update("ticks", lambda t: 1 if t == None else t + 1)
    return n()

def flaky():
    assert(n() >= 2, "not ready on attempt " + str(n()))
    return "ready"

def wave():
    state.update("waves", lambda w: 1 if w == None else w + 1)
    pass

def settle():
    sleep(10)
    return "settled"

def both_greeted():
    return state.get("greeted") == 2

def announce():
    state.set("branch", "announced")
    pass

def hush():
    state.set("branch", "hushed")
    pass

def kind():
    return "beta"

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
          "| waves", state.get("waves"), "| branch", state.get("branch"),
          "| matched", state.get("matched"), "| record", state.get("record"))
    pass
GREETING = "bob"

def main():
    h1 = spawn(lambda: greet("alice"))
    h2 = spawn(lambda: greet(GREETING))
    join(h1, h2)
    record("ada", 36, True, ["x", 1.5], {"k": 1}, None)
    repeat(3, tick, 5)
    retry(5, flaky)
    for _ in range(2):
        wave()
        pass
    sleep(20)
    timeout(2000, settle)
    if both_greeted():
        announce()
        pass
    else:
        hush()
        pass
    _match = kind()
    if _match == "alpha":
        on_alpha()
        pass
    elif _match == "beta":
        on_beta()
        pass
    else:
        on_other()
        pass
    report()
    pass
