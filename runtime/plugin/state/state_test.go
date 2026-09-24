package state_test

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/state"
)

const (
	FIXTURE_DIR        = "testdata"
	SHARE_SCRIPT       = "share.star"
	COUNT_SCRIPT       = "count.star"
	EXPECTED           = "written by one thread, read by another"
	MUTATE_SCRIPT      = "mutate.star"
	ATOMIC_SCRIPT      = "atomic.star"
	NESTED_SCRIPT      = "nested.star"
	SAMEKEY_SCRIPT     = "samekey.star"
	UNPUBLISHED_SCRIPT = "unpublished.star"
	CYCLIC_SCRIPT      = "cyclic.star"
	MISSING_SCRIPT     = "missing.star"
	EXPECTED_TOTAL     = "800"
	FIRST_RUN          = "1"

	// PROMPT is how long a run that should end at once may take on a loaded
	// machine before it is called hung.
	PROMPT = 1500 * time.Millisecond
)

// _Disk is a Loader over testdata.
type _Disk struct{}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-20 00:36: initial creation
func (d *_Disk) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the fixture at name.
//
// Revisions:
//   - 2026-09-20 00:36: initial creation
func (d *_Disk) Load(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(FIXTURE_DIR, name))
}

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-20 00:36: initial creation
func _Built(t *testing.T, name string) *runtime.Artifact {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	built, err := runtime.NewCompiler(runtime.WithLoader(&_Disk{})).Compile(name, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built
}

// TestState_IsSharedBetweenThreadsOfOneRun proves what the store is for: module
// scope is frozen, so two spawned threads have no other way to pass anything.
//
// Revisions:
//   - 2026-09-20 00:37: initial creation
func TestState_IsSharedBetweenThreadsOfOneRun(t *testing.T) {
	value, err := _Built(t, SHARE_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	text, ok := value.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", value)
	}

	if string(text) != EXPECTED {
		t.Fatalf("got %q, want %q", string(text), EXPECTED)
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
func TestState_IsPerExecutionNotPerCompile(t *testing.T) {
	built := _Built(t, COUNT_SCRIPT)

	for attempt := range 2 {
		value, err := built.Run(t.Context())
		if err != nil {
			t.Fatalf("run %d: %v", attempt, err)
		}

		if value.String() != FIRST_RUN {
			t.Fatalf("run %d saw a count of %s, want %s", attempt, value.String(), FIRST_RUN)
		}
	}
}

// TestState_MissingKeyIsNone proves reading what nothing has written is the
// ordinary case in a store threads share, not an error.
//
// Revisions:
//   - 2026-09-20 00:39: initial creation
func TestState_MissingKeyIsNone(t *testing.T) {
	built := _Built(t, COUNT_SCRIPT)

	value, err := built.Invoke(t.Context(), "unset")
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if value != starlark.None {
		t.Fatalf("got %v, want None", value)
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
func TestState_ReadsAreCopies(t *testing.T) {
	value, err := _Built(t, MUTATE_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != `["one", "two", "three"]` {
		t.Fatalf("got %s", value.String())
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
func TestUpdate_IsAtomic(t *testing.T) {
	value, err := _Built(t, ATOMIC_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != EXPECTED_TOTAL {
		t.Fatalf("counted %s, want %s", value.String(), EXPECTED_TOTAL)
	}
}

// TestUpdate_SeesNoneWhenNothingIsStored proves the first update of a name is
// not a special case a script has to guard against with get first.
//
// Revisions:
//   - 2026-09-20 00:47: initial creation
func TestUpdate_SeesNoneWhenNothingIsStored(t *testing.T) {
	value, err := _Built(t, MISSING_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != "0" {
		t.Fatalf("got %s, want 0", value.String())
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
func TestUpdate_RefusesToNest(t *testing.T) {
	scripts := []string{
		NESTED_SCRIPT,
		SAMEKEY_SCRIPT,
		"nested_through_spawn.star",
		"nested_through_timeout.star",
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			started := time.Now()

			_, err := _Built(t, script).Run(t.Context())
			if !errors.Is(err, runtime.ErrNested) {
				t.Fatalf("want ErrNested, got %v", err)
			}

			if !strings.Contains(err.Error(), "cannot start another") {
				t.Fatalf("refused without saying so: %v", err)
			}

			if time.Since(started) > PROMPT {
				t.Fatalf("a refusal took %s", time.Since(started))
			}
		})
	}
}

// TestUpdate_AContendedNameEndsWithTheRun is review defect E's other half: a
// thread waiting for a name another thread holds must wake when the run
// ends, which a mutex cannot do.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestUpdate_AContendedNameEndsWithTheRun(t *testing.T) {
	started := time.Now()

	_, err := _Built(t, "contended.star").Run(t.Context())
	if !errors.Is(err, runtime.ErrAssert) {
		t.Fatalf("want the assertion, got %v", err)
	}

	if time.Since(started) > PROMPT {
		t.Fatalf("a run with a thread blocked on a lock took %s to end", time.Since(started))
	}
}

// TestState_CopiesATuple is review defect B: a stored value holding a tuple
// could not be read back, because the copy looked every value up in a Go map
// and a tuple is a slice.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestState_CopiesATuple(t *testing.T) {
	value, err := _Built(t, "tuple.star").Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := `[(1, 2), [("a", 1), {"k": ("b", 2)}]]`

	if value.String() != want {
		t.Fatalf("got %s, want %s", value.String(), want)
	}
}

// TestState_AChangeIsInvisibleUntilPublished proves the middle of
// read-copy-update: the store still holds the old value while a script works on
// its copy.
//
// Revisions:
//   - 2026-09-20 00:51: initial creation
func TestState_AChangeIsInvisibleUntilPublished(t *testing.T) {
	value, err := _Built(t, UNPUBLISHED_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != `[["one"], ["one", "two"]]` {
		t.Fatalf("got %s, want the store unchanged beside the changed copy", value.String())
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
func TestState_CopiesSurviveCyclesAndSharing(t *testing.T) {
	value, err := _Built(t, CYCLIC_SCRIPT).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != `[3, [9, 8]]` {
		t.Fatalf("got %s, want [3, [9, 8]]", value.String())
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
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestSet_IsNotLostToAnUpdateThatStartedEarlier(t *testing.T) {
	got, err := _Built(t, "setwins.star").Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if got.String() != `"from-set"` {
		t.Fatalf("got %s, want the set that finished last to be what is stored", got.String())
	}
}

// TestSet_FromInsideAnUpdateIsRefusedRatherThanWaited checks that closing the
// race did not open a deadlock: the lock set now takes is the one the
// surrounding update already holds.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestSet_FromInsideAnUpdateIsRefusedRatherThanWaited(t *testing.T) {
	done := make(chan error, 1)

	go func() {
		_, err := _Built(t, "setinside.star").Run(t.Context())
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, runtime.ErrNested) {
			t.Fatalf("got %v, want ErrNested", err)
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
func TestUpdate_AChildIsRefusedAfterTheUpdateHasFinished(t *testing.T) {
	_, err := _Built(t, "afterupdate.star").Run(t.Context())
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}

	// And it says which of the two refusals this is. The parent finished
	// before the child ever called set, so "already updating" would be false.
	if strings.Contains(err.Error(), "already updating") {
		t.Fatalf("a child that inherited the mark was told it is updating: %v", err)
	}

	if !strings.Contains(err.Error(), `started inside an update of "a"`) {
		t.Fatalf("the refusal does not say why it refused: %v", err)
	}
}

// TestUpdate_ANestedUpdateStillSaysItIsUpdating is the other half: the
// evaluation that took the name is told the true thing about itself.
//
// Revisions:
//   - 2026-09-24 16:55: initial creation
func TestUpdate_ANestedUpdateStillSaysItIsUpdating(t *testing.T) {
	_, err := _Built(t, NESTED_SCRIPT).Run(t.Context())
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}

	if !strings.Contains(err.Error(), "already updating") {
		t.Fatalf("an evaluation holding the name was not told so: %v", err)
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
func TestJoin_UnderAHeldNameIsRefused(t *testing.T) {
	started := time.Now()

	_, err := _Built(t, "join_under_lock.star").Run(t.Context())
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}

	if !strings.Contains(err.Error(), "join") {
		t.Fatalf("the refusal does not say what was refused: %v", err)
	}

	// It refuses rather than waits, so this returns at once. Before the
	// refusal the same script ran until something killed it.
	if taken := time.Since(started); taken > PROMPT {
		t.Fatalf("refusing took %s, which is a wait rather than a refusal", taken)
	}
}

// TestJoin_AfterTheUpdateHasReturnedIsLegal is the other side: the refusal is
// about a name being held, not about the handle.
//
// Revisions:
//   - 2026-09-24 17:12: initial creation
func TestJoin_AfterTheUpdateHasReturnedIsLegal(t *testing.T) {
	// afterupdate.star spawns inside an update and joins the handle after the
	// update returned. Its child is refused at set, not at join - so if the
	// join were refused this would say so instead.
	_, err := _Built(t, "afterupdate.star").Run(t.Context())
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested from the child", err)
	}

	if strings.Contains(err.Error(), "join") {
		t.Fatalf("the join after the update was refused, and should not be: %v", err)
	}
}
