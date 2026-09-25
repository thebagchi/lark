package remote_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/runtime/plugin/remote"
)

// _Directory is a plugins directory holding named, each an empty file.
//
// Empty because discovery reads a name and nothing else. What happens to a
// match that cannot run is Load's business, and testing/ proves it with real
// binaries.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func _Directory(t *testing.T, named ...string) string {
	t.Helper()

	dir := t.TempDir()

	for _, name := range named {
		err := os.WriteFile(filepath.Join(dir, name), nil, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// TestDiscovered_TakesThePrefixAndTheExtension is why a plugin is called
// lark-something.bin.
//
// The extension says what kind of artefact a file is and the prefix says whose
// it is, so a directory can hold binaries that have nothing to do with this
// runtime without any of them being started. lark.bin itself does not match,
// because the hyphen is required - a host finding and running itself would be
// a memorable afternoon.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func TestDiscovered_TakesThePrefixAndTheExtension(t *testing.T) {
	dir := _Directory(t,
		"lark-clock.bin",
		"lark-vault.bin",
		"lark.bin",
		"other.bin",
		"lark-notes.txt",
		"lark-shared.so",
	)

	got, err := remote.Discovered(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(dir, "lark-clock.bin"),
		filepath.Join(dir, "lark-vault.bin"),
	}

	if len(got) != len(want) {
		t.Fatalf("found %v, want %v", got, want)
	}

	// Sorted, so two plugins supplying one name report the same clash on every
	// run rather than whichever the filesystem offered first.
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("found %v, want %v", got, want)
		}
	}
}

// TestDiscovered_TakesAGlobFromTheHost keeps the default from being the only
// answer, since a host may have its own naming.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func TestDiscovered_TakesAGlobFromTheHost(t *testing.T) {
	dir := _Directory(t, "lark-clock.bin", "mine-clock.bin")

	got, err := remote.Discovered(dir, "mine-*.bin")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 || filepath.Base(got[0]) != "mine-clock.bin" {
		t.Fatalf("found %v, want only mine-clock.bin", got)
	}
}

// TestDiscovered_AnEmptyOrMissingDirectoryIsNotAFailure records that nothing to
// load is a legal answer.
//
// A host pointed at a directory that is not there has nothing to start, which
// is the same situation as a directory holding nothing. Refusing would make
// "no plugins" an error for a runtime whose plugins are all optional.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func TestDiscovered_AnEmptyOrMissingDirectoryIsNotAFailure(t *testing.T) {
	cases := map[string]string{
		"empty":   _Directory(t),
		"missing": filepath.Join(t.TempDir(), "nope"),
	}

	for name, dir := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := remote.Discovered(dir, "")
			if err != nil {
				t.Fatalf("got %v, want no error", err)
			}

			if len(got) != 0 {
				t.Fatalf("found %v, want nothing", got)
			}
		})
	}
}

// TestLoad_AMatchThatWillNotRunIsSkipped is the case go-plugin's own Discover
// warns about: the glob reads a name, not the execute bit.
//
// One plugin that cannot start does not stop the others and does not fail the
// host. It comes back in Skipped, naming the file, for a host to say what it
// likes about.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func TestLoad_AMatchThatWillNotRunIsSkipped(t *testing.T) {
	listener, _ := _Listening(t)

	// Matches the glob, is not a program.
	dir := _Directory(t, "lark-broken.bin")

	loading, err := listener.Load(dir, "")
	if err != nil {
		t.Fatalf("loading a directory of one broken plugin: %v", err)
	}

	if len(loading.Loaded) != 0 {
		t.Fatalf("loaded %v, want none", loading.Loaded)
	}

	if len(loading.Skipped) != 1 {
		t.Fatalf("skipped %v, want one", loading.Skipped)
	}

	if !strings.Contains(loading.Skipped[0].Error(), "lark-broken.bin") {
		t.Fatalf("the refusal does not name the file: %v", loading.Skipped[0])
	}
}

// TestLoad_NothingToLoadIsNotAFailure keeps an empty plugins directory from
// stopping a host that does not need one.
//
// Revisions:
//   - 2026-09-26 00:18: initial creation
func TestLoad_NothingToLoadIsNotAFailure(t *testing.T) {
	listener, _ := _Listening(t)

	loading, err := listener.Load(_Directory(t), "")
	if err != nil {
		t.Fatalf("got %v, want no error", err)
	}

	if len(loading.Loaded) != 0 || len(loading.Skipped) != 0 {
		t.Fatalf("loaded %v and skipped %v, want neither", loading.Loaded, loading.Skipped)
	}

	// And a name nothing supplied is still undefined, rather than present and
	// broken.
	_, err = _Ran(t, listener, "def main():\n    return clock.now()\n")
	if err == nil {
		t.Fatal("clock resolved with no plugin loaded")
	}

	if errors.Is(err, remote.ErrGone) {
		t.Fatalf("a name nothing supplied reported as a plugin that left: %v", err)
	}
}
