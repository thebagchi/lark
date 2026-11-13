package tests

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// SCRIPT is what a failure calls the expression it was given.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SCRIPT = "file_test.star"

	// DEVICE is the character device the unbounded read was found on. It has
	// no end, so reading it is the defect.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DEVICE = "/dev/zero"

	// DEADLINE is how long a refusal may take before the read is taken to be
	// unbounded. A refusal is a stat and a comparison, so this is enormous on
	// purpose.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DEADLINE = 10 * time.Second
)

// _Eval evaluates one expression, with root bound so a fixture can name paths
// under a directory of its own.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-10-02 17:12: gives the script this plugin, which a host hands over
//     rather than one that registers itself
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Eval(
	t *testing.T,
	root string,
	expression string,
) (string, error) {
	t.Helper()

	held := _Environment(t)
	held["root"] = starlark.String(root)

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		expression,
		held,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Environment is what a script here is given: every plugin registered by
// import, path among them, and this one, which a host hands over.
//
// Revisions:
//   - 2026-10-02 17:12: initial creation
//   - 2026-10-03 08:29: merges a list, there being no registry
//   - 2026-11-02 08:27: takes a testing.TB, so a benchmark may call it
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Environment(tb testing.TB) starlark.StringDict {
	tb.Helper()

	var plugins = slices.Concat(
		plugin.Registered(),
		[]common.Plugin{larkfile.New()},
	)

	env, err := plugin.Environment(plugins)
	if err != nil {
		tb.Fatalf(
			"environment: %v",
			err,
		)
	}

	return env
}

// TestFile_WritesAndReadsBack is the round trip, in both the shapes this
// offers.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_WritesAndReadsBack(t *testing.T) {
	var (
		root  = t.TempDir()
		cases = []struct{ name, expression, want string }{
			{
				"write then read",
				`file.write(path.join(root, "a.txt"), "hello") or ` +
					`file.read(path.join(root, "a.txt"))`,
				`"hello"`,
			},
			{
				"read as bytes",
				`file.bytes(path.join(root, "a.txt"))`,
				`b"hello"`,
			},
			{
				"write bytes",
				`file.write(path.join(root, "b.bin"), b"\x00\xff") or ` +
					`file.bytes(path.join(root, "b.bin"))`,
				`b"\x00\xff"`,
			},
			{
				"append",
				`file.append(path.join(root, "a.txt"), " there") or ` +
					`file.read(path.join(root, "a.txt"))`,
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
	)

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
						"%s: %v",
						item.expression,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestFile_AsksAndLists records the two questions that do not fail when the
// answer is no or empty.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_AsksAndLists(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"b.txt", "a.txt", "c.txt"} {
		err := os.WriteFile(
			filepath.Join(
				root,
				name,
			),
			nil,
			0o600,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct{ name, expression, want string }{
		{
			"exists",
			`file.exists(path.join(root, "a.txt"))`,
			"True",
		},
		{
			"does not exist",
			`file.exists(path.join(root, "nope.txt"))`,
			"False",
		},
		{
			"list is sorted",
			`file.list(root)`,
			`["a.txt", "b.txt", "c.txt"]`,
		},
		{
			"remove then gone",
			`file.remove(path.join(root, "a.txt")) or file.exists(path.join(root, ` +
				`"a.txt"))`,
			"False",
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
						"%s: %v",
						item.expression,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestFile_RemovesOneThingAndNeverATree is the refusal that stops a mistyped
// path undoing an afternoon's work.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_RemovesOneThingAndNeverATree(t *testing.T) {
	var root = t.TempDir()

	err := os.WriteFile(
		filepath.Join(
			root,
			"kept.txt",
		),
		nil,
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = _Eval(
		t,
		root,
		`file.remove(root)`,
	)
	if !errors.Is(
		err,
		larkfile.ErrFile,
	) {
		t.Fatalf(
			"removing a directory with something in it gave %v, want ErrFile",
			err,
		)
	}

	got, err := _Eval(
		t,
		root,
		`file.exists(path.join(root, "kept.txt"))`,
	)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_ReportsWhatTheFilesystemRefused(t *testing.T) {
	var (
		root  = t.TempDir()
		cases = []struct{ name, expression string }{
			{
				"reading nothing",
				`file.read(path.join(root, "nope.txt"))`,
			},
			{
				"listing a file",
				`file.list(path.join(root, "nope.txt"))`,
			},
			{
				"sizing nothing",
				`file.size(path.join(root, "nope.txt"))`,
			},
			{
				"removing nothing",
				`file.remove(path.join(root, "nope.txt"))`,
			},
		}
	)

	for _, item := range cases {
		t.Run(
			item.name,
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
						"%s gave %v, want ErrFile",
						item.expression,
						err,
					)
				}

				if !errors.Is(
					err,
					os.ErrNotExist,
				) {
					t.Fatalf(
						"%s lost the reason underneath: %v",
						item.expression,
						err,
					)
				}
			},
		)
	}
}

// TestFile_MakesDirectories records that making one twice is not a failure.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_MakesDirectories(t *testing.T) {
	root := t.TempDir()

	for range 2 {
		_, err := _Eval(
			t,
			root,
			`file.mkdir(path.join(root, "made", "deeper"))`,
		)
		if err != nil {
			t.Fatalf(
				"mkdir: %v",
				err,
			)
		}
	}

	got, err := _Eval(
		t,
		root,
		`file.list(path.join(root, "made"))`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != `["deeper"]` {
		t.Fatalf(
			"got %s, want the directory made once",
			got,
		)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestExists_TellsAbsenceFromBeingUnableToLook(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may search a directory whatever its mode")
	}

	var (
		root   = t.TempDir()
		closed = filepath.Join(
			root,
			"secret",
		)
		err = os.Mkdir(
			closed,
			0o700,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	hidden := filepath.Join(
		closed,
		"note",
	)

	err = os.WriteFile(
		hidden,
		[]byte("still here"),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chmod(
		closed,
		0o000,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Put it back, or the temporary directory cannot be removed.
	defer func() {
		err := os.Chmod(
			closed,
			0o700,
		)
		if err != nil {
			t.Error(err)
		}
	}()

	_, err = _Eval(
		t,
		root,
		`file.exists(path.join(root, "secret", "note"))`,
	)
	if !errors.Is(
		err,
		larkfile.ErrFile,
	) {
		t.Fatalf(
			"a file behind a closed directory gave %v, want ErrFile",
			err,
		)
	}

	if !errors.Is(
		err,
		os.ErrPermission,
	) {
		t.Fatalf(
			"the refusal lost the reason underneath: %v",
			err,
		)
	}

	// And absence is still False rather than a refusal.
	got, err := _Eval(
		t,
		root,
		`file.exists(path.join(root, "nope.txt"))`,
	)
	if err != nil {
		t.Fatalf(
			"asking about an absent file gave %v, want False",
			err,
		)
	}

	if got != "False" {
		t.Fatalf(
			"an absent file gave %s, want False",
			got,
		)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRegular, formerly ErrNotAFile (C121, Q349)
func TestRead_RefusesWhatIsNotARegularFile(t *testing.T) {
	var root = t.TempDir()

	err := os.WriteFile(
		filepath.Join(
			root,
			"ordinary.txt",
		),
		[]byte("text"),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(
		filepath.Join(
			root,
			"inner",
		),
		0o750,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"a directory":  `file.read(path.join(root, "inner"))`,
		"and as bytes": `file.bytes(path.join(root, "inner"))`,
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
					larkfile.ErrRegular,
				) {
					t.Fatalf(
						"%s gave %v, want ErrRegular",
						expression,
						err,
					)
				}

				// And it still answers the general question, so a caller
				// branching on ErrFile does not miss it.
				if !errors.Is(
					err,
					larkfile.ErrFile,
				) {
					t.Fatalf(
						"%s did not also match ErrFile: %v",
						expression,
						err,
					)
				}
			},
		)
	}

	// The device is the case this exists for, and it is checked apart from
	// the others because a regression does not fail here - it hangs. With the
	// guard removed this call never returns, so the deadline is what turns
	// "the fix is gone" into a failure instead of a suite that has to be
	// killed. Measured: 174 seconds and still reading.
	if _, err := os.Stat(DEVICE); err == nil {
		t.Run(
			"a device with no end",
			func(t *testing.T) {
				done := make(
					chan error,
					1,
				)

				go func() {
					_, err := _Eval(
						t,
						root,
						`file.read("`+DEVICE+`")`,
					)
					done <- err
				}()

				select {
				case err := <-done:
					if !errors.Is(
						err,
						larkfile.ErrRegular,
					) {
						t.Fatalf(
							"reading %s gave %v, want ErrRegular",
							DEVICE,
							err,
						)
					}
				case <-time.After(DEADLINE):
					t.Fatalf(
						"reading"+
							" %s did not return: the read is "+
							"unbounded again",
						DEVICE,
					)
				}
			},
		)
	}

	// An ordinary file is untouched by any of this.
	got, err := _Eval(
		t,
		root,
		`file.read(path.join(root, "ordinary.txt"))`,
	)
	if err != nil {
		t.Fatalf(
			"reading an ordinary file gave %v",
			err,
		)
	}

	if got != `"text"` {
		t.Fatalf(
			"an ordinary file read as %s, want \"text\"",
			got,
		)
	}
}

// TestWrite_RefusesWhatIsNotARegularFile proves write, append, writelines and
// appendlines refuse a named pipe and a directory with ErrRegular, as read
// does, and do not wait for a reader that never comes (B241).
//
// With the guard removed each call on the pipe blocks in open(2), so the
// deadline turns the defect into a failure. The test then opens the pipe for
// reading, so the blocked goroutine ends.
//
// Revisions:
//   - 2026-11-10 11:48: initial creation (C117, B241)
//   - 2026-11-10 17:15: uses ErrRegular, formerly ErrNotAFile (C121, Q349)
//   - 2026-11-12 17:20: makes and releases the pipe with _Pipe and _Release, which skip
//     where there are no pipes (C128, B277)
func TestWrite_RefusesWhatIsNotARegularFile(t *testing.T) {
	var (
		root = t.TempDir()
		fifo = filepath.Join(
			root,
			"pipe",
		)
	)

	_Pipe(
		t,
		fifo,
	)

	err := os.Mkdir(
		filepath.Join(
			root,
			"inner",
		),
		0o750,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"pipe", "inner"} {
		for _, call := range []string{"write", "append", "writelines", "appendlines"} {
			t.Run(
				call+" "+target,
				func(t *testing.T) {
					argument := `"a"`
					if strings.HasSuffix(
						call,
						"lines",
					) {
						argument = `["a"]`
					}

					var (
						done = make(
							chan error,
							1,
						)
						path = `path.join(root, "` + target + `")`
					)

					go func() {
						_, err := _Eval(
							t,
							root,
							"file."+call+"("+path+", "+argument+")",
						)
						done <- err
					}()

					select {
					case err := <-done:
						if !errors.Is(
							err,
							larkfile.ErrRegular,
						) || !errors.Is(
							err,
							larkfile.ErrFile,
						) {
							t.Fatalf(
								"got %v, want both sentinels",
								err,
							)
						}
					case <-time.After(DEADLINE):
						_Release(fifo)

						t.Fatalf(
							"file.%s on a %s did not return",
							call,
							target,
						)
					}
				},
			)
		}
	}
}

// TestFile_RejectionQuotesAShortPrefixOfThePath makes a file with a name of
// 200 bytes, then writes a line of the wrong type to it. The refusal is the
// module's own, so it names the path, and the name must stop at
// common.QUOTE_LIMIT bytes with the marker.
//
// Revisions:
//   - 2026-11-02 11:04: initial creation, for decision Q4 (change C21)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFile_RejectionQuotesAShortPrefixOfThePath(t *testing.T) {
	var (
		root = t.TempDir()
		name = filepath.Join(
			root,
			strings.Repeat(
				"z",
				200,
			),
		)
	)

	_, err := _Eval(
		t,
		root,
		"file.writelines(path.join(root, '"+strings.Repeat(
			"z",
			200,
		)+"'), [1])",
	)
	if err == nil {
		t.Fatal("got no error for a line that is not a string")
	}

	want := "file.writelines: " + unpack.ShortText(name) + ": line 0 is int: not a line"
	if err.Error() != want {
		t.Fatalf(
			"got %q, want %q",
			err.Error(),
			want,
		)
	}

	if !errors.Is(
		err,
		larkfile.ErrFile,
	) {
		t.Fatalf(
			"got %v, want ErrFile",
			err,
		)
	}
}
