package tests

// This file tests the order of the held lines and the graph file. It drives
// Opened, Run, Running and the spool close directly. The same order, seen
// through the binary, is in hold_test.go (CLAUDE.md 1.8.1; C65a).
//
// Revisions:
//   - 2026-11-08 22:49: header states why the file is internal (C65a)
//   - 2026-11-10 01:37: moved to tests/ from hold_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

// _Probe is an io.Writer that notes, at its first write, whether the file at
// path holds anything, and keeps what it is given.
//
// Revisions:
//   - 2026-11-03 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: notes whether the file holds anything (C119, Q304)
type _Probe struct {
	path    string
	seen    bool
	existed bool
	got     bytes.Buffer
}

// Write notes the file at the first call and keeps p.
//
// Revisions:
//   - 2026-11-03 01:10: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: notes whether the file holds patches, the file being opened at
//     the start (C119, Q304)
func (p *_Probe) Write(b []byte) (int, error) {
	if !p.seen {
		p.seen = true

		info, err := os.Stat(p.path)
		p.existed = err == nil && info.Size() > 0
	}

	return p.got.Write(b)
}

// TestHold_GraphIsWrittenBeforeTheLines proves the order at the end of a held
// run, through the command's own pieces (Opened, Run, Running, and the
// close): the -graph file holds the patches when the first held line reaches
// out.
//
// Internal, because the output, the spool and the close are main's.
//
// Revisions:
//   - 2026-11-03 01:10: initial creation (C42)
//   - 2026-11-03 23:59: builds a _Run in place of a _Ready and a _Held (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the file holds patches, which the run hands to Watch (C119, Q304)
func TestHold_GraphIsWrittenBeforeTheLines(t *testing.T) {
	var (
		graph = filepath.Join(
			t.TempDir(),
			"end.jsonl",
		)
		asked = &command.Asked{
			Path:  "s.star",
			Noun:  command.SCRIPT,
			Graph: graph,
			Hold:  true,
		}
		probe = &_Probe{
			path: graph,
		}
		output, err = command.Opened(asked)
	)
	if err != nil {
		t.Fatal(err)
	}

	var (
		spool, _ = _Opening(
			t,
			true,
			true,
			probe,
		)
		run = &command.Run{
			Options: []common.RunOption{
				runtime.WithLog(spool),
				runtime.WithChanges(output.Watch()),
			},
			Output: output,
			Spool:  spool,
			Hosted: func() error {
				return nil
			},
		}
	)

	err = command.Running(
		context.Background(),
		asked,
		[]byte("def main():\n    print(\"held line\")\n"),
		run,
	)
	if err != nil {
		t.Fatal(err)
	}

	if probe.got.Len() != 0 {
		t.Fatalf(
			"out holds %q before the close",
			probe.got.String(),
		)
	}

	err = run.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !probe.existed || !strings.Contains(
		probe.got.String(),
		"held line",
	) {
		t.Fatalf(
			"graph held patches at the first line: %v; out %q",
			probe.existed,
			probe.got.String(),
		)
	}
}

// TestAbandoning_WritesTheHeldLines proves a memory abandon with held lines
// loses none: the held patches are written to the graph file, then the held
// lines go to standard output, then the process exits 5. A temporary spool is removed; a -logs
// spool (keep) stays, holding the lines. A child process runs Abandon.
//
// Internal, because the output, the held run and Abandon are main's.
//
// Revisions:
//   - 2026-11-03 01:10: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the graph file holds patches (C119, Q304)
func TestAbandoning_WritesTheHeldLines(t *testing.T) {
	const (
		CHILD    = "LARK_ABANDON_HOLD_CHILD"
		KEEP     = "LARK_ABANDON_HOLD_KEEP"
		SLEEPING = "def napping():\n    sleep(60000)\n\n" +
			"def main():\n    print(\"held line\")\n    napping()\n"
	)

	if dir := os.Getenv(CHILD); dir != "" {
		_HeldChild(
			dir,
			SLEEPING,
			os.Getenv(KEEP) == "1",
		)

		return
	}

	for _, keep := range []bool{false, true} {
		var (
			dir = t.TempDir()
			cmd = exec.Command(
				os.Args[0],
				"-test.run=^TestAbandoning_WritesTheHeldLines$",
			)
			stdout, stderr bytes.Buffer
			exit           *exec.ExitError
			flag           = "0"
		)

		if keep {
			flag = "1"
		}

		cmd.Env = append(
			os.Environ(),
			CHILD+"="+dir,
			KEEP+"="+flag,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if !errors.As(
			err,
			&exit,
		) || exit.ExitCode() != command.MEMORY {
			t.Fatalf(
				"keep %v: got %v, want exit 5; stderr %q",
				keep,
				err,
				stderr.String(),
			)
		}

		if !strings.Contains(
			stdout.String(),
			"held line",
		) ||
			stderr.String() != "lark: memory limit reached\n" {
			t.Fatalf(
				"keep %v: stdout %q stderr %q",
				keep,
				stdout.String(),
				stderr.String(),
			)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		want := 1
		if keep {
			want = 2
		}

		if len(entries) != want {
			t.Fatalf(
				"keep %v: directory holds %d files, want %d",
				keep,
				len(entries),
				want,
			)
		}

		if keep {
			raw, err := os.ReadFile(filepath.Join(
				dir,
				"spool.log",
			))
			if err != nil || !strings.Contains(
				string(raw),
				"held line",
			) {
				t.Fatalf(
					"the kept spool holds %q (%v)",
					raw,
					err,
				)
			}
		}
	}
}

// _HeldChild is the child of the test above. It holds the lines of a sleeping
// script in a spool in dir, removed at the close unless keep, waits until the
// script has printed and is asleep, and calls Abandon with the real exit.
//
// Revisions:
//   - 2026-11-03 01:10: initial creation (C42)
//   - 2026-11-03 23:59: builds a _Run in place of a _Ready and a _Held (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: hands the run the changes, which the graph file holds (C119, Q304)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func _HeldChild(
	dir string,
	script string,
	keep bool,
) {
	file, err := os.OpenFile(
		filepath.Join(
			dir,
			"spool.log",
		),
		os.O_CREATE|os.O_RDWR|os.O_TRUNC,
		command.LOG_FILE,
	)
	if err != nil {
		panic(err)
	}

	var (
		asked = &command.Asked{
			Path: "s.star",
			Noun: command.SCRIPT,
			Graph: filepath.Join(
				dir,
				"end.jsonl",
			),
			Hold:   true,
			Memory: true,
		}
		spool = command.NewSpool(
			file,
			true,
			!keep,
			os.Stdout,
		)
		output, oer = command.Opened(asked)
		run         = &command.Run{
			Output: output,
			Spool:  spool,
		}
	)
	if oer != nil {
		panic(oer)
	}

	run.Options = append(
		command.MustWatch(
			asked,
			run,
		),
		runtime.WithLog(spool),
		runtime.WithChanges(output.Watch()),
	)

	go func() {
		_ = command.Running(
			context.Background(),
			asked,
			[]byte(script),
			run,
		)
	}()

	for {
		started := run.Started.Load()
		if started != nil && len(started.Snapshot().GetFunctions()) == 2 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	command.Abandon(
		run,
		os.Stderr,
		os.Exit,
	)(memwatch.ErrLimit)
}
