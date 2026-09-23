package observe_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/observe"
)

const (
	// SPEAKERS is how many threads the concurrent fixture prints from, and
	// LANE what a logged line puts in front of the text.
	BRANCHED = "thread_"

	// PRINTS is a script that prints from the spine and from two spawns.
	PRINTS = "testdata/prints.star"
)

// _Lines is a log file's lines.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func _Lines(t *testing.T, path string) []string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()

	var lines []string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	return lines
}

// TestWithLog_WritesTheFileItWasGiven is the naming rule now that nothing
// mints a name: two runs of one artifact are two files because the caller said
// so, and each holds only its own run's lines.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, as TestWithLogs_WritesOneFilePerRun,
//     where the rule was the directory, the run's id and .log
//   - 2026-09-23 23:28: the caller names the file
func TestWithLog_WritesTheFileItWasGiven(t *testing.T) {
	dir := t.TempDir()
	built := _Compile(t, PRINTS)

	for _, name := range []string{"first.log", "second.log"} {
		path := filepath.Join(dir, name)

		_, err := observe.Start(t.Context(), built, observe.WithLog(path)).Wait()
		if err != nil {
			t.Fatal(err)
		}

		lines := _Lines(t, path)
		if len(lines) != 3 {
			t.Fatalf("%s wrote %v, want three lines", name, lines)
		}
	}
}

// TestWithLog_PutsTheLaneInFrontOfTheLine is what the file buys over a
// printer: a concurrent script interleaves, and the transcript says which
// thread said what.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
//   - 2026-09-23 23:28: the caller names the file
func TestWithLog_PutsTheLaneInFrontOfTheLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")

	_, err := observe.Start(t.Context(), _Compile(t, PRINTS), observe.WithLog(path)).Wait()
	if err != nil {
		t.Fatal(err)
	}

	lanes := make(map[string]bool)

	for _, line := range _Lines(t, path) {
		lane, said, found := strings.Cut(line, "  ")
		if !found {
			t.Fatalf("no lane in front of %q", line)
		}

		if !strings.HasPrefix(lane, BRANCHED) {
			t.Fatalf("want a thread id, got %q", lane)
		}

		if said == "" {
			t.Fatalf("want the line after the lane, got %q", line)
		}

		lanes[lane] = true
	}

	if len(lanes) != 3 {
		t.Fatalf("want the spine and two spawns, got %v", lanes)
	}
}

// TestWithLog_ARunWhoseFileCannotBeOpenedFails records that a host asking for
// a transcript and not getting one hears about it, rather than the run going
// ahead with its output nowhere.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
//   - 2026-09-23 23:28: the caller names the file
func TestWithLog_ARunWhoseFileCannotBeOpenedFails(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// A file where the directory would have to be.
	path := filepath.Join(blocked, "run.log")

	run := observe.Start(t.Context(), _Compile(t, PRINTS), observe.WithLog(path))

	_, err := run.Wait()
	if err == nil {
		t.Fatal("want the run to fail")
	}

	if !strings.Contains(err.Error(), observe.SUFFIX) {
		t.Fatalf("want the path named, got %v", err)
	}

	if run.Status().GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want it reported failed, got %v", run.Status().GetStatus())
	}
}

// TestWithLog_IsOptional records that a run without one prints as it always
// did and writes nothing.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
//   - 2026-09-23 23:28: starts the run directly
func TestWithLog_IsOptional(t *testing.T) {
	dir := t.TempDir()

	_, err := observe.Start(
		t.Context(),
		_Compile(t, PRINTS),
		observe.WithPrinter(func(string) {}),
	).Wait()
	if err != nil {
		t.Fatal(err)
	}

	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(left) != 0 {
		t.Fatalf("want nothing written, got %v", left)
	}
}
