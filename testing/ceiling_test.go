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

	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/scheduler"
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
func TestCeiling_AHostChoosesWhatARunMayHold(t *testing.T) {
	src := _Reading(t)

	built, err := runtime.NewCompiler().Compile("ceiling.star", src)
	if err != nil {
		t.Fatal(err)
	}

	_, err = built.Run(scheduler.Allowing(context.Background(), _NARROW))
	if !errors.Is(err, scheduler.ErrMemory) {
		t.Fatalf("a host's ceiling did not reach the run: %v", err)
	}

	t.Logf("refused with: %v", err)

	// And the same artifact under the default ceiling still reads the file, so
	// the refusal was the ceiling rather than the file.
	got, err := built.Run(context.Background())
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

	built, err := runtime.NewCompiler().Compile("repeat.star", src)
	if err != nil {
		t.Fatal(err)
	}

	// Room for a few copies at once, nowhere near a hundred.
	got, err := built.Run(scheduler.Allowing(context.Background(), 8*_BLOB))
	if err != nil {
		t.Fatalf("a hundred reads of one file needed more than eight of it: %v", err)
	}

	if got.String() != fmt.Sprint(100*_BLOB) {
		t.Fatalf("read %s bytes in total, want %d", got, 100*_BLOB)
	}
}
