package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	EXPECTED       = "written by one thread, read by another"
	EXPECTED_TOTAL = "800"
	FIRST_RUN      = "1"

	// PROMPT is how long a run that should end at once may take on a loaded
	// machine before it is called hung.
	//
	// Revisions:
	//   - 2026-09-21 10:00: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PROMPT = 1500 * time.Millisecond
)

const (
	// AFTERUPDATE is the text of the module afterupdate.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	AFTERUPDATE = `
# A thread spawned inside an update, reaching the store after the update has
# returned.
#
# The child copied the mark at birth and nothing clears a copy, so its set is
# refused for the rest of its life. It cannot be joined - an update's function
# returns data now, so the handle cannot leave - and an unjoined failure does
# not reach the run, so what it did is read from what it printed.

def child():
    sleep(50)

    print("child reached the store")

    state.set("b", 1)

    print("child stored")

def spawner(v):
    spawn(child)

    return "spawned"

def main():
    state.update("a", spawner)

    sleep(300)
`

	// ATOMIC_SCRIPT is the text of the module atomic.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ATOMIC_SCRIPT = `
def bump():
    for i in range(200):
        state.update("count", lambda n: n + 1)

def main():
    state.set("count", 0)

    join(spawn(bump), spawn(bump), spawn(bump), spawn(bump))

    return state.get("count")
`

	// CONTENDED is the text of the module contended.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	CONTENDED = `
def holds():
    state.update("k", lambda v: sleep(30000))

def waits():
    state.update("k", lambda v: 1)

def main():
    spawn(holds)
    sleep(50)
    spawn(waits)
    assert(False, "end the run while waits is blocked on the lock")
`

	// COUNT_SCRIPT is the text of the module count.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	COUNT_SCRIPT = `
def main():
    seen = state.get("runs")

    if seen == None:
        seen = 0

    state.set("runs", seen + 1)

    return state.get("runs")
`

	// CYCLIC_SCRIPT is the text of the module cyclic.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	CYCLIC_SCRIPT = `
def main():
    looping = [1]
    looping.append(looping)

    state.set("looping", looping)

    got = state.get("looping")
    got.append(2)

    shared = [9]
    state.set("twice", [shared, shared])

    pair = state.get("twice")
    pair[0].append(8)

    return [len(got), pair[1]]
`

	// JOIN_UNDER_LOCK is the text of the module join_under_lock.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	JOIN_UNDER_LOCK = `
# Joining while a name is held. The update takes "k" before it calls its
# function, so a join inside that function waits for a thread that may need
# "k" - and nothing in a script can end that wait.
#
# Refused at the join. The child never runs.

def bump():
    state.update("k", lambda v: 1)

def main():
    state.update("k", lambda v: join(spawn(bump)))
`

	// JOINAFTER is the text of the module joinafter.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	JOINAFTER = `
# Joining once the update has returned. The refusal is about a name being
# held, not about the handle, so this is legal and must stay so.

def worker():
    return "joined"

def main():
    held = spawn(worker)

    state.update("k", lambda v: 1)

    return join(held)[0]
`

	// MISSING_SCRIPT is the text of the module missing.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MISSING_SCRIPT = `
def main():
    # Nothing is stored, so the update sees None and decides for itself.
    return state.update("fresh", lambda n: 0 if n == None else n + 1)
`

	// MUTATE_SCRIPT is the text of the module mutate.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MUTATE_SCRIPT = `
def writer():
    state.set("findings", ["one", "two"])

def reader():
    # A copy, so this is the reader's own list.
    found = state.get("findings")

    found.append("three")

    return found

def main():
    join(spawn(writer))

    return join(spawn(reader))[0]
`

	// NESTED_SCRIPT is the text of the module nested.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NESTED_SCRIPT = `
def inner(current):
    return state.update("other", lambda n: 1)

def main():
    return state.update("count", inner)
`

	// NESTED_THROUGH_TIMEOUT is the text of the module nested_through_timeout.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NESTED_THROUGH_TIMEOUT = `
def again():
    return state.update("k", lambda v: 2)

def main():
    return state.update("k", lambda v: timeout(1000, again))
`

	// RACY is the text of the module racy.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RACY = `
# The same work through get and set, which is two operations with a gap.
def bump():
    for i in range(200):
        state.set("count", state.get("count") + 1)

def main():
    state.set("count", 0)

    join(spawn(bump), spawn(bump), spawn(bump), spawn(bump))

    return state.get("count")
`

	// SAMEKEY_SCRIPT is the text of the module samekey.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SAMEKEY_SCRIPT = `
# An update inside an update, on the same name. Without the refusal this waits
# on a lock its own caller holds, and the run never ends.

def inner(current):
    return state.update("count", lambda n: 1)

def main():
    return state.update("count", inner)
`

	// SETINSIDE is the text of the module setinside.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SETINSIDE = `
# A set from inside an update is a nested update, and must be refused rather
# than wait for a lock this evaluation already holds.

def inner(current):
    state.set("k", "never stored")

    return current

def main():
    return state.update("k", inner)
`

	// SETWINS is the text of the module setwins.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SETWINS = `
# A set that lands while an update's function is running must not be lost.
#
# The update reads the value, posts "reading", then sleeps inside its
# function, and stores what it read. The writer waits for "reading" and then
# sets. So the set is always made after the update's read and while the update
# holds the name. Without the name's lock the update's store lands second and
# the set is silently overwritten.
#
# The event, not a sleep, orders the two: a writer that started first on a
# loaded machine would otherwise set before the update read, and the update
# would then win for a reason that is no fault of the store.

def slow(current):
    event.post("reading", True)
    sleep(50)

    return "from-update"

def updater():
    state.update("k", slow)

def writer():
    event.wait("reading", 5000)
    state.set("k", "from-set")

def main():
    join(spawn(updater), spawn(writer))

    return state.get("k")
`

	// SHARE_SCRIPT is the text of the module share.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SHARE_SCRIPT = `
def writer():
    state.set("message", "written by one thread, read by another")

def reader():
    return state.get("message")

def main():
    join(spawn(writer))

    return join(spawn(reader))[0]
`

	// SPAWNSTORE is the text of the module spawnstore.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPAWNSTORE = `
# A thread nobody joins, storing something that is not data.
#
# Its own failure would reach the report and never become the run's result, so
# without the whole run stopping this script would print "run survived" and
# leave nothing in the store, saying nothing about why.

def helper():
    return 1

def pick():
    return helper

def bad():
    state.set("k", pick())

def main():
    spawn(bad)

    sleep(100)

    print("run survived")
`

	// TUPLE is the text of the module tuple.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TUPLE = `
def main():
    state.set("pair", (1, 2))
    state.set("nested", [("a", 1), {"k": ("b", 2)}])

    return [state.get("pair"), state.get("nested")]
`

	// UNPUBLISHED_SCRIPT is the text of the module unpublished.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	UNPUBLISHED_SCRIPT = `
def main():
    state.set("findings", ["one"])

    mine = state.get("findings")
    mine.append("two")

    return [state.get("findings"), mine]
`

	// UNSET_SCRIPT is the text of the module unset.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	UNSET_SCRIPT = `
# Reads what nothing has written, which is the ordinary case in a store threads
# share rather than an error.

def main():
    return state.get("never written")
`
)

var (
	// MODULES maps each module name to its text. The in-memory loader reads it
	// where the tests once read testdata.
	//
	// Revisions:
	//   - 2026-11-09 11:16: initial creation (C114, Q281)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MODULES = map[string]string{
		"afterupdate.star":            _Script(AFTERUPDATE),
		"atomic.star":                 _Script(ATOMIC_SCRIPT),
		"bench_contend.star":          _Script(BENCH_CONTEND),
		"bench_getmany.star":          _Script(GETMANY_SCRIPT),
		"bench_setget.star":           _Script(SETGET_SCRIPT),
		"bench_threads.star":          _Script(THREADS_SCRIPT),
		"contended.star":              _Script(CONTENDED),
		"count.star":                  _Script(COUNT_SCRIPT),
		"cyclic.star":                 _Script(CYCLIC_SCRIPT),
		"join_under_lock.star":        _Script(JOIN_UNDER_LOCK),
		"joinafter.star":              _Script(JOINAFTER),
		"missing.star":                _Script(MISSING_SCRIPT),
		"mutate.star":                 _Script(MUTATE_SCRIPT),
		"nested.star":                 _Script(NESTED_SCRIPT),
		"nested_through_timeout.star": _Script(NESTED_THROUGH_TIMEOUT),
		"racy.star":                   _Script(RACY),
		"samekey.star":                _Script(SAMEKEY_SCRIPT),
		"setinside.star":              _Script(SETINSIDE),
		"setwins.star":                _Script(SETWINS),
		"share.star":                  _Script(SHARE_SCRIPT),
		"spawnstore.star":             _Script(SPAWNSTORE),
		"tuple.star":                  _Script(TUPLE),
		"unpublished.star":            _Script(UNPUBLISHED_SCRIPT),
		"unset.star":                  _Script(UNSET_SCRIPT),
	}
)

// _Memory is a Loader over MODULES.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation, in place of _Disk (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Memory struct{}

// Resolve reads target as a module beside from.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Memory) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text of the module at name.
//
// Revisions:
//   - 2026-11-09 11:16: initial creation (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Memory) Load(name string) ([]byte, error) {
	text, ok := MODULES[name]
	if !ok {
		return nil, fmt.Errorf(
			"module %s: %w",
			name,
			fs.ErrNotExist,
		)
	}

	return []byte(text), nil
}

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-20 00:36: initial creation
//   - 2026-11-07 23:04: returns common.Artifact, an interface (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads MODULES, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Built(
	t *testing.T,
	name string,
) common.Artifact {
	t.Helper()

	src, ok := MODULES[name]
	if !ok {
		t.Fatalf(
			"no fixture %s",
			name,
		)
	}

	built, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   []byte(src),
		Loader: &_Memory{},
	})
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	return built
}

// TestState_IsSharedBetweenThreadsOfOneRun proves what the store is for: module
// scope is frozen, so two spawned threads have no other way to pass anything.
//
// Revisions:
//   - 2026-09-20 00:37: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_IsSharedBetweenThreadsOfOneRun(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"share.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	text, ok := value.(starlark.String)
	if !ok {
		t.Fatalf(
			"got %T, want starlark.String",
			value,
		)
	}

	if string(text) != EXPECTED {
		t.Fatalf(
			"got %q, want %q",
			string(text),
			EXPECTED,
		)
	}
}

// TestState_IsPerExecutionNotPerCompile proves a second run of one artifact
// starts with an empty store.
//
// The script counts how many times it has run by reading, incrementing and
// writing. Shared across runs it would report 1 then 2; scoped to an execution
// it reports 1 twice.
//
// Revisions:
//   - 2026-09-20 00:38: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_IsPerExecutionNotPerCompile(t *testing.T) {
	built := _Built(
		t,
		"count.star",
	)

	for attempt := range 2 {
		run, err := runtime.Start(
			t.Context(),
			built,
		)
		if err != nil {
			t.Fatal(err)
		}

		value, err := run.Wait()
		if err != nil {
			t.Fatalf(
				"run %d: %v",
				attempt,
				err,
			)
		}

		if value.String() != FIRST_RUN {
			t.Fatalf(
				"run %d saw a count of %s, want %s",
				attempt,
				value.String(),
				FIRST_RUN,
			)
		}
	}
}

// TestState_MissingKeyIsNone proves reading what nothing has written is the
// ordinary case in a store threads share, not an error.
//
// Revisions:
//   - 2026-09-20 00:39: initial creation
//   - 2026-10-03 00:21: runs a fixture of its own, a run calling main and nothing else
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_MissingKeyIsNone(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"unset.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value != starlark.None {
		t.Fatalf(
			"got %v, want None",
			value,
		)
	}
}

// TestState_ReadsAreCopies proves read-copy-update: a script owns what it read,
// changes it freely, and nothing it does is visible until it publishes.
//
// Until 2026-09-20 00:42 this asserted the opposite - that mutating what was
// read failed loudly, because the frozen value itself was handed out. Freezing
// is still what makes concurrent reads safe; the copy is what makes the store
// usable, since a script that cannot change what it read cannot build the next
// value from it.
//
// Revisions:
//   - 2026-09-20 00:42: initial creation, as TestState_FreezesWhatItStores
//   - 2026-09-20 00:50: reversed, for the copy that replaced the refusal
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_ReadsAreCopies(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"mutate.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != `["one", "two", "three"]` {
		t.Fatalf(
			"got %s",
			value.String(),
		)
	}
}

// TestUpdate_IsAtomic proves what update exists for: four threads incrementing
// one name two hundred times each reach exactly eight hundred.
//
// The same script written with get and set is in racy.star, and reaches a
// different number every run - measured at 294, 575, 317, 644 and 442 before
// update existed. That is why this is not a matter of taste.
//
// Revisions:
//   - 2026-09-20 00:46: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestUpdate_IsAtomic(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"atomic.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"counted %s, want %s",
			value.String(),
			EXPECTED_TOTAL,
		)
	}
}

// TestUpdate_SeesNoneWhenNothingIsStored proves the first update of a name is
// not a special case a script has to guard against with get first.
//
// Revisions:
//   - 2026-09-20 00:47: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestUpdate_SeesNoneWhenNothingIsStored(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"missing.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != "0" {
		t.Fatalf(
			"got %s, want 0",
			value.String(),
		)
	}
}

// TestUpdate_RefusesToNest proves a deadlock is refused rather than reached.
//
// Two cases, and the second is why this is a refusal rather than a warning.
// Nesting a different name is merely unsafe - two threads doing it in opposite
// orders would wait on each other forever. Nesting the *same* name deadlocks
// immediately, on a lock the caller itself holds: with the check removed, that
// script hangs until something kills it, which is what a run that never ends
// looks like from outside.
//
// A script cannot be asked to take locks in an order it cannot see, because the
// locks are Go's and a script never touches one. So nesting is an error, which
// is something a script author can act on.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-09-20 00:52: covers nesting the same name, which is the case that
//     hangs rather than merely risking it
//   - 2026-09-21 08:09: covers nesting through a spawn and through a timeout,
//     which used to hang forever, and matches the sentinel
//   - 2026-09-24 17:12: the spawn case joins after the update rather than
//     inside it, so the refusal under test is the child's own. Joining inside
//     is refused earlier now, and has a test of its own
//   - 2026-09-24 20:20: the spawn case is gone. An update's function returns
//     data, so a handle cannot leave one, and a child spawned inside is no
//     longer joinable - TestUpdate_AChildIsRefusedAfterTheUpdateHasFinished
//     reads what it printed instead
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestUpdate_RefusesToNest(t *testing.T) {
	scripts := []string{
		"nested.star",
		"samekey.star",
		"nested_through_timeout.star",
	}

	for _, script := range scripts {
		t.Run(
			script,
			func(t *testing.T) {
				var (
					started = time.Now()
					err     error
				)

				run, err := runtime.Start(
					t.Context(),
					_Built(
						t,
						script,
					),
				)
				if err != nil {
					t.Fatal(err)
				}

				_, err = run.Wait()
				if !errors.Is(
					err,
					runtime.ErrNested,
				) {
					t.Fatalf(
						"want ErrNested, got %v",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					"cannot start another",
				) {
					t.Fatalf(
						"refused without saying so: %v",
						err,
					)
				}

				if time.Since(started) > PROMPT {
					t.Fatalf(
						"a refusal took %s",
						time.Since(started),
					)
				}
			},
		)
	}
}

// TestUpdate_AContendedNameEndsWithTheRun is review defect E's other half: a
// thread waiting for a name another thread holds must wake when the run
// ends, which a mutex cannot do.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestUpdate_AContendedNameEndsWithTheRun(t *testing.T) {
	var (
		started = time.Now()
		err     error
	)

	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"contended.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrAssert,
	) {
		t.Fatalf(
			"want the assertion, got %v",
			err,
		)
	}

	if time.Since(started) > PROMPT {
		t.Fatalf(
			"a run with a thread blocked on a lock took %s to end",
			time.Since(started),
		)
	}
}

// TestState_CopiesATuple is review defect B: a stored value holding a tuple
// could not be read back, because the copy looked every value up in a Go map
// and a tuple is a slice.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_CopiesATuple(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"tuple.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	want := `[(1, 2), [("a", 1), {"k": ("b", 2)}]]`

	if value.String() != want {
		t.Fatalf(
			"got %s, want %s",
			value.String(),
			want,
		)
	}
}

// TestState_AChangeIsInvisibleUntilPublished proves the middle of
// read-copy-update: the store still holds the old value while a script works on
// its copy.
//
// Revisions:
//   - 2026-09-20 00:51: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_AChangeIsInvisibleUntilPublished(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"unpublished.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != `[["one"], ["one", "two"]]` {
		t.Fatalf(
			"got %s, want the store unchanged beside the changed copy",
			value.String(),
		)
	}
}

// TestState_CopiesSurviveCyclesAndSharing proves two things a deep copy gets
// wrong if it is written the obvious way.
//
// Starlark allows a list that contains itself, and a copy that did not remember
// what it had already made would follow that reference until the stack ran out.
// And a value reachable by two paths must stay one value: copying it twice
// would silently turn one list into two, so a script appending through one path
// would no longer see it through the other.
//
// Revisions:
//   - 2026-09-20 00:52: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_CopiesSurviveCyclesAndSharing(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"cyclic.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if value.String() != `[3, [9, 8]]` {
		t.Fatalf(
			"got %s, want [3, [9, 8]]",
			value.String(),
		)
	}
}

// TestSet_IsNotLostToAnUpdateThatStartedEarlier is the race a store mutex
// cannot close.
//
// Update holds the name's lock across its read, its call and its write. Set
// took only the store's mutex, which update releases around the call - so a
// set landing in that window was overwritten by what the update had read
// before the set happened, and the write that finished second lost silently.
//
// The script orders the set after the update's read with an event, not a
// sleep, so a loaded machine cannot run the set first (B185).
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
//   - 2026-11-08 02:12: fixture waits on an event, not a 10 ms sleep (C106)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSet_IsNotLostToAnUpdateThatStartedEarlier(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"setwins.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if got.String() != `"from-set"` {
		t.Fatalf(
			"got %s, want the set that finished last to be what is stored",
			got.String(),
		)
	}
}

// TestSet_FromInsideAnUpdateIsRefusedRatherThanWaited checks that closing the
// race did not open a deadlock: the lock set now takes is the one the
// surrounding update already holds.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSet_FromInsideAnUpdateIsRefusedRatherThanWaited(t *testing.T) {
	done := make(
		chan error,
		1,
	)

	go func() {
		run, err := runtime.Start(
			t.Context(),
			_Built(
				t,
				"setinside.star",
			),
		)
		if err != nil {
			done <- err

			return
		}

		_, err = run.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(
			err,
			runtime.ErrNested,
		) {
			t.Fatalf(
				"got %v, want ErrNested",
				err,
			)
		}
	case <-time.After(PROMPT):
		t.Fatal("a set inside an update hung rather than being refused")
	}
}

// TestUpdate_AChildIsRefusedAfterTheUpdateHasFinished is the lifetime of the
// mark, written down because it is surprising and deliberate.
//
// A thread spawned inside an update copies the fact that one is held, and
// nothing clears a copy. So it is refused after the parent has released the
// name, and refused for a different name than the one that was held.
//
// The alternative - clearing on release - was considered and rejected: the
// child's set would land either side of a release it cannot see, so the same
// script would succeed or fail on the scheduler's timing. A ban a reader can
// predict beats a failure that flickers.
//
// Revisions:
//   - 2026-09-24 16:55: initial creation
//   - 2026-10-02 13:12: reads what the script said from its transcript
//   - 2026-10-02 16:21: reads it from the run's logger
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: gives the run WithLog, WithLogger being dropped (C121, Q340)
func TestUpdate_AChildIsRefusedAfterTheUpdateHasFinished(t *testing.T) {
	var out bytes.Buffer

	var err error

	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"afterupdate.star",
		),
		runtime.WithLog(&out),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	said := out.String()

	// It got as far as the store, which is what makes the next line a
	// refusal rather than a thread that never ran.
	if !strings.Contains(
		said,
		"child reached the store",
	) {
		t.Fatalf(
			"the child never ran: %q",
			said,
		)
	}

	if strings.Contains(
		said,
		"child stored",
	) {
		t.Fatalf(
			"a child spawned inside an update stored after it returned: %q",
			said,
		)
	}
}

// TestUpdate_ANestedUpdateStillSaysItIsUpdating is the other half: the
// evaluation that took the name is told the true thing about itself.
//
// Revisions:
//   - 2026-09-24 16:55: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestUpdate_ANestedUpdateStillSaysItIsUpdating(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"nested.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrNested,
	) {
		t.Fatalf(
			"got %v, want ErrNested",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"already updating",
	) {
		t.Fatalf(
			"an evaluation holding the name was not told so: %v",
			err,
		)
	}
}

// TestJoin_UnderAHeldNameIsRefused is the wait nothing in a script can end.
//
// An update takes the name before it calls its function, so a join inside that
// function waits for a thread that may need the name the caller is holding.
// The parent waits for the child and the child waits for the parent. Only a
// cancel breaks it, and a script cannot cancel itself - so before this, the
// script ran until the host stopped the run.
//
// Refused whether or not this evaluation is the one holding the name, and
// whether or not the joined thread would in fact have wanted it: which threads
// will touch the store cannot be known before they run, so the refusal is the
// broad one. That is the same trade the lifetime ban makes, and for the same
// reason - a rule a reader can predict beats one that depends on what a thread
// turns out to do.
//
// Revisions:
//   - 2026-09-24 17:12: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJoin_UnderAHeldNameIsRefused(t *testing.T) {
	var (
		started = time.Now()
		err     error
	)

	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"join_under_lock.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrNested,
	) {
		t.Fatalf(
			"got %v, want ErrNested",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"join",
	) {
		t.Fatalf(
			"the refusal does not say what was refused: %v",
			err,
		)
	}

	// It refuses rather than waits, so this returns at once. Before the
	// refusal the same script ran until something killed it.
	if taken := time.Since(started); taken > PROMPT {
		t.Fatalf(
			"refusing took %s, which is a wait rather than a refusal",
			taken,
		)
	}
}

// TestJoin_AfterTheUpdateHasReturnedIsLegal is the other side: the refusal is
// about a name being held, not about the handle.
//
// Revisions:
//   - 2026-09-24 17:12: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJoin_AfterTheUpdateHasReturnedIsLegal(t *testing.T) {
	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"joinafter.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"joining after an update returned was refused: %v",
			err,
		)
	}

	if got.String() != `"joined"` {
		t.Fatalf(
			"got %s, want the joined value",
			got.String(),
		)
	}
}

// TestSet_RefusesWhatIsNotData is the rule that a store holds data.
//
// A function read back by another thread is the same frozen code the script
// already had, and a handle names a thread that means nothing to whoever did
// not start it. Both used to go in and come back out, doing nothing - a
// mistake that reads as if it worked.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_RefusesWhatIsNotData(t *testing.T) {
	cases := map[string]string{
		"a function, at run time": `
def pick():
    return helper

def helper():
    return 1

def main():
    state.set("k", pick())
`,
		"one inside a list": `
def helper():
    return 1

def main():
    state.set("k", [1, helper])
`,
		"a handle": `
def worker():
    return 1

def main():
    state.set("k", spawn(worker))
`,
		"what an update returns": `
def helper():
    return 1

def main():
    state.update("k", lambda v: helper)
`,
	}

	for name, script := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				built, err := runtime.Compile(
					&common.Source{
						Entry: name + ".star",
						Text:  []byte(script),
					},
				)
				if err != nil {
					t.Fatalf(
						"compile: %v",
						err,
					)
				}

				run, err := runtime.Start(
					t.Context(),
					built,
				)
				if err != nil {
					t.Fatal(err)
				}

				_, err = run.Wait()
				if !errors.Is(
					err,
					unpack.ErrConvert,
				) {
					t.Fatalf(
						"got %v, want unpack.ErrConvert",
						err,
					)
				}
			},
		)
	}
}

// TestSet_RefusesAVisibleFunctionBeforeItRuns is the half the source can see.
//
// A name this file declares, or a lambda written in place, is certain before
// anything runs - and an author would rather hear it then. The compiler does
// not know what set means: it asks every plugin whether the source is
// acceptable, and this one answers.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-09-24 21:12: cover the parenthesized spellings
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_RefusesAVisibleFunctionBeforeItRuns(t *testing.T) {
	cases := map[string]string{
		"a declared function": "def helper():\n    return 1\n\ndef main():\n" +
			"    state.set(\"k\", helper)\n",
		"a lambda": "def main():\n    state.set(\"k\", lambda: 1)\n",
		"in parentheses": "def helper():\n    return 1\n\ndef main():\n" +
			"    state.set(\"k\", (helper))\n",
		"and nested ones": "def helper():\n    return 1\n\ndef main():\n" +
			"    state.set(\"k\", ((helper)))\n",
	}

	for name, script := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := runtime.Compile(
					&common.Source{
						Entry: name + ".star",
						Text:  []byte(script),
					},
				)
				if !errors.Is(
					err,
					unpack.ErrConvert,
				) {
					t.Fatalf(
						"compiling"+
							" gave %v, want unpack.ErrConvert "+
							"before it ran",
						err,
					)
				}
			},
		)
	}
}

// TestSet_KeepsTakingData checks the rule refused only what it should.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSet_KeepsTakingData(t *testing.T) {
	const SCRIPT = `
def main():
    state.set("k", [1, {"a": (2, 3)}, None, True, b"x", 1.5])

    return state.get("k")
`

	built, err := runtime.Compile(&common.Source{
		Entry: "data.star",
		Text:  []byte(SCRIPT),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if got.String() != `[1, {"a": (2, 3)}, None, True, b"x", 1.5]` {
		t.Fatalf(
			"got %s",
			got.String(),
		)
	}
}

// TestState_AGetWhileASetRunsIsGuarded covers the one read that takes no lock
// on its name.
//
// A set and an update hold the name's lock, so they cannot overlap each other,
// but a get takes none - a read should never wait on a writer. What keeps a get
// from reading a value half replaced is the entry's own guard. Only the race
// detector can see that guard missing, so this proves it under -race, which is
// how the gate runs, and passes either way without it.
//
// Revisions:
//   - 2026-09-30 21:19: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_AGetWhileASetRunsIsGuarded(t *testing.T) {
	const SCRIPT = `
def writer():
    for i in range(200):
        state.set("k", [i, i + 1])

def reader():
    for i in range(200):
        state.get("k")

def main():
    state.set("k", [0, 1])

    first = spawn(writer)
    second = spawn(reader)
    third = spawn(writer)
    fourth = spawn(reader)

    join(first, second, third, fourth)

    return "done"
`

	built, err := runtime.Compile(&common.Source{
		Entry: "guarded.star",
		Text:  []byte(SCRIPT),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if got.String() != `"done"` {
		t.Fatalf(
			"got %s",
			got.String(),
		)
	}
}

// TestSet_StoringSomethingElseStopsTheWholeRun is why the refusal goes through
// the same door a failed assertion does.
//
// A thread nobody joins fails silently: its error reaches the report and never
// becomes the run's result. So a spawned worker storing a function would put
// nothing in the store, say nothing about it, and let the script finish as
// though it had worked - which is the shape of mistake this rule exists to
// catch.
//
// Revisions:
//   - 2026-09-24 20:26: initial creation
//   - 2026-10-02 13:12: reads what the script said from its transcript
//   - 2026-10-02 16:21: reads it from the run's logger
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue, and gives the run WithLog,
//     WithLogger being dropped (C121, Q340, Q349)
func TestSet_StoringSomethingElseStopsTheWholeRun(t *testing.T) {
	var out bytes.Buffer

	var err error

	run, err := runtime.Start(
		t.Context(),
		_Built(
			t,
			"spawnstore.star",
		),
		runtime.WithLog(&out),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"got %v, want the run stopped with unpack.ErrConvert",
			err,
		)
	}

	for line := range strings.Lines(out.String()) {
		if strings.Contains(
			line,
			"run survived",
		) {
			t.Fatal("the run finished, so an unjoined thread's mistake went unsaid")
		}
	}
}

// TestCheck_AScriptsOwnStateIsNotThePlugin is the false positive a source
// check invites: a global shadows a predeclared name, so a file that binds
// state means its own thing by that name.
//
// Refusing its calls would refuse a script this runtime runs, and blame a
// store it never reached. The same mistake as reading every call named arg as
// a declaration, met twice in one day in two different plugins - which is
// worth knowing about any check written against a plugin's own spelling.
//
// Revisions:
//   - 2026-09-24 20:34: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestCheck_AScriptsOwnStateIsNotThePlugin(t *testing.T) {
	const ALIASED = `
def helper():
    return 1

state = 1

def main():
    return state.set("k", helper)
`

	built, err := runtime.Compile(
		&common.Source{
			Entry: "aliased.star",
			Text:  []byte(ALIASED),
		},
	)
	if err != nil {
		t.Fatalf(
			"compiling a script with its own state was refused: %v",
			err,
		)
	}

	// It fails when it runs, for the reason it should: that value has no set.
	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err == nil {
		t.Fatal("want the run to fail on the aliased value")
	}

	if errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"the store was blamed for a call it never saw: %v",
			err,
		)
	}
}

// TestState_ABareThreadIsToldWhatItLacks pins what each builtin answers on a
// thread no run set up. A builtin finds its evaluation once, so these answers
// are where a change to that lookup shows: a set of data and a get and an
// update say there is no run, and a set of a function still says what is
// wrong with the value, since that is asked before the run is.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_ABareThreadIsToldWhatItLacks(t *testing.T) {
	const NOT_DATA = `state.set "k": builtin_function_or_method: ` +
		`not data a store can hold: value cannot be converted`

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env[spec.NAME].(*starlarkstruct.Module)
	if !ok {
		t.Fatalf(
			"environment holds %v under %s, want a module",
			env[spec.NAME],
			spec.NAME,
		)
	}

	var (
		change = starlark.NewBuiltin(
			"change",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.None, nil
			},
		)

		// The four calls, each with the answer the builtin gave before it found
		// its evaluation once.
		cases = map[string]struct {
			member string
			args   starlark.Tuple
			want   string
		}{
			"a set of data": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.MakeInt(1),
				},
				want: "state.set: no run or nothing to act on",
			},
			"a set of a function": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
				want: NOT_DATA,
			},
			"a get": {
				member: spec.GET,
				args: starlark.Tuple{
					starlark.String("k"),
				},
				want: "state.get: no run or nothing to act on",
			},
			"an update": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
				want: "state.update: no run or nothing to act on",
			},
		}
	)

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := starlark.Call(
					&starlark.Thread{},
					module.Members[tc.member],
					tc.args,
					nil,
				)
				if err == nil || err.Error() != tc.want {
					t.Fatalf(
						"got %v, want %s",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// _Members returns the builtins of the state module, as the default
// environment holds them, or ends the test.
//
// Revisions:
//   - 2026-10-29 01:31: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Members(t *testing.T) starlark.StringDict {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env[spec.NAME].(*starlarkstruct.Module)
	if !ok {
		t.Fatalf(
			"environment holds %v under %s, want a module",
			env[spec.NAME],
			spec.NAME,
		)
	}

	return module.Members
}

// TestState_AWrongCallIsToldWhatItGot pins the text of each refusal of the
// arguments to set, get and update. The texts are what
// starlark.UnpackPositionalArgs wrote when the builtins called it, recorded
// before they read their arguments without it. Each now names its builtin
// once, not twice.
//
// The thread is bare. Each refusal comes before the builtin looks for its run,
// so no run is needed, and a call that got past its arguments would say "no
// run on this thread" and fail the row.
//
// Revisions:
//   - 2026-10-29 01:31: initial creation
//   - 2026-11-02 08:41: each refusal names its builtin once
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestState_AWrongCallIsToldWhatItGot(t *testing.T) {
	// Each refusal names its builtin once, from the unpacking. The builtin
	// returns that error as it is.
	const (
		FROM_SET    = "state.set: "
		FROM_GET    = "state.get: "
		FROM_UPDATE = "state.update: "
	)

	var (
		members = _Members(t)

		change = starlark.NewBuiltin(
			"change",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.None, nil
			},
		)

		keyword = []starlark.Tuple{
			{
				starlark.String("name"),
				starlark.String("k"),
			},
		}

		cases = map[string]struct {
			member string
			args   starlark.Tuple
			kwargs []starlark.Tuple
			want   string
		}{
			"a set of nothing": {
				member: spec.SET,
				want:   FROM_SET + "got 0 arguments, want 2",
			},
			"a set of a name alone": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.String("k"),
				},
				want: FROM_SET + "got 1 arguments, want 2",
			},
			"a set of three": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.MakeInt(1),
					starlark.MakeInt(2),
				},
				want: FROM_SET + "got 3 arguments, want 2",
			},
			"a set with a keyword": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.MakeInt(1),
				},
				kwargs: keyword,
				want:   FROM_SET + "unexpected keyword arguments",
			},
			"a set with a keyword and nothing else": {
				member: spec.SET,
				kwargs: keyword,
				want:   FROM_SET + "unexpected keyword arguments",
			},
			"a set of a name that is an int": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.MakeInt(1),
				},
				want: FROM_SET + "for parameter 1: got int, want string",
			},
			"a set of a name that is bytes": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.Bytes("k"),
					starlark.MakeInt(1),
				},
				want: FROM_SET + "for parameter 1: got bytes, want string",
			},
			"a set of a name that is None": {
				member: spec.SET,
				args: starlark.Tuple{
					starlark.None,
					starlark.MakeInt(1),
				},
				want: FROM_SET + "for parameter 1: got NoneType, want string",
			},
			"a get of nothing": {
				member: spec.GET,
				want:   FROM_GET + "got 0 arguments, want 1",
			},
			"a get of two": {
				member: spec.GET,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.String("j"),
				},
				want: FROM_GET + "got 2 arguments, want 1",
			},
			"a get with a keyword": {
				member: spec.GET,
				kwargs: keyword,
				want:   FROM_GET + "unexpected keyword arguments",
			},
			"a get of a name that is an int": {
				member: spec.GET,
				args: starlark.Tuple{
					starlark.MakeInt(1),
				},
				want: FROM_GET + "for parameter 1: got int, want string",
			},
			"a get of a name that is a list": {
				member: spec.GET,
				args: starlark.Tuple{
					starlark.NewList(nil),
				},
				want: FROM_GET + "for parameter 1: got list, want string",
			},
			"an update of nothing": {
				member: spec.UPDATE,
				want:   FROM_UPDATE + "got 0 arguments, want 2",
			},
			"an update of a name alone": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
				},
				want: FROM_UPDATE + "got 1 arguments, want 2",
			},
			"an update of three": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
					change,
					change,
				},
				want: FROM_UPDATE + "got 3 arguments, want 2",
			},
			"an update with a keyword": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
				kwargs: keyword,
				want:   FROM_UPDATE + "unexpected keyword arguments",
			},
			"an update of a name that is an int": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.MakeInt(1),
					change,
				},
				want: FROM_UPDATE + "for parameter 1: got int, want string",
			},
			"an update of a name that is wrong and a change that is wrong": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.MakeInt(2),
				},
				want: FROM_UPDATE + "for parameter 1: got int, want string",
			},
			"an update of a change that is an int": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.MakeInt(1),
				},
				want: FROM_UPDATE + "for parameter 2: got int, want callable",
			},
			"an update of a change that is a string": {
				member: spec.UPDATE,
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.String("f"),
				},
				want: FROM_UPDATE + "for parameter 2: got string, want callable",
			},
		}
	)

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := starlark.Call(
					&starlark.Thread{},
					members[tc.member],
					tc.args,
					tc.kwargs,
				)
				if err == nil || err.Error() != tc.want {
					t.Fatalf(
						"got %v, want %s",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// _Script returns text without the newline that opens its raw string.
//
// Revisions:
//   - 2026-11-09 11:36: initial creation (C114, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Script(text string) string {
	return strings.TrimPrefix(
		text,
		"\n",
	)
}
