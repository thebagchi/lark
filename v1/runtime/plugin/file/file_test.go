package file_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	larkfile "github.com/thebagchi/lark/v1/runtime/plugin/file"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
)

const (
	// SCRIPT is what a failure calls the expression it was given.
	SCRIPT = "file_test.star"

	// _DEVICE is the character device the unbounded read was found on. It has
	// no end, so reading it is the defect.
	_DEVICE = "/dev/zero"

	// _DEADLINE is how long a refusal may take before the read is taken to be
	// unbounded. A refusal is a stat and a comparison, so this is enormous on
	// purpose.
	_DEADLINE = 10 * time.Second
)

// _Eval evaluates one expression, with root bound so a fixture can name paths
// under a directory of its own.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Eval(t *testing.T, root string, expression string) (string, error) {
	t.Helper()

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	held := starlark.StringDict{}

	for name, value := range env {
		held[name] = value
	}

	held["root"] = starlark.String(root)

	value, err := starlark.EvalOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		expression,
		held,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// TestFile_WritesAndReadsBack is the round trip, in both the shapes this
// offers.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFile_WritesAndReadsBack(t *testing.T) {
	root := t.TempDir()

	cases := []struct{ name, expression, want string }{
		{
			"write then read",
			`file.write(path.join(root, "a.txt"), "hello") or file.read(path.join(root, "a.txt"))`,
			`"hello"`,
		},
		{
			"read as bytes",
			`file.bytes(path.join(root, "a.txt"))`,
			`b"hello"`,
		},
		{
			"write bytes",
			`file.write(path.join(root, "b.bin"), b"\x00\xff") or file.bytes(path.join(root, "b.bin"))`,
			`b"\x00\xff"`,
		},
		{
			"append",
			`file.append(path.join(root, "a.txt"), " there") or file.read(path.join(root, "a.txt"))`,
			`"hello there"`,
		},
		{
			"size",
			`file.size(path.join(root, "a.txt"))`,
			"11",
		},
		{
			"write makes the directory above it",
			`file.write(path.join(root, "deep", "down", "c.txt"), "x") or ` +
				`file.read(path.join(root, "deep", "down", "c.txt"))`,
			`"x"`,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, root, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.expression, got, item.want)
			}
		})
	}
}

// TestFile_AsksAndLists records the two questions that do not fail when the
// answer is no or empty.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFile_AsksAndLists(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"b.txt", "a.txt", "c.txt"} {
		err := os.WriteFile(filepath.Join(root, name), nil, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct{ name, expression, want string }{
		{"exists", `file.exists(path.join(root, "a.txt"))`, "True"},
		{"does not exist", `file.exists(path.join(root, "nope.txt"))`, "False"},
		{"list is sorted", `file.list(root)`, `["a.txt", "b.txt", "c.txt"]`},
		{
			"remove then gone",
			`file.remove(path.join(root, "a.txt")) or file.exists(path.join(root, "a.txt"))`,
			"False",
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, root, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.expression, got, item.want)
			}
		})
	}
}

// TestFile_RemovesOneThingAndNeverATree is the refusal that stops a mistyped
// path undoing an afternoon's work.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFile_RemovesOneThingAndNeverATree(t *testing.T) {
	root := t.TempDir()

	err := os.WriteFile(filepath.Join(root, "kept.txt"), nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = _Eval(t, root, `file.remove(root)`)
	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("removing a directory with something in it gave %v, want ErrFile", err)
	}

	got, err := _Eval(t, root, `file.exists(path.join(root, "kept.txt"))`)
	if err != nil {
		t.Fatal(err)
	}

	if got != "True" {
		t.Fatal("the refusal did not leave the directory alone")
	}
}

// TestFile_ReportsWhatTheFilesystemRefused records that a failure carries both
// this package's sentinel and the reason underneath it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFile_ReportsWhatTheFilesystemRefused(t *testing.T) {
	root := t.TempDir()

	cases := []struct{ name, expression string }{
		{"reading nothing", `file.read(path.join(root, "nope.txt"))`},
		{"listing a file", `file.list(path.join(root, "nope.txt"))`},
		{"sizing nothing", `file.size(path.join(root, "nope.txt"))`},
		{"removing nothing", `file.remove(path.join(root, "nope.txt"))`},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, root, item.expression)
			if !errors.Is(err, larkfile.ErrFile) {
				t.Fatalf("%s gave %v, want ErrFile", item.expression, err)
			}

			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s lost the reason underneath: %v", item.expression, err)
			}
		})
	}
}

// TestFile_MakesDirectories records that making one twice is not a failure.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFile_MakesDirectories(t *testing.T) {
	root := t.TempDir()

	for range 2 {
		_, err := _Eval(t, root, `file.mkdir(path.join(root, "made", "deeper"))`)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	got, err := _Eval(t, root, `file.list(path.join(root, "made"))`)
	if err != nil {
		t.Fatal(err)
	}

	if got != `["deeper"]` {
		t.Fatalf("got %s, want the directory made once", got)
	}
}

// TestExists_TellsAbsenceFromBeingUnableToLook is the difference between "not
// there" and "I cannot tell".
//
// A directory this process may not search used to answer False, which says the
// file is not there when it is - and a script deciding whether to write would
// overwrite something it could not see.
//
// Skipped as root, who may search anything, so the mode this sets up would not
// stop the stat and the test would pass without having looked.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestExists_TellsAbsenceFromBeingUnableToLook(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may search a directory whatever its mode")
	}

	root := t.TempDir()
	closed := filepath.Join(root, "secret")

	err := os.Mkdir(closed, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	hidden := filepath.Join(closed, "note")

	err = os.WriteFile(hidden, []byte("still here"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chmod(closed, 0o000)
	if err != nil {
		t.Fatal(err)
	}

	// Put it back, or the temporary directory cannot be removed.
	defer func() {
		err := os.Chmod(closed, 0o700)
		if err != nil {
			t.Error(err)
		}
	}()

	_, err = _Eval(t, root, `file.exists(path.join(root, "secret", "note"))`)
	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("a file behind a closed directory gave %v, want ErrFile", err)
	}

	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("the refusal lost the reason underneath: %v", err)
	}

	// And absence is still False rather than a refusal.
	got, err := _Eval(t, root, `file.exists(path.join(root, "nope.txt"))`)
	if err != nil {
		t.Fatalf("asking about an absent file gave %v, want False", err)
	}

	if got != "False" {
		t.Fatalf("an absent file gave %s, want False", got)
	}
}

// TestRead_RefusesWhatIsNotARegularFile is the unbounded read, removed without
// inventing a limit.
//
// os.ReadFile sizes its buffer from the file, which bounds a read of anything
// with an end. A character device has none, so reading /dev/zero grows until
// the process dies. Refusing what is not a regular file removes that, and
// catches a directory on the way past - where a size limit would have needed a
// number, and a number is a policy nobody has chosen.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
func TestRead_RefusesWhatIsNotARegularFile(t *testing.T) {
	root := t.TempDir()

	err := os.WriteFile(filepath.Join(root, "ordinary.txt"), []byte("text"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(filepath.Join(root, "inner"), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"a directory":  `file.read(path.join(root, "inner"))`,
		"and as bytes": `file.bytes(path.join(root, "inner"))`,
	}

	for name, expression := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := _Eval(t, root, expression)
			if !errors.Is(err, larkfile.ErrNotAFile) {
				t.Fatalf("%s gave %v, want ErrNotAFile", expression, err)
			}

			// And it still answers the general question, so a caller
			// branching on ErrFile does not miss it.
			if !errors.Is(err, larkfile.ErrFile) {
				t.Fatalf("%s did not also match ErrFile: %v", expression, err)
			}
		})
	}

	// The device is the case this exists for, and it is checked apart from
	// the others because a regression does not fail here - it hangs. With the
	// guard removed this call never returns, so the deadline is what turns
	// "the fix is gone" into a failure instead of a suite that has to be
	// killed. Measured: 174 seconds and still reading.
	if _, err := os.Stat(_DEVICE); err == nil {
		t.Run("a device with no end", func(t *testing.T) {
			done := make(chan error, 1)

			go func() {
				_, err := _Eval(t, root, `file.read("`+_DEVICE+`")`)
				done <- err
			}()

			select {
			case err := <-done:
				if !errors.Is(err, larkfile.ErrNotAFile) {
					t.Fatalf("reading %s gave %v, want ErrNotAFile", _DEVICE, err)
				}
			case <-time.After(_DEADLINE):
				t.Fatalf("reading %s did not return: the read is unbounded again", _DEVICE)
			}
		})
	}

	// An ordinary file is untouched by any of this.
	got, err := _Eval(t, root, `file.read(path.join(root, "ordinary.txt"))`)
	if err != nil {
		t.Fatalf("reading an ordinary file gave %v", err)
	}

	if got != `"text"` {
		t.Fatalf("an ordinary file read as %s, want \"text\"", got)
	}
}
