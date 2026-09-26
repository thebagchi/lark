// Regression probes for what landed after the file module grew: the memory a
// run may hold, reading a file by lines, what a stat answers with, and a
// sentinel that no longer speaks for the filesystem.
package testing_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/file"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestRead_ChargesTwiceAndCreditsTheCopy pins both halves of what a read costs.
//
// os.ReadFile allocates the bytes and starlark.String copies them, so a read
// holds twice the file at once - one byte under that is refused. The second
// read proves the reserve came back, because two reads at exactly twice the
// file only fit if the first gave up its copy.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func TestRead_ChargesTwiceAndCreditsTheCopy(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "note.txt")
	body := strings.Repeat("a", 4096)
	if err := os.WriteFile(named, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `
def main():
    file.read("` + named + `")
    return file.read("` + named + `")
`

	_, err := _RunCtx(t, scheduler.Allowing(t.Context(), int64(len(body)*2-1)), src)
	if !errors.Is(err, scheduler.ErrMemory) {
		t.Fatalf("a file of %d bytes under a ceiling of %d: %v", len(body), len(body)*2-1, err)
	}

	value, err := _RunCtx(t, scheduler.Allowing(t.Context(), int64(len(body)*2)), src)
	if err != nil {
		t.Fatalf("two reads at twice the file, after a credit: %v", err)
	}
	if !strings.Contains(value.String(), "a") {
		t.Fatalf("got %s", value.String())
	}
}

// TestLines_WalksOnceAndAppendsALine is the round trip and the one walk.
//
// writelines puts no newline after the last line, so appendlines has to put
// the separator back or the first added line runs onto the end of the old one.
// The second walk is refused rather than quietly starting again, and the
// refusal answers ErrFile without saying the filesystem refused anything.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func TestLines_WalksOnceAndAppendsALine(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "log.txt")

	value, err := _Run(t, `
def main():
    path = "`+named+`"
    file.writelines(path, ["a", "b"])
    file.appendlines(path, ["c"])
    out = []
    for line in file.lines(path):
        out.append(line)
    return out
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `["a", "b", "c"]` {
		t.Fatalf("got %s", value.String())
	}

	_, err = _Run(t, `
def main():
    held = file.lines("`+named+`")
    first = [line for line in held]
    second = [line for line in held]
    return first
`)
	if !errors.Is(err, file.ErrWalked) || !errors.Is(err, file.ErrFile) {
		t.Fatalf("second walk: %v", err)
	}
	if strings.Contains(err.Error(), file.ErrFile.Error()) {
		t.Fatalf("the sentinel is in the message: %v", err)
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
func TestLines_OneWalkerWins(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(named, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := _Run(t, `
def main():
    box = [file.lines("`+named+`")]
    def walk():
        n = 0
        for line in box[0]:
            n = n + 1
        return n
    return join(spawn(walk), spawn(walk))
`)
	if !errors.Is(err, file.ErrWalked) {
		t.Fatalf("two walks: %v", err)
	}
}

// TestLines_AShortLineFitsWhenTheFileDoesNot is the whole argument for lines
// in one test.
//
// The same file under the same ceiling: read is refused because it wants the
// file, and a walk succeeds because it wants a line. That is why the module
// needs no limit on how big a file may be.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func TestLines_AShortLineFitsWhenTheFileDoesNot(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "wide.txt")
	line := strings.Repeat("x", 200)
	var body strings.Builder
	for range 40 {
		body.WriteString(line)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(named, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := _RunCtx(t, scheduler.Allowing(t.Context(), 1000), `
def main():
    return file.read("`+named+`")
`)
	if !errors.Is(err, scheduler.ErrMemory) {
		t.Fatalf("read of a file past the ceiling: %v", err)
	}

	value, err := _RunCtx(t, scheduler.Allowing(t.Context(), 1000), `
def main():
    n = 0
    for line in file.lines("`+named+`"):
        n = n + 1
    return n
`)
	if err != nil {
		t.Fatalf("lines under the same ceiling: %v", err)
	}
	if value.String() != "40" {
		t.Fatalf("got %s", value.String())
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
func TestStat_ModeIsANumberAndModifiedIsATime(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(named, []byte("hi"), 0o640); err != nil {
		t.Fatal(err)
	}

	value, err := _Run(t, `
def main():
    info = file.stat("`+named+`")
    return [type(info.mode), type(info.modified), info.size, info.dir, info.mode & 0o700]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `["int", "time.time", 2, False, 384]` {
		t.Fatalf("got %s", value.String())
	}
}

// TestErrFile_DoesNotSayTheFilesystemRefused checks the sentinel answers
// without speaking.
//
// ErrFile is carried through Unwrap rather than wrapped, so errors.Is still
// finds it while its words stay out of the message. The cause underneath has
// to survive that: a missing file is still os.ErrNotExist, and a directory
// nobody may search is still os.ErrPermission.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func TestErrFile_DoesNotSayTheFilesystemRefused(t *testing.T) {
	dir := t.TempDir()
	_, err := _Run(t, `
def main():
    return file.read("`+filepath.Join(dir, "missing.txt")+`")
`)
	if !errors.Is(err, file.ErrFile) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if strings.Contains(err.Error(), file.ErrFile.Error()) {
		t.Fatalf("sentinel text: %v", err)
	}

	_, err = _Run(t, `
def main():
    return file.read("`+dir+`")
`)
	if !errors.Is(err, file.ErrFile) || !errors.Is(err, file.ErrNotAFile) {
		t.Fatalf("directory: %v", err)
	}

	if os.Geteuid() == 0 {
		t.Skip("root can search a mode 000 directory")
	}
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(secret, "note")
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	_, err = _Run(t, `
def main():
    return file.exists("`+note+`")
`)
	if !errors.Is(err, os.ErrPermission) || !errors.Is(err, file.ErrFile) {
		t.Fatalf("permission: %v", err)
	}
}

// TestLark_MemoryFlag is the ceiling from the command line, run as a command.
//
// Built and executed rather than called, because the exit code is the thing
// being checked and go run reports its own rather than the program's.
//
// Absent and zero both mean the library's default, so both run. A negative is
// a usage error rather than quietly meaning unlimited, and so is a word.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func TestLark_MemoryFlag(t *testing.T) {
	root := _ModuleRoot(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "lark")
	build := exec.Command("go", "build", "-o", bin, "./cmd/lark")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	script := filepath.Join(dir, "hi.star")
	if err := os.WriteFile(script, []byte("def main():\n    print(\"hi\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, _ := _LarkCode(t, bin, "-s", script); code != 0 {
		t.Fatalf("no -m: exit %d", code)
	}
	if code, _ := _LarkCode(t, bin, "-s", script, "-m", "0"); code != 0 {
		t.Fatalf("-m 0: exit %d, want the default ceiling and a run", code)
	}
	if code, _ := _LarkCode(t, bin, "-s", script, "-m", "-5"); code != 2 {
		t.Fatalf("-m -5: exit %d, want 2", code)
	}
	if code, _ := _LarkCode(t, bin, "-s", script, "-m", "nope"); code != 2 {
		t.Fatalf("-m nope: exit %d, want 2", code)
	}
}

// _LarkCode runs the built binary and answers with the exit code it chose.
//
// Revisions:
//   - 2026-09-24 23:50: initial creation
func _LarkCode(t *testing.T, bin string, args ...string) (int, []byte) {
	t.Helper()

	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, out
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), out
	}

	t.Fatalf("lark: %v\n%s", err, out)
	return 0, out
}
