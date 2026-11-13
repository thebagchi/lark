// Regression probes for what landed after the file module grew: reading a file
// by lines, what a stat answers with, and a
// sentinel that no longer speaks for the filesystem.
//
// Revisions:
//   - 2026-09-24 23:54: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/plugin/file"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
)

// TestLines_WalksOnceAndAppendsALine is the round trip and the one walk.
//
// writelines puts no newline after the last line, so appendlines has to put
// the separator back or the first added line runs onto the end of the old one.
// The second walk is refused rather than quietly starting again, and the
// refusal answers file.ErrFile without saying the filesystem refused anything.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_WalksOnceAndAppendsALine(t *testing.T) {
	var (
		dir   = t.TempDir()
		named = filepath.Join(
			dir,
			"log.txt",
		)
		value, err = _Run(
			t,
			`
def main():
    path = "`+named+`"
    file.writelines(path, ["a", "b"])
    file.appendlines(path, ["c"])
    out = []
    for line in file.lines(path):
        out.append(line)
    return out
`,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	if value.String() != `["a", "b", "c"]` {
		t.Fatalf(
			"got %s",
			value.String(),
		)
	}

	_, err = _Run(
		t,
		`
def main():
    held = file.lines("`+named+`")
    first = [line for line in held]
    second = [line for line in held]
    return first
`,
	)
	if !errors.Is(
		err,
		file.ErrWalked,
	) || !errors.Is(
		err,
		file.ErrFile,
	) {
		t.Fatalf(
			"second walk: %v",
			err,
		)
	}

	if strings.Contains(
		err.Error(),
		file.ErrFile.Error(),
	) {
		t.Fatalf(
			"the sentinel is in the message: %v",
			err,
		)
	}
}

// TestLines_OneWalkerWins is the claim on a walk taken atomically.
//
// Two threads holding one lines value both try to read it. One wins and the
// other is refused; without the atomic claim both could open the file and each
// would see half the lines, which is the kind of result nobody could explain
// afterwards.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
//   - 2026-09-29 23:30: hands both threads the lines value through a lambda
//     written inside spawn, since a nested walk capturing it is now refused
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLines_OneWalkerWins(t *testing.T) {
	var (
		dir   = t.TempDir()
		named = filepath.Join(
			dir,
			"log.txt",
		)
	)
	if err := os.WriteFile(
		named,
		[]byte("a\nb\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := _Run(
		t,
		`
def walk(lines):
    n = 0
    for line in lines:
        n = n + 1
    return n

def main():
    lines = file.lines("`+named+`")
    first = spawn(lambda: walk(lines))
    second = spawn(lambda: walk(lines))
    return join(first, second)
`,
	)
	if !errors.Is(
		err,
		file.ErrWalked,
	) {
		t.Fatalf(
			"two walks: %v",
			err,
		)
	}
}

// TestStat_ModeIsANumberAndModifiedIsATime pins the two fields whose type
// cannot be guessed from outside.
//
// mode is the permission bits as a number, so masking works; a string would
// have to be parsed first. modified is the time module's own value, because
// from_timestamp takes whole seconds and a float would lose precision before a
// script could convert it.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStat_ModeIsANumberAndModifiedIsATime(t *testing.T) {
	var (
		dir   = t.TempDir()
		named = filepath.Join(
			dir,
			"note.txt",
		)
	)
	if err := os.WriteFile(
		named,
		[]byte("hi"),
		0o640,
	); err != nil {
		t.Fatal(err)
	}

	value, err := _Run(
		t,
		`
def main():
    info = file.stat("`+named+`")

    return [type(info.mode), type(info.modified), info.size, info.dir, info.mode & 0o700]
`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if value.String() != `["int", "time.time", 2, False, 384]` {
		t.Fatalf(
			"got %s",
			value.String(),
		)
	}
}

// TestErrFile_DoesNotSayTheFilesystemRefused checks the sentinel answers
// without speaking.
//
// file.ErrFile is carried through Unwrap rather than wrapped, so errors.Is still
// finds it while its words stay out of the message. The cause underneath has
// to survive that: a missing file is still os.ErrNotExist, and a directory
// nobody may search is still os.ErrPermission.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRegular, formerly ErrNotAFile (C121, Q349)
func TestErrFile_DoesNotSayTheFilesystemRefused(t *testing.T) {
	var (
		dir    = t.TempDir()
		_, err = _Run(
			t,
			`
def main():
    return file.read("`+filepath.Join(
				dir,
				"missing.txt",
			)+`")
`,
		)
	)
	if !errors.Is(
		err,
		file.ErrFile,
	) || !errors.Is(
		err,

		os.ErrNotExist,
	) {
		t.Fatalf(
			"missing: %v",
			err,
		)
	}

	if strings.Contains(
		err.Error(),
		file.ErrFile.Error(),
	) {
		t.Fatalf(
			"sentinel text: %v",
			err,
		)
	}

	_, err = _Run(
		t,
		`
def main():
    return file.read("`+dir+`")
`,
	)
	if !errors.Is(
		err,
		file.ErrFile,
	) || !errors.Is(
		err,
		file.ErrRegular,
	) {
		t.Fatalf(

			"directory: %v",
			err,
		)
	}

	if os.Geteuid() == 0 {
		t.Skip("root can search a mode 000 directory")
	}

	secret := filepath.Join(

		dir,
		"secret",
	)
	if err := os.Mkdir(
		secret,
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	note := filepath.Join(

		secret,
		"note",
	)
	if err := os.WriteFile(
		note,
		[]byte("x"),

		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(
		secret,
		0,
	); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(
			secret,
			0o755,
		)
	})

	_, err = _Run(
		t,
		`
def main():
    return file.exists("`+note+`")
`,
	)
	if !errors.Is(
		err,
		os.ErrPermission,
	) || !errors.Is(
		err,
		file.ErrFile,
	) {
		t.Fatalf(
			"permission: %v",
			err,
		)
	}
}

// TestLark_RunsABundleItWrote is compile and run as a pair: a script compiled into a
// bundle by one command runs from that bundle alone in another.
//
// Revisions:
//   - 2026-10-03 00:28: initial creation
//   - 2026-11-01 12:30: runs lark with -script, -bundle and -run
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 11:45: runs lark with the verbs compile and run; the bundle ends in .bin, which
//     is how run knows it (C119, Q303, Q308)
func TestLark_RunsABundleItWrote(t *testing.T) {
	var (
		root   = _ModuleRoot(t)
		dir    = t.TempDir()
		script = filepath.Join(
			dir,
			"said.star",
		)
		bundle = filepath.Join(
			dir,
			"said.bin",
		)
		err = os.WriteFile(
			script,
			[]byte("def main():\n    print(\"from the bundle\")\n"),
			0o600,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	_Lark(
		t,
		root,
		"compile",
		"-o",
		bundle,
		script,
	)

	out := _Lark(
		t,
		root,
		"run",
		bundle,
	)
	if !strings.Contains(
		string(out),
		"from the bundle",
	) {
		t.Fatalf(
			"running the bundle printed\n%s",
			out,
		)
	}
}

// _LarkCode runs the built binary and answers with the exit code it chose.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _LarkCode(
	t *testing.T,
	bin string,
	args ...string,
) (int, []byte) {
	t.Helper()

	var (
		cmd = exec.Command(
			bin,
			args...,
		)
		out, err = cmd.CombinedOutput()
	)
	if err == nil {
		return 0, out
	}

	var exit *exec.ExitError
	if errors.As(
		err,
		&exit,
	) {
		return exit.ExitCode(), out
	}

	t.Fatalf(
		"lark: %v\n%s",
		err,
		out,
	)

	return 0, out
}
