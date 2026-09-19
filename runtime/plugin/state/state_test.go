package state_test

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
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
func TestUpdate_RefusesToNest(t *testing.T) {
	for _, script := range []string{NESTED_SCRIPT, SAMEKEY_SCRIPT} {
		t.Run(script, func(t *testing.T) {
			_, err := _Built(t, script).Run(t.Context())
			if err == nil {
				t.Fatal("an update started another")
			}

			if !strings.Contains(err.Error(), "cannot start another") {
				t.Fatalf("refused for some other reason: %v", err)
			}

			t.Logf("refused: %v", err)
		})
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
