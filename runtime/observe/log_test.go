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

// TestWithLogs_WritesOneFilePerRun is the naming rule a host relies on, since
// a snapshot carries no path: the directory it named, the run's id, .log.
//
// Two runs of one artifact are two files, which is why the id names them
// rather than the script.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func TestWithLogs_WritesOneFilePerRun(t *testing.T) {
	dir := t.TempDir()
	built := _Compile(t, PRINTS)

	store := observe.New()

	first := store.Start(t.Context(), built, observe.WithLogs(dir))

	if _, err := store.Wait(t.Context(), first); err != nil {
		t.Fatal(err)
	}

	second := store.Start(t.Context(), built, observe.WithLogs(dir))

	if _, err := store.Wait(t.Context(), second); err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("want two runs")
	}

	for _, id := range []string{first, second} {
		lines := _Lines(t, filepath.Join(dir, id+observe.SUFFIX))

		if len(lines) != 3 {
			t.Fatalf("%s wrote %v, want three lines", id, lines)
		}
	}
}

// TestWithLogs_PutsTheLaneInFrontOfTheLine is what the file buys over a
// printer: a concurrent script interleaves, and the transcript says which
// thread said what.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func TestWithLogs_PutsTheLaneInFrontOfTheLine(t *testing.T) {
	dir := t.TempDir()

	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, PRINTS), observe.WithLogs(dir))

	if _, err := store.Wait(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	lanes := make(map[string]bool)

	for _, line := range _Lines(t, filepath.Join(dir, id+observe.SUFFIX)) {
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

// TestWithLogs_ARunWhoseFileCannotBeOpenedFails records that a host asking for
// a transcript and not getting one hears about it, rather than the run going
// ahead with its output nowhere.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func TestWithLogs_ARunWhoseFileCannotBeOpenedFails(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	store := observe.New()

	// A file where the directory would have to be.
	id := store.Start(t.Context(), _Compile(t, PRINTS), observe.WithLogs(blocked))

	_, err := store.Wait(t.Context(), id)
	if err == nil {
		t.Fatal("want the run to fail")
	}

	if !strings.Contains(err.Error(), observe.SUFFIX) {
		t.Fatalf("want the path named, got %v", err)
	}

	snap, err := store.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	if snap.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want it reported failed, got %v", snap.GetStatus())
	}
}

// TestWithLogs_IsOptional records that a run without one prints as it always
// did and writes nothing.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func TestWithLogs_IsOptional(t *testing.T) {
	dir := t.TempDir()

	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, PRINTS), observe.WithPrinter(func(string) {}))

	if _, err := store.Wait(t.Context(), id); err != nil {
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
