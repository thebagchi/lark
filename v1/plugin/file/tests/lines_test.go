package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime/common"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/time"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// LONG is a line far past any buffer a reader would choose by itself.
	//
	// Revisions:
	//   - 2026-09-24 23:30: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LONG = 2 << 20

	// BIG is how many lines the memory proof writes.
	//
	// Revisions:
	//   - 2026-09-24 23:30: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BIG = 400000

	// FILLER is the text every one of those lines holds.
	//
	// Revisions:
	//   - 2026-09-24 23:30: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FILLER = "the quick brown fox jumps over the lazy dog, and then does it again"
)

// _Within evaluates one expression inside a run, so a walk that fails has a run
// to end.
//
// The order scheduler.Eval uses: end the run, then let what ended it win.
// A walk that fails has already returned by the time it knows, so the run's
// outcome is the only place its error can be.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-10-02 17:12: gives the script this plugin, which a host hands over
//     rather than one that registers itself
//   - 2026-10-03 20:55: returns what ending the run answers with, rather than applying
//     the rule itself
//   - 2026-11-01 13:00: takes no ceiling
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
func _Within(
	t *testing.T,
	root string,
	expression string,
) (string, error) {
	t.Helper()

	held := _Environment(t)
	held["root"] = starlark.String(root)

	var (
		thread = &starlark.Thread{
			Name: SCRIPT,
		}
		ending = scheduler.Begin(
			context.Background(),
			thread,
			SCRIPT,
			new(common.Settings),
		)
	)

	var produced error

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		thread,
		SCRIPT,
		expression,
		held,
	)

	produced = ending(err)
	if produced != nil {
		return "", produced
	}

	return value.String(), nil
}

// _Put writes a file under root and answers with its path.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Put(
	t *testing.T,
	root string,
	name string,
	body string,
) string {
	t.Helper()

	var (
		named = filepath.Join(
			root,
			name,
		)
		err = os.WriteFile(
			named,
			[]byte(body),
			0o600,
		)
	)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStat_AnswersWithWhatTheSyscallRead(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"note.txt",
		"hello",
	)

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
		t.Run(
			expression,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					root,
					expression,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						expression,
						err,
					)
				}

				if got != want {
					t.Fatalf(
						"%s gave %s, want %s",
						expression,
						got,
						want,
					)
				}
			},
		)
	}
}

// TestStat_RefusesWhatIsNotThere keeps the module's one error shape.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStat_RefusesWhatIsNotThere(t *testing.T) {
	var root = t.TempDir()

	_, err := _Eval(
		t,
		root,
		`file.stat(path.join(root, "nope.txt"))`,
	)
	if !errors.Is(
		err,
		larkfile.ErrFile,
	) {
		t.Fatalf(
			"got %v, want ErrFile",
			err,
		)
	}

	if !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"the refusal lost the reason underneath: %v",
			err,
		)
	}
}

// TestLines_WalksAndRoundTrips is the ordinary use of all three.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_WalksAndRoundTrips(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"three.txt",
		"alpha\nbeta\ngamma\n",
	)

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
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					root,
					item.expression,
				)
				if err != nil {
					t.Fatalf(
						"%v",
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestAppendlines_JoinsWhateverWasAlreadyThere is the cost of trimming the
// last newline, and the proof that it is paid.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestAppendlines_JoinsWhateverWasAlreadyThere(t *testing.T) {
	var (
		root  = t.TempDir()
		cases = []struct{ name, start, want string }{
			{
				"nothing there yet",
				"",
				`["c", "d"]`,
			},
			{
				"trimmed, as this writer leaves it",
				"a\nb",
				`["a", "b", "c", "d"]`,
			},
			{
				"ended by someone else",
				"a\nb\n",
				`["a", "b", "c", "d"]`,
			},
			{
				"one line, no terminator",
				"a",
				`["a", "c", "d"]`,
			},
		}
	)

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var named = _Put(
					t,
					root,
					"log.txt",
					item.start,
				)

				got, err := _Eval(
					t,
					root,
					fmt.Sprintf(
						`file.appendlines(%q,`+
							` ["c", "d"]) or [line for `+
							`line in `+
							`file.lines(%q)]`,
						named,
						named,
					),
				)
				if err != nil {
					t.Fatal(err)
				}

				if got != item.want {
					t.Fatalf(
						"starting from %q gave %s, want %s",
						item.start,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestLines_RefusesAtTheCallThatNamedTheFile is where an error belongs: the
// line that names a missing file, not the loop three lines later.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRegular, formerly ErrNotAFile (C121, Q349)
func TestLines_RefusesAtTheCallThatNamedTheFile(t *testing.T) {
	var (
		root  = t.TempDir()
		cases = map[string]struct {
			expression string
			want       error
		}{
			"missing": {
				`file.lines(path.join(root, "nope.txt"))`,
				os.ErrNotExist,
			},
			"a directory": {
				`file.lines(root)`,
				larkfile.ErrRegular,
			},
		}
	)

	for name, item := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					root,
					item.expression,
				)
				if !errors.Is(
					err,
					larkfile.ErrFile,
				) {
					t.Fatalf(
						"got %v, want ErrFile",
						err,
					)
				}

				if !errors.Is(
					err,
					item.want,
				) {
					t.Fatalf(
						"got %v, want it to carry %v",
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestLines_ANamedFileNeverWalkedOpensNothing is why the handle waits for the
// loop rather than opening where the file was named.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_ANamedFileNeverWalkedOpensNothing(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"idle.txt",
		"one\n",
	)

	var before = _Handles(t)

	_, err := _Eval(
		t,
		root,
		`[file.lines(path.join(root, "idle.txt")) for i in range(200)] and "named"`,
	)
	if err != nil {
		t.Fatal(err)
	}

	after := _Handles(t)
	if after > before+1 {
		t.Fatalf(
			"naming 200 files left %d handles open, was %d",
			after,
			before,
		)
	}
}

// _Handles counts this process's open descriptors, or skips when the system
// does not say.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Handles(t *testing.T) int {
	t.Helper()

	held, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf(
			"no descriptor count on this system: %v",
			err,
		)
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
//   - 2026-11-01 13:00: calls _Within without a ceiling
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_IsWalkedOnce(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"once.txt",
		"a\nb\n",
	)

	_, err := _Within(
		t,
		root,
		`[[line for line in held] for held in `+
			`[file.lines(path.join(root, "once.txt"))] * 2]`,
	)
	if !errors.Is(
		err,
		larkfile.ErrWalked,
	) {
		t.Fatalf(
			"got %v, want ErrWalked",
			err,
		)
	}

	got, err := _Eval(
		t,
		root,
		`[line for line in file.lines(path.join(root, "once.txt"))]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != `["a", "b"]` {
		t.Fatalf(
			"got %s",
			got,
		)
	}
}

// TestLines_CostsOneLineNotTheFile is the measurement the whole change is for.
//
// read of a 512MB file peaked at 1057MB resident, because ReadFile allocates
// the bytes and starlark.String copies them. A walk reuses one buffer, so what
// it costs is set by the longest line rather than by the file - which is why
// this needs no limit on how big a file may be.
//
// HeapAlloc counts garbage the collector has not yet swept, so its reading
// follows the collector's pace as well as the walk: it grew from 17MB to 38MB
// when GOGC went from 100 to 400. The test sets the GC percent to 10 so the
// garbage above the live heap stays small, and restores it with a defer. That
// is safe because the setting is process-wide and no test in this package calls
// t.Parallel, so nothing else runs while it is lowered.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-02 00:02: bound the unswept garbage with a low GC percent, so the reading
//     does not depend on GOGC or on load
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_CostsOneLineNotTheFile(t *testing.T) {
	var (
		root  = t.TempDir()
		named = filepath.Join(
			root,
			"big.txt",
		)
	)

	held, err := os.Create(named)
	if err != nil {
		t.Fatal(err)
	}

	for range BIG {
		_, err = fmt.Fprintln(
			held,
			FILLER,
		)
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

	previous := debug.SetGCPercent(10)
	defer debug.SetGCPercent(previous)

	runtime.GC()
	runtime.ReadMemStats(&before)

	got, err := _Eval(
		t,
		root,
		`len([1 for line in file.lines(path.join(root, "big.txt"))])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	runtime.ReadMemStats(&after)

	t.Logf(
		"file %d bytes, walk allocated %d in total, heap reached %d",
		about.Size(),
		after.TotalAlloc-before.TotalAlloc,
		after.HeapAlloc,
	)

	if got != fmt.Sprint(BIG) {
		t.Fatalf(
			"walked %s lines, want %d",
			got,
			BIG,
		)
	}

	// The list of results is what grows here, not the reader. What matters is
	// that nothing ever held the file: the peak heap, not the running total.
	if after.HeapAlloc > uint64(about.Size()) {
		t.Fatalf(
			"heap reached %d on a %d byte file, so the walk is holding it",
			after.HeapAlloc,
			about.Size(),
		)
	}
}

// TestLines_ALongLineIsReadRatherThanRefused is what replaced a fixed limit:
// a line may be as long as the file is.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-01 13:00: a line may be as long as the file is, no budget now
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_ALongLineIsReadRatherThanRefused(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"long.txt",
		strings.Repeat(
			"x",
			LONG,
		)+"\nshort\n",
	)

	got, err := _Eval(
		t,
		root,
		`[len(line) for line in file.lines(path.join(root, "long.txt"))]`,
	)
	if err != nil {
		t.Fatalf(
			"a %d byte line was refused: %v",
			LONG,
			err,
		)
	}

	want := fmt.Sprintf(
		"[%d, 5]",
		LONG,
	)
	if got != want {
		t.Fatalf(
			"got %s, want %s",
			got,
			want,
		)
	}
}

// TestRead_ABigFileIsReadWhole is the answer to the size question that needs
// no limit on how big a file may be: a file of LONG bytes is read, whole.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation, as
//     TestRead_TooBigForTheRunIsRefusedBeforeItIsAllocated
//   - 2026-11-01 13:00: only the read that succeeds is left, the budget that
//     refused the other being gone
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRead_ABigFileIsReadWhole(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"big.txt",
		strings.Repeat(
			"y",
			LONG,
		),
	)

	got, err := _Eval(
		t,
		root,
		`len(file.read(path.join(root, "big.txt")))`,
	)
	if err != nil {
		t.Fatalf(
			"a file of %d bytes: %v",
			LONG,
			err,
		)
	}

	if got != fmt.Sprint(LONG) {
		t.Fatalf(
			"read %s bytes, want %d",
			got,
			LONG,
		)
	}
}

// TestWritelines_RefusesSomethingThatIsNotALine names the position, because a
// list of a thousand has to say which one.
//
// Revisions:
//   - 2026-09-24 23:14: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWritelines_RefusesSomethingThatIsNotALine(t *testing.T) {
	var root = t.TempDir()

	_, err := _Eval(
		t,
		root,
		`file.writelines(path.join(root, "bad.txt"), ["fine", 7, "also fine"])`,
	)
	if !errors.Is(
		err,
		larkfile.ErrLine,
	) {
		t.Fatalf(
			"got %v, want ErrLine",
			err,
		)
	}

	if !errors.Is(
		err,
		larkfile.ErrFile,
	) {
		t.Fatalf(
			"got %v, want it to also answer ErrFile",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"line 1",
	) {
		t.Fatalf(
			"the refusal does not say which line: %v",
			err,
		)
	}
}

// TestErrFile_IsAnsweredWithoutBeingSaid is the sentinel doing its job
// silently.
//
// ErrFile answers "did a file call fail", which every refusal here answers yes
// to. Wrapping it with %w put its words at the end of every message, where a
// line of the wrong type claimed the filesystem had refused something. The
// question still has to be answerable, so this checks both halves at once:
// every refusal matches ErrFile, and none of them says so.
//
// Revisions:
//   - 2026-09-24 23:46: initial creation
//   - 2026-11-01 13:00: drops the memory refusals, the budget being gone
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestErrFile_IsAnsweredWithoutBeingSaid(t *testing.T) {
	root := t.TempDir()

	_Put(
		t,
		root,
		"note.txt",
		"hello",
	)

	cases := map[string]string{
		"a missing file":           `file.read(path.join(root, "nope.txt"))`,
		"a directory":              `file.read(root)`,
		"walking one":              `[line for line in file.lines(root)]`,
		"a line of the wrong type": `file.writelines(path.join(root, "bad.txt"), [1])`,
		"sizing nothing":           `file.size(path.join(root, "nope.txt"))`,
		"removing nothing":         `file.remove(path.join(root, "nope.txt"))`,
		"listing a file":           `file.list(path.join(root, "note.txt"))`,
	}

	for name, expression := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					root,
					expression,
				)
				if !errors.Is(
					err,
					larkfile.ErrFile,
				) {
					t.Fatalf(
						"%s did not answer ErrFile: %v",
						expression,
						err,
					)
				}

				if strings.Contains(
					err.Error(),
					larkfile.ErrFile.Error(),
				) {
					t.Fatalf(
						"the sentinel put its words in the message: %v",
						err,
					)
				}
			},
		)
	}
}
