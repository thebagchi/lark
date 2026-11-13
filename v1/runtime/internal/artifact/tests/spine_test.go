package tests

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
)

var (
	// SPOKEN reads a log line WithLog writes: the thread, the function that
	// printed, and the message, which is quoted when it has a space, and absent
	// when the line printed is empty.
	//
	// Revisions:
	//   - 2026-11-10 17:15: initial creation (C121, Q340)
	SPOKEN = regexp.MustCompile(`thread=(\S+) function=(\S+)(?: msg=("(?:[^"\\]|\\.)*"|\S+))?`)
)

// _Spine keeps what a run reports, and what it prints, one line of text each.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Spine struct {
	guard sync.Mutex
	lines []string
}

// Started keeps a start.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Spine) Started(
	thread string,
	line *common.Line,
) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(
		s.lines,
		fmt.Sprintf(
			"start %s %s %s",
			thread,
			line.Name,
			line.Builtin,
		),
	)
}

// Ended keeps an end.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Spine) Ended(
	thread string,
	name string,
	err error,
) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(
		s.lines,
		fmt.Sprintf(
			"end %s %s %t",
			thread,
			name,
			err == nil,
		),
	)
}

// Write keeps a line the run printed, from the log line WithLog writes: the
// thread, the function that printed it, and the line.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation, as Printed
//   - 2026-10-02 13:12: a line of the transcript, with the function that
//     printed it
//   - 2026-10-02 16:21: a record of the run's logger
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: reads a log line WithLog writes, not a JSON record (C121, Q340)
func (s *_Spine) Write(p []byte) (int, error) {
	found := SPOKEN.FindSubmatch(p)
	if found == nil {
		return 0, fmt.Errorf(
			"not a log line: %q",
			p,
		)
	}

	said := string(found[3])
	if strings.HasPrefix(
		said,
		`"`,
	) {
		unquoted, err := strconv.Unquote(said)
		if err != nil {
			return 0, err
		}

		said = unquoted
	}

	line := fmt.Sprintf(
		"print %s %s %s",
		found[1],
		found[2],
		said,
	)

	s.guard.Lock()
	defer s.guard.Unlock()

	s.lines = append(
		s.lines,
		line,
	)

	return len(p), nil
}

// _Compiled compiles src as MAIN or ends the test.
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Compiled(
	t *testing.T,
	src string,
) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(&common.Source{
		Entry: MAIN,
		Text:  []byte(src),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	return built
}

// TestRun_AModuleLevelSpawnIsRefused checks a module's top level runs inside
// the run, so a spawn there reaches the spawn builtin, which refuses it with
// core.ErrInit: a thread started there would read globals the top level is
// still writing (Q383).
//
// Revisions:
//   - 2026-10-02 01:28: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-11 22:55: renamed from TestRun_AModuleLevelSpawnHasARunToJoin; asserts the
//     refusal, where it asserted the join; the doc names core.ErrInit (C127, B274, Q383)
func TestRun_AModuleLevelSpawnIsRefused(t *testing.T) {
	var built = _Compiled(
		t,
		`
def work():
    return 1

handle = spawn(work)
GOT = join(handle)

def main():
    return GOT
`,
	)

	_, err := artifact.Run(
		t.Context(),
		built,
	)
	if !errors.Is(
		err,
		core.ErrInit,
	) {
		t.Fatalf(
			"run: %v, want core.ErrInit",
			err,
		)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: gives the spine to WithLog, WithLogger being dropped (C121, Q340)
func TestRun_AModuleLevelCallIsALineOfTheSpine(t *testing.T) {
	var (
		built = _Compiled(
			t,
			`
def label():
    pass

label()
print("top")

def main():
    label()
`,
		)
		into = new(_Spine)
	)

	_, err := artifact.Run(
		t.Context(),
		built,
		artifact.WithLog(into),
		artifact.Reporting(into),
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
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

	if strings.Join(
		into.lines,
		"\n",
	) != strings.Join(
		want,
		"\n",
	) {
		t.Fatalf(
			"got %q, want %q",
			into.lines,
			want,
		)
	}
}
