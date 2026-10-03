// Regression probes for the memory a run may use: that a host's choice
// reaches the run, and that leaving it out leaves the default standing.
package testing_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// _BLOB is how big the fixture is. Comfortably under the default ceiling
	// and comfortably over the narrow one.
	_BLOB = 1 << 20

	// _NARROW is the ceiling that cannot hold the fixture, in bytes.
	_NARROW = 64 << 10
)

// _Reading is a script that reads the named file and answers with its length.
//
// Revisions:
//   - 2026-09-24 23:40: initial creation
func _Reading(t *testing.T) []byte {
	t.Helper()

	named := filepath.Join(t.TempDir(), "blob.bin")

	err := os.WriteFile(named, []byte(strings.Repeat("y", _BLOB)), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return fmt.Appendf(nil, "def main():\n    return len(file.read(%q))\n", named)
}

// TestCeiling_AHostChoosesWhatARunMayHold is the whole of the feature from
// outside: a host sets a ceiling and the run it starts is held to it.
//
// The ceiling rides on the context, so this also proves the value survives
// every hop between Run and the builtin that charges it - which is the part
// nothing else would notice breaking.
//
// Revisions:
//   - 2026-09-24 23:40: initial creation
//   - 2026-10-02 17:12: hands the compiler file, which the runtime no longer
//     gives a script by itself
func TestCeiling_AHostChoosesWhatARunMayHold(t *testing.T) {
	src := _Reading(t)

	built, err := runtime.Compile(
		&runtime.Source{Entry: "ceiling.star", Text: src},
		runtime.WithPlugins(&larkfile.Plugin{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(context.Background(), built, runtime.WithMemory(_NARROW)).Wait()
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf("a host's ceiling did not reach the run: %v", err)
	}

	t.Logf("refused with: %v", err)

	// And the same artifact under the default ceiling still reads the file, so
	// the refusal was the ceiling rather than the file.
	got, err := runtime.Start(context.Background(), built).Wait()
	if err != nil {
		t.Fatalf("under the default ceiling: %v", err)
	}

	if got.String() != fmt.Sprint(_BLOB) {
		t.Fatalf("read %s bytes, want %d", got, _BLOB)
	}
}

// TestCeiling_IsGivenBackBetweenReads is what keeps a ceiling from being a
// budget for the whole run's history.
//
// A script reading the same file a hundred times holds one copy at a time. If
// what a read reserved were never given back, the hundredth would fail under a
// ceiling the first passed - and a loop over a directory is the ordinary shape
// of this library's work.
//
// Revisions:
//   - 2026-09-24 23:40: initial creation
//   - 2026-10-02 17:12: hands the compiler file, which the runtime no longer
//     gives a script by itself
func TestCeiling_IsGivenBackBetweenReads(t *testing.T) {
	named := filepath.Join(t.TempDir(), "blob.bin")

	err := os.WriteFile(named, []byte(strings.Repeat("y", _BLOB)), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	src := fmt.Appendf(nil, "def main():\n"+
		"    total = 0\n"+
		"    for i in range(100):\n"+
		"        total += len(file.read(%q))\n"+
		"    return total\n", named)

	built, err := runtime.Compile(
		&runtime.Source{Entry: "repeat.star", Text: src},
		runtime.WithPlugins(&larkfile.Plugin{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Room for a few copies at once, nowhere near a hundred.
	got, err := runtime.Start(context.Background(), built, runtime.WithMemory(8*_BLOB)).Wait()
	if err != nil {
		t.Fatalf("a hundred reads of one file needed more than eight of it: %v", err)
	}

	if got.String() != fmt.Sprint(100*_BLOB) {
		t.Fatalf("read %s bytes in total, want %d", got, 100*_BLOB)
	}
}

// TestCeiling_ASpawnedThreadIsCharged is the largest thing this library
// allocates for a script, and it went uncharged until it was measured.
//
// A thread is a goroutine with its stack, an interpreter thread, its locals and
// a handle. Measured 2026-09-27 at about 14KB each: twenty thousand of them took
// 287MB under a ceiling of one megabyte, and succeeded.
//
// Revisions:
//   - 2026-09-27 01:40: initial creation
func TestCeiling_ASpawnedThreadIsCharged(t *testing.T) {
	src := []byte(`
def quiet():
    sleep(5000)

    return 1

def main():
    held = []

    for i in range(200):
        held.append(spawn(quiet))

    return len(held)
`)

	built, err := runtime.Compile(&runtime.Source{Entry: "threads.star", Text: src})
	if err != nil {
		t.Fatal(err)
	}

	// Room for a handful of threads, nowhere near two hundred.
	_, err = runtime.Start(
		t.Context(),
		built,
		runtime.WithMemory(10*scheduler.THREAD_COST),
	).Wait()
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf("two hundred threads under ten threads' worth: %v, want ERR_MEMORY", err)
	}
}

// TestCeiling_AThreadIsCreditedWhenItEnds is what keeps the charge from being a
// limit on how many threads a run may ever start.
//
// Two hundred threads one at a time hold one thread's worth at a time. If the
// charge were never given back, the eleventh would fail under a ceiling the
// first passed - and spawning in a loop is the ordinary shape of this library's
// work.
//
// Revisions:
//   - 2026-09-27 01:40: initial creation
func TestCeiling_AThreadIsCreditedWhenItEnds(t *testing.T) {
	src := []byte(`
def quick():
    return 1

def main():
    total = 0

    for i in range(200):
        total += join(spawn(quick))[0]

    return total
`)

	built, err := runtime.Compile(&runtime.Source{Entry: "churn.star", Text: src})
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.Start(
		t.Context(),
		built,
		runtime.WithMemory(10*scheduler.THREAD_COST),
	).Wait()
	if err != nil {
		t.Fatalf("two hundred threads one at a time: %v", err)
	}

	if got.String() != "200" {
		t.Fatalf("got %s, want 200", got)
	}
}

// TestCeiling_TheStoreIsChargedForWhatItHolds covers the one call whose purpose
// is to keep something for the life of the run.
//
// There is no delete, so a name once set is held until the run ends. A script
// could fill memory through it and nothing said so until 2026-09-27.
//
// One name each time, under one ceiling, with only the value's size differing.
// A name is charged as well, so a script storing many would be refused for the
// names whether or not the values were charged.
//
// Revisions:
//   - 2026-09-27 01:40: initial creation
//   - 2026-09-30 21:19: one name rather than twenty thousand, so the value is
//     what is refused
func TestCeiling_TheStoreIsChargedForWhatItHolds(t *testing.T) {
	small := []byte(`
def main():
    state.set("small", "x" * 1024)

    return "stored"
`)

	built, err := runtime.Compile(&runtime.Source{Entry: "small.star", Text: small})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW)).Wait()
	if err != nil {
		t.Fatalf("one value of 1KB under %d bytes: %v", _NARROW, err)
	}

	large := []byte(`
def main():
    state.set("large", "x" * (128 * 1024))

    return "stored"
`)

	built, err = runtime.Compile(&runtime.Source{Entry: "large.star", Text: large})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW)).Wait()
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf("one value of 128KB under %d bytes: %v, want ERR_MEMORY", _NARROW, err)
	}

	// One name replaced many times costs what one of them costs, because the
	// store charges the difference and remembers what it charged.
	reset := []byte(`
def main():
    for i in range(20000):
        state.set("k", "a value long enough to be worth counting, number %d" % i)

    return state.get("k")
`)

	built, err = runtime.Compile(&runtime.Source{Entry: "reset.star", Text: reset})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW)).Wait()
	if err != nil {
		t.Fatalf("one name set twenty thousand times: %v", err)
	}
}

// TestCeiling_TheStoreIsChargedForEachName covers what a name costs apart from
// its value, which an event is charged for too.
//
// A thousand names holding None: the values come to about 16KB and fit, and the
// names come to about 320KB and do not. So this fails only if the names are
// charged. Reading a thousand names nothing has written, under the same
// ceiling, is the control - a read makes no entry, so it costs nothing.
//
// Revisions:
//   - 2026-09-30 21:19: initial creation
func TestCeiling_TheStoreIsChargedForEachName(t *testing.T) {
	named := []byte(`
def main():
    for i in range(1000):
        state.set("k%d" % i, None)

    return "stored"
`)

	built, err := runtime.Compile(&runtime.Source{Entry: "named.star", Text: named})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW)).Wait()
	if !errors.Is(err, scheduler.ERR_MEMORY) {
		t.Fatalf("a thousand names under %d bytes: %v, want ERR_MEMORY", _NARROW, err)
	}

	read := []byte(`
def main():
    for i in range(1000):
        state.get("k%d" % i)

    return "read"
`)

	built, err = runtime.Compile(&runtime.Source{Entry: "read.star", Text: read})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW)).Wait()
	if err != nil {
		t.Fatalf("reading a thousand unwritten names under %d bytes: %v", _NARROW, err)
	}
}

// WALKING is a script whose spawned reader loops over the lines of the file it
// is given, beside a sibling that sleeps far longer than the test.
const WALKING = `
def reader():
    for line in file.lines(%q):
        pass

def slow():
    sleep(30000)

def main():
    r = spawn(reader)
    s = spawn(slow)
    join(s)
`

// TestCeiling_AWalkCutShortFailsItsOwnThread checks a file.lines loop the
// ceiling cuts short, in a spawned thread, fails that thread and is named as
// the run's cause, while the sibling still going is cancelled.
//
// The loop ends quietly, since Starlark's iterator has no error to raise, and
// the failure is recorded once it is done. Unless the thread's own result
// carries it, the run ends for a reason nothing in its graph owns: the cause
// named main, and the thread that read the file read as succeeded.
//
// Revisions:
//   - 2026-10-03 23:52: initial creation
func TestCeiling_AWalkCutShortFailsItsOwnThread(t *testing.T) {
	named := filepath.Join(t.TempDir(), "line.txt")

	err := os.WriteFile(named, []byte(strings.Repeat("y", _BLOB)), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	built, err := runtime.Compile(
		&runtime.Source{Entry: "walk.star", Text: fmt.Appendf(nil, WALKING, named)},
		runtime.WithPlugins(&larkfile.Plugin{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	run := runtime.Start(t.Context(), built, runtime.WithMemory(_NARROW))

	_, err = run.Wait()
	if !errors.Is(err, runtime.ERR_MEMORY) {
		t.Fatalf("got %v, want the ceiling's refusal", err)
	}

	graph := run.Status()

	if got := graph.GetCause().GetFunction(); got != "reader" {
		t.Fatalf("the run's cause is %q, want the reader whose loop was cut short", got)
	}

	want := map[string]string{"reader": "STATUS_FAILED", "slow": "STATUS_CANCELLED"}

	for _, node := range graph.GetFunctions() {
		status, listed := want[node.GetName()]
		if listed && node.GetStatus().String() != status {
			t.Errorf("%s reads %s, want %s", node.GetName(), node.GetStatus(), status)
		}
	}
}
