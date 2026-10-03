package artifact_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// _Spine keeps what a run reports, and what it prints, one line of text each.
type _Spine struct {
	guard sync.Mutex
	lines []string
}

// Started keeps a start.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
func (s *_Spine) Started(thread string, line *scheduler.Line) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(s.lines, fmt.Sprintf("start %s %s %s", thread, line.Name, line.Builtin))
}

// Ended keeps an end.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
func (s *_Spine) Ended(thread string, name string, err error) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(s.lines, fmt.Sprintf("end %s %s %t", thread, name, err == nil))
}

// Write keeps a line the run printed, from the record a JSON handler writes:
// the thread, the function that printed it, and the line.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation, as Printed
//   - 2026-10-02 13:12: a line of the transcript, with the function that
//     printed it
//   - 2026-10-02 16:21: a record of the run's logger
func (s *_Spine) Write(p []byte) (int, error) {
	record := map[string]any{}

	err := json.Unmarshal(p, &record)
	if err != nil {
		return 0, err
	}

	line := fmt.Sprintf(
		"print %v %v %v",
		record[scheduler.ATTR_THREAD],
		record[scheduler.ATTR_FUNCTION],
		record[slog.MessageKey],
	)

	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(s.lines, line)

	return len(p), nil
}

// _Compiled compiles src as MAIN or ends the test.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
func _Compiled(t *testing.T, src string) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(&script.Source{Entry: MAIN, Text: []byte(src)})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	return built
}

// TestRun_AModuleLevelSpawnHasARunToJoin checks a module's top level runs
// inside the run: a spawn there starts a thread, and a join there waits for
// it.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
func TestRun_AModuleLevelSpawnHasARunToJoin(t *testing.T) {
	built := _Compiled(t, `
def work():
    return 1

handle = spawn(work)
GOT = join(handle)

def main():
    return GOT
`)

	value, err := artifact.Run(t.Context(), built)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != "[1]" {
		t.Fatalf("got %s, want [1]", value)
	}
}

// TestRun_AModuleLevelCallIsALineOfTheSpine checks the run begins before the
// top level runs: the spine starts first, a call there of a function the
// script defines is the spine's line, and what the top level prints is
// written on the spine's lane, all before main's own lines.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
//   - 2026-10-02 13:12: reads the print from the run's transcript
//   - 2026-10-02 16:21: reads the print from the run's logger
func TestRun_AModuleLevelCallIsALineOfTheSpine(t *testing.T) {
	built := _Compiled(t, `
def label():
    pass

label()
print("top")

def main():
    label()
`)

	into := new(_Spine)

	logger := slog.New(slog.NewJSONHandler(into, nil))

	_, err := artifact.Run(
		t.Context(),
		built,
		artifact.WithLogger(logger),
		artifact.Reporting(into),
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{
		"start thread_0 main ",
		"start thread_0 label ",
		"end thread_0 label true",
		"print thread_0 <toplevel> top",
		"start thread_0 label ",
		"end thread_0 label true",
		"end thread_0 main true",
	}

	if strings.Join(into.lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", into.lines, want)
	}
}
