package tests

import (
	_ "github.com/thebagchi/lark/v1/runtime/plugin/core"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
)

// The runtime's own builtins and the flow wrappers arrive by import, as they
// do for a host. Nothing else has to be registered by hand.

const (
	// SLOW is a script whose main spawns one thread that sleeps thirty seconds, then joins it.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/slow.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SLOW = `
def slow():
    sleep(30000)

def main():
    h = spawn(slow)
    join(h)
`

	// QUICK is a script whose main returns 7. RESULT is its value.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/quick.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	QUICK = `
def main():
    return 7
`

	// FAILING is a script whose main fails an assertion.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/failing.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FAILING = `
def main():
    assert(False, "no good")
`

	// WATCHED is a run wide enough to change from several goroutines at once,
	// with a retry so one function reaches running more than once.
	//
	// Revisions:
	//   - 2026-09-23 22:46: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: inlined the script from testdata/watched.star and moved from
	//     changes_test.go (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WATCHED = `
# A run wide enough that it reports from several goroutines at once, and with
# a retry in it so that one node reaches RUNNING more than once.

def flaky():
    assert(n() == 3, "not ready yet")
    return "ready"

def lane():
    return retry(3, flaky)

def main():
    return join(spawn(lane), spawn(lane), spawn(lane))
`

	// BRANCHING is a script whose main spawns alpha and beta and joins both. Its
	// function unreached is never called.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/branching.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BRANCHING = `
def alpha():
    return 1

def beta():
    return 2

def unreached():
    return 0

def main():
    a = spawn(alpha)
    b = spawn(beta)
    join(a, b)
`

	// RETRYING is a script whose main retries a function that succeeds on the third try.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/retrying.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	RETRYING = `
def flaky():
    assert(n() == 3, "not ready yet")
    return "ready"

def main():
    return retry(3, flaky)
`

	// REPEATING is a script whose main repeats a fifty millisecond step three times.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/repeating.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	REPEATING = `
def step():
    sleep(50)
    return n()

def main():
    return repeat(3, step)
`

	// RAISING is a script whose main fails with "this one raises".
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/raising.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	RAISING = `
def main():
    fail("this one raises")
`

	// PRINTS is a script that prints from the spine and from two spawns, so
	// SPEAKERS threads print.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: holds the script text, inlined from testdata/prints.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PRINTS = `
# Prints from the spine and from two spawned threads, so a transcript has
# three lanes in it.

def alpha():
    print("from alpha")

def beta():
    print("from beta")

def main():
    print("from the spine")

    join(spawn(alpha), spawn(beta))
`
)
