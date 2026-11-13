package tests

// This file tests the log lines of the command: Logging writes the full line
// everywhere, on a terminal too (Q342), and keeps the same line in the -logs
// file. A test cannot open a real terminal, and the command has no check for
// one.
//
// Revisions:
//   - 2026-11-10 01:37: moved to tests/ from plain_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)
//   - 2026-11-10 17:26: renamed from plain_test.go, with the header for the full lines; the
//     tests of the terminal check and of Plain are dropped with them (C121, Q342)

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	PRINTS_HELLO = "def main():\n    print(\"hello\")\n    print(\"\")\n"
)

var (
	// FULL_LINES is what PRINTS_HELLO prints, as the command writes it: the
	// time, the level, the thread, the function, and the message last.
	//
	// Revisions:
	//   - 2026-11-10 17:26: initial creation (C121, Q342)
	FULL_LINES = regexp.MustCompile(
		`^time=\S+ level=INFO thread=thread_0 function=main msg=hello\n` +
			`time=\S+ level=INFO thread=thread_0 function=main\n$`,
	)
)

// _Printed runs PRINTS_HELLO through the options Logging chose for asked, with
// standard output a file, and returns what standard output received.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C72)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:26: takes no terminal answer, Logging having no terminal check (C121, Q342)
func _Printed(
	t *testing.T,
	asked *command.Asked,
) string {
	t.Helper()

	out, err := os.Create(filepath.Join(
		t.TempDir(),
		"stdout",
	))
	if err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	os.Stdout = out

	t.Cleanup(func() {
		os.Stdout = stdout
	})

	option, spool, err := command.Logging(
		asked,
		func() {
			t.Log("stopped")
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Run(
		context.Background(),
		&common.Source{
			Entry: "s.star",
			Text:  []byte(PRINTS_HELLO),
		},
		option,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = spool.Close()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

// TestLogging_FullLines proves Q342: every case prints the full line to
// standard output, with the time, the level, the thread and the function, and
// the message last. A line printed empty has no message.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C72)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:26: renamed from TestLogging_PlainOnATerminal; the cases no longer have a
//     terminal, since the line is the same on it (C121, Q342)
func TestLogging_FullLines(t *testing.T) {
	for _, tc := range []struct {
		name string
		logs bool
		hold bool
	}{
		{
			"no flag",
			false,
			false,
		},
		{
			"-logs",
			true,
			false,
		},
		{
			"-hold",
			false,
			true,
		},
		{
			"-logs and -hold",
			true,
			true,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				asked := &command.Asked{
					Path: "s.star",
					Noun: command.SCRIPT,
					Hold: tc.hold,
				}
				if tc.logs {
					asked.Logs = t.TempDir()
				}

				got := _Printed(
					t,
					asked,
				)

				if !FULL_LINES.MatchString(got) {
					t.Fatalf(
						"got %q, want two full lines",
						got,
					)
				}
			},
		)
	}
}

// TestLogging_LogsFileStaysStructured proves -logs keeps the structured form in
// the file.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C72)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:26: Logging takes no terminal answer, so the case has none (C121, Q342)
func TestLogging_LogsFileStaysStructured(t *testing.T) {
	dir := t.TempDir()

	_Printed(
		t,
		&command.Asked{
			Path: "s.star",
			Noun: command.SCRIPT,
			Logs: dir,
		},
	)

	raw, err := os.ReadFile(filepath.Join(
		dir,
		"s.star.log",
	))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(raw),
		"thread=",
	) {
		t.Fatalf(
			"log file: %q",
			raw,
		)
	}
}
