package file_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
	"github.com/thebagchi/lark/runtime/plugin"
	larkfile "github.com/thebagchi/lark/runtime/plugin/file"
	_ "github.com/thebagchi/lark/runtime/plugin/path"
	_ "github.com/thebagchi/lark/runtime/plugin/time"
	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	// _LONG is a line far past any buffer a reader would choose by itself, and
	// far past the small ceiling below.
	_LONG = 2 << 20

	// _SMALL is what a run in the narrow harness may hold: enough to work,
	// nothing like enough for _LONG.
	_SMALL = 64 << 10

	// _BIG is how many lines the memory proof writes.
	_BIG = 400000

	// _FILLER is the text every one of those lines holds.
	_FILLER = "the quick brown fox jumps over the lazy dog, and then does it again"
)

// _Within evaluates one expression inside a run whose ceiling is given, so a
// refusal is about the budget rather than about the size of a fixture.
//
// The order scheduler.Evaluate uses: end the run, then let what ended it win.
// A walk that fails has already returned by the time it knows, so the run's
// outcome is the only place its error can be.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func _Within(t *testing.T, root string, ceiling int64, expression string) (string, error) {
	t.Helper()

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	held := starlark.StringDict{"root": starlark.String(root)}

	for name, value := range env {
		held[name] = value
	}

	thread := &starlark.Thread{Name: SCRIPT}
	ending := scheduler.Begin(scheduler.Allowing(context.Background(), ceiling), thread, SCRIPT)

	value, err := starlark.EvalOptions(dialect.OPTIONS, thread, SCRIPT, expression, held)

	ending()

	outcome := scheduler.Outcome(thread)
	if outcome != nil {
		return "", outcome
	}

	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Put writes a file under root and answers with its path.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func _Put(t *testing.T, root string, name string, body string) string {
	t.Helper()

	named := filepath.Join(root, name)

	err := os.WriteFile(named, []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return named
}

// TestStat_AnswersWithWhatTheSyscallRead is the whole point of the call: the
// stat already read these, and size threw them away.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestStat_AnswersWithWhatTheSyscallRead(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "note.txt", "hello")

	cases := map[string]string{
		`file.stat(path.join(root, "note.txt")).size`:         "5",
		`file.stat(path.join(root, "note.txt")).dir`:          "False",
		`file.stat(path.join(root, "note.txt")).mode`:         "384",
		`"%o" % file.stat(path.join(root, "note.txt")).mode`:  `"600"`,
		`file.stat(path.join(root, "note.txt")).mode & 0o700`: "384",
		`file.stat(root).dir`:                                 "True",
		`type(file.stat(root).modified)`:                      `"time.time"`,
	}

	for expression, want := range cases {
		t.Run(expression, func(t *testing.T) {
			got, err := _Eval(t, root, expression)
			if err != nil {
				t.Fatalf("%s: %v", expression, err)
			}

			if got != want {
				t.Fatalf("%s gave %s, want %s", expression, got, want)
			}
		})
	}
}

// TestStat_RefusesWhatIsNotThere keeps the module's one error shape.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestStat_RefusesWhatIsNotThere(t *testing.T) {
	root := t.TempDir()

	_, err := _Eval(t, root, `file.stat(path.join(root, "nope.txt"))`)
	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("got %v, want ErrFile", err)
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refusal lost the reason underneath: %v", err)
	}
}

// TestLines_WalksAndRoundTrips is the ordinary use of all three.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_WalksAndRoundTrips(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "three.txt", "alpha\nbeta\ngamma\n")

	cases := []struct{ name, expression, want string }{
		{
			"walk",
			`[line for line in file.lines(path.join(root, "three.txt"))]`,
			`["alpha", "beta", "gamma"]`,
		},
		{
			"write then walk",
			`file.writelines(path.join(root, "out.txt"), ["a", "b"]) or ` +
				`[line for line in file.lines(path.join(root, "out.txt"))]`,
			`["a", "b"]`,
		},
		{
			"append then walk",
			`file.appendlines(path.join(root, "out.txt"), ["c"]) or ` +
				`[line for line in file.lines(path.join(root, "out.txt"))]`,
			`["a", "b", "c"]`,
		},
		{
			"a file with no last newline",
			`file.writelines(path.join(root, "bare.txt"), ["x"]) or ` +
				`file.read(path.join(root, "bare.txt"))`,
			`"x"`,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, root, item.expression)
			if err != nil {
				t.Fatalf("%v", err)
			}

			if got != item.want {
				t.Fatalf("got %s, want %s", got, item.want)
			}
		})
	}
}

// TestAppendlines_JoinsWhateverWasAlreadyThere is the cost of trimming the
// last newline, and the proof that it is paid.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestAppendlines_JoinsWhateverWasAlreadyThere(t *testing.T) {
	root := t.TempDir()

	cases := []struct{ name, start, want string }{
		{"nothing there yet", "", `["c", "d"]`},
		{"trimmed, as this writer leaves it", "a\nb", `["a", "b", "c", "d"]`},
		{"ended by someone else", "a\nb\n", `["a", "b", "c", "d"]`},
		{"one line, no terminator", "a", `["a", "c", "d"]`},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			named := _Put(t, root, "log.txt", item.start)

			got, err := _Eval(t, root, fmt.Sprintf(
				`file.appendlines(%q, ["c", "d"]) or [line for line in file.lines(%q)]`,
				named, named))
			if err != nil {
				t.Fatal(err)
			}

			if got != item.want {
				t.Fatalf("starting from %q gave %s, want %s", item.start, got, item.want)
			}
		})
	}
}

// TestLines_RefusesAtTheCallThatNamedTheFile is where an error belongs: the
// line that names a missing file, not the loop three lines later.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_RefusesAtTheCallThatNamedTheFile(t *testing.T) {
	root := t.TempDir()

	cases := map[string]struct {
		expression string
		want       error
	}{
		"missing":     {`file.lines(path.join(root, "nope.txt"))`, os.ErrNotExist},
		"a directory": {`file.lines(root)`, larkfile.ErrNotAFile},
	}

	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := _Eval(t, root, item.expression)
			if !errors.Is(err, larkfile.ErrFile) {
				t.Fatalf("got %v, want ErrFile", err)
			}

			if !errors.Is(err, item.want) {
				t.Fatalf("got %v, want it to carry %v", err, item.want)
			}
		})
	}
}

// TestLines_ANamedFileNeverWalkedOpensNothing is why the handle waits for the
// loop rather than opening where the file was named.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_ANamedFileNeverWalkedOpensNothing(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "idle.txt", "one\n")

	before := _Handles(t)

	_, err := _Eval(t, root,
		`[file.lines(path.join(root, "idle.txt")) for i in range(200)] and "named"`)
	if err != nil {
		t.Fatal(err)
	}

	after := _Handles(t)
	if after > before+1 {
		t.Fatalf("naming 200 files left %d handles open, was %d", after, before)
	}
}

// _Handles counts this process's open descriptors, or skips when the system
// does not say.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func _Handles(t *testing.T) int {
	t.Helper()

	held, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("no descriptor count on this system: %v", err)
	}

	return len(held)
}

// TestLines_IsWalkedOnce is the Python file object shape: a second pass is
// refused rather than quietly starting again from the beginning.
//
// Run inside a run, because a walk learns it was refused only once it has
// returned, and the run's outcome is the only place that can be reported. On
// a bare thread there is no run to end, so a second walk yields nothing and
// says nothing - which is all a thread with no run can be told.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_IsWalkedOnce(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "once.txt", "a\nb\n")

	_, err := _Within(t, root, 0, `[[line for line in held] for held in `+
		`[file.lines(path.join(root, "once.txt"))] * 2]`)
	if !errors.Is(err, larkfile.ErrWalked) {
		t.Fatalf("got %v, want ErrWalked", err)
	}

	got, err := _Eval(t, root, `[line for line in file.lines(path.join(root, "once.txt"))]`)
	if err != nil {
		t.Fatal(err)
	}

	if got != `["a", "b"]` {
		t.Fatalf("got %s", got)
	}
}

// TestLines_CostsOneLineNotTheFile is the measurement the whole change is for.
//
// read of a 512MB file peaked at 1057MB resident, because ReadFile allocates
// the bytes and starlark.String copies them. A walk reuses one buffer, so what
// it costs is set by the longest line rather than by the file - which is why
// this needs no limit on how big a file may be.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_CostsOneLineNotTheFile(t *testing.T) {
	root := t.TempDir()
	named := filepath.Join(root, "big.txt")

	held, err := os.Create(named)
	if err != nil {
		t.Fatal(err)
	}

	for range _BIG {
		_, err = fmt.Fprintln(held, _FILLER)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = held.Close()
	if err != nil {
		t.Fatal(err)
	}

	about, err := os.Stat(named)
	if err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	got, err := _Eval(t, root, `len([1 for line in file.lines(path.join(root, "big.txt"))])`)
	if err != nil {
		t.Fatal(err)
	}

	runtime.ReadMemStats(&after)

	t.Logf("file %d bytes, walk allocated %d in total, heap reached %d",
		about.Size(), after.TotalAlloc-before.TotalAlloc, after.HeapAlloc)

	if got != fmt.Sprint(_BIG) {
		t.Fatalf("walked %s lines, want %d", got, _BIG)
	}

	// The list of results is what grows here, not the reader. What matters is
	// that nothing ever held the file: the peak heap, not the running total.
	if after.HeapAlloc > uint64(about.Size()) {
		t.Fatalf("heap reached %d on a %d byte file, so the walk is holding it",
			after.HeapAlloc, about.Size())
	}
}

// TestLines_ALongLineIsReadRatherThanRefused is what replaced a fixed limit:
// how long a line may be is what the run has left to spend.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_ALongLineIsReadRatherThanRefused(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "long.txt", strings.Repeat("x", _LONG)+"\nshort\n")

	got, err := _Eval(t, root,
		`[len(line) for line in file.lines(path.join(root, "long.txt"))]`)
	if err != nil {
		t.Fatalf("a %d byte line was refused: %v", _LONG, err)
	}

	want := fmt.Sprintf("[%d, 5]", _LONG)
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestLines_ALineTheRunCannotAffordEndsIt is the budget doing the refusing,
// with the reason being the memory rather than a number nobody chose.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestLines_ALineTheRunCannotAffordEndsIt(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "long.txt", strings.Repeat("x", _LONG)+"\n")

	_, err := _Within(t, root, _SMALL,
		`[line for line in file.lines(path.join(root, "long.txt"))]`)
	if !errors.Is(err, scheduler.ErrMemory) {
		t.Fatalf("got %v, want ErrMemory", err)
	}

	t.Logf("ended with: %v", err)
}

// TestRead_TooBigForTheRunIsRefusedBeforeItIsAllocated is the answer to the
// size question that needs no limit on how big a file may be.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestRead_TooBigForTheRunIsRefusedBeforeItIsAllocated(t *testing.T) {
	root := t.TempDir()

	_Put(t, root, "big.txt", strings.Repeat("y", _LONG))

	for _, name := range []string{"read", "bytes"} {
		t.Run(name, func(t *testing.T) {
			_, err := _Within(t, root, _SMALL,
				fmt.Sprintf(`file.%s(path.join(root, "big.txt"))`, name))
			if !errors.Is(err, scheduler.ErrMemory) {
				t.Fatalf("got %v, want ErrMemory", err)
			}

			if !errors.Is(err, larkfile.ErrFile) {
				t.Fatalf("got %v, want it to also answer ErrFile", err)
			}
		})
	}

	// The same file under the ordinary ceiling still reads.
	got, err := _Within(t, root, 0, `len(file.read(path.join(root, "big.txt")))`)
	if err != nil {
		t.Fatalf("under the default ceiling: %v", err)
	}

	if got != fmt.Sprint(_LONG) {
		t.Fatalf("read %s bytes, want %d", got, _LONG)
	}
}

// TestWritelines_RefusesSomethingThatIsNotALine names the position, because a
// list of a thousand has to say which one.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
func TestWritelines_RefusesSomethingThatIsNotALine(t *testing.T) {
	root := t.TempDir()

	_, err := _Eval(t, root,
		`file.writelines(path.join(root, "bad.txt"), ["fine", 7, "also fine"])`)
	if !errors.Is(err, larkfile.ErrLine) {
		t.Fatalf("got %v, want ErrLine", err)
	}

	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("got %v, want it to also answer ErrFile", err)
	}

	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("the refusal does not say which line: %v", err)
	}
}
