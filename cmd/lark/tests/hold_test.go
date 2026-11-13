//go:build unix

package tests

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	// NAPPING is the sleep in the scripts that nap, in milliseconds. Tests
	// wait for events, never for a delay; this only keeps the script alive
	// while they look.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NAPPING = "3000"

	// HOLDING is the first words of what -hold says on standard error.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	HOLDING = "lark: lines are held until the run ends: "

	// STOPPED_CODE and FAILED_CODE are the exit codes of a stopped run and a
	// failed script, which the command keeps to itself.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	STOPPED_CODE = 4
	FAILED_CODE  = 1

	// PRINTS_THEN_NAPS prints a line, sleeps NAPPING milliseconds and prints
	// another, so a test can look between them.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PRINTS_THEN_NAPS = "def main():\n    print(\"first\")\n    sleep(" +
		NAPPING + ")\n    print(\"second\")\n"

	// PRINTS_THEN_NAPS_IN and PRINTS_THEN_SLEEPS_IN do the same inside a function
	// called napping. Each is a format for the path of a marker file, which the
	// script writes just before it calls napping: that is the event a test
	// waits for, the first line having been printed before it. A marker, since
	// under -hold a -graph file is written at the end and cannot tell.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-10 11:45: the event is a marker file the script writes, not a function
	//     in the changes file (C119, Q304)
	PRINTS_THEN_NAPS_IN = "def napping():\n    sleep(" + NAPPING + ")\n\n" +
		"def main():\n    print(\"first\")\n    file.write(%q, \"x\")\n" +
		"    napping()\n    print(\"second\")\n"
	PRINTS_THEN_SLEEPS_IN = "def napping():\n    sleep(60000)\n\n" +
		"def main():\n    print(\"before\")\n    file.write(%q, \"x\")\n    napping()\n"
)

var (
	// STARTED maps a started command to the channel that closes when its
	// standard output has been read to the end, which Wait must follow.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	STARTED = map[*exec.Cmd]chan struct{}{}

	// STAMP is the time field of a line, which differs between two runs.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	STAMP = regexp.MustCompile(`time=\S+ `)
)

// _Started starts lark with args and returns the command, with standard output
// collected as it arrives (read it with the returned function) and standard
// error as the first line, read at once, and the rest read by _Ended.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Started(
	t *testing.T,
	args ...string,
) (*exec.Cmd, func() string, *bufio.Reader) {
	t.Helper()

	var (
		ctx, cancel = context.WithTimeout(
			t.Context(),
			RUN_LIMIT,
		)
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			args...,
		)
		lock sync.Mutex
		got  bytes.Buffer
	)

	t.Cleanup(cancel)

	_Grouped(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		buf := make(
			[]byte,
			4096,
		)

		for {
			n, err := stdout.Read(buf)

			lock.Lock()
			got.Write(buf[:n])
			lock.Unlock()

			if err != nil {
				return
			}
		}
	}()

	STARTED[cmd] = done

	return cmd, func() string {
		lock.Lock()
		defer lock.Unlock()

		return got.String()
	}, bufio.NewReader(stderr)
}

// _Ended waits for cmd and returns its exit code and the rest of standard
// error. stdout must have been read to its end first: the caller waits for the
// collector, so it does this by reading until the run is over.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ended(
	t *testing.T,
	cmd *exec.Cmd,
	stderr *bufio.Reader,
) (int, string) {
	t.Helper()

	rest, err := io.ReadAll(stderr)
	if err != nil {
		t.Fatal(err)
	}

	<-STARTED[cmd]

	err = cmd.Wait()

	var exit *exec.ExitError
	if errors.As(
		err,
		&exit,
	) {
		return exit.ExitCode(), string(rest)
	}

	if err != nil {
		t.Fatal(err)
	}

	return 0, string(rest)
}

// _Captured runs lark with args to its end and returns the exit code, standard
// output and standard error.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Captured(
	t *testing.T,
	args ...string,
) (int, string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		t.Context(),
		RUN_LIMIT,
	)

	defer cancel()

	var (
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			args...,
		)
		stdout, err bytes.Buffer
		exit        *exec.ExitError
		code        int
	)

	cmd.Stdout = &stdout
	cmd.Stderr = &err

	_Grouped(cmd)

	run := cmd.Run()
	if errors.As(
		run,
		&exit,
	) {
		code = exit.ExitCode()
	} else if run != nil {
		t.Fatal(run)
	}

	return code, stdout.String(), err.String()
}

// _Scripted writes source to a file in a new directory and returns its path.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Scripted(
	t *testing.T,
	source string,
) string {
	t.Helper()

	var (
		path = filepath.Join(
			t.TempDir(),
			"held.star",
		)
		err = os.WriteFile(
			path,
			[]byte(source),
			SHARED,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// _Named is the file the first line of standard error names, or fails.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Named(
	t *testing.T,
	stderr *bufio.Reader,
) string {
	t.Helper()

	line, err := stderr.ReadString('\n')
	if err != nil || !strings.HasPrefix(
		line,
		HOLDING,
	) {
		t.Fatalf(
			"first line of standard error %q (%v), want %q and a name",
			line,
			err,
			HOLDING,
		)
	}

	return strings.TrimSpace(strings.TrimPrefix(
		line,
		HOLDING,
	))
}

// TestHold_StreamsByDefault proves that with no -hold the first line reaches
// standard output while the script sleeps, as it did before the flag existed
// (decision Q43), and nothing is said on standard error.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_StreamsByDefault(t *testing.T) {
	cmd, stdout, stderr := _Started(
		t,
		"run",
		_Scripted(
			t,
			PRINTS_THEN_NAPS,
		),
	)

	// Wait for the first line, however long the start takes, then look: the
	// second is NAPPING milliseconds behind it.
	for !strings.Contains(
		stdout(),
		"msg=first",
	) {
		select {
		case <-t.Context().Done():
			t.Fatal("no first line")
		case <-time.After(5 * time.Millisecond):
		}
	}

	if strings.Contains(
		stdout(),
		"msg=second",
	) {
		t.Fatalf(
			"standard output is %q, want the first line only",
			stdout(),
		)
	}

	code, rest := _Ended(
		t,
		cmd,
		stderr,
	)
	if code != 0 || rest != "" {
		t.Fatalf(
			"exit %d stderr %q, want 0 and nothing",
			code,
			rest,
		)
	}
}

// TestHold_HoldsUntilTheEnd proves -hold says nothing on standard output while
// the script sleeps, names a temporary file on standard error first, and at the
// end prints both lines and removes the file.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: waits for a marker file, not for the changes file (C119, Q304)
func TestHold_HoldsUntilTheEnd(t *testing.T) {
	var (
		marker = filepath.Join(
			t.TempDir(),
			"asleep",
		)
		cmd, stdout, stderr = _Started(
			t,
			"run",
			"-hold",
			_Scripted(
				t,
				fmt.Sprintf(
					PRINTS_THEN_NAPS_IN,
					marker,
				),
			),
		)
		name = _Named(
			t,
			stderr,
		)
	)

	if filepath.Dir(name) != os.TempDir() {
		t.Fatalf(
			"file %q is not in %q",
			name,
			os.TempDir(),
		)
	}

	_, err := os.Stat(name)
	if err != nil {
		t.Fatalf(
			"the named file is not there during the run: %v",
			err,
		)
	}

	_Asleep(
		t,
		marker,
	)

	if stdout() != "" {
		t.Fatalf(
			"with the script asleep standard output is %q, want nothing",
			stdout(),
		)
	}

	code, rest := _Ended(
		t,
		cmd,
		stderr,
	)
	if code != 0 || rest != "" {
		t.Fatalf(
			"exit %d stderr %q, want 0 and nothing more",
			code,
			rest,
		)
	}

	first := strings.Index(
		stdout(),
		"msg=first",
	)
	if first < 0 || strings.Index(
		stdout(),
		"msg=second",
	) < first {
		t.Fatalf(
			"standard output %q, want first then second",
			stdout(),
		)
	}

	_, err = os.Stat(name)
	if !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"the temporary file is left: %v",
			err,
		)
	}
}

// TestHold_SameLinesAsStream proves the held lines are the streamed lines: the
// same count, order and text once the time field is taken out, for a script on
// one thread.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_SameLinesAsStream(t *testing.T) {
	path := _Scripted(
		t,
		"def main():\n    for i in range(3000):\n        print(\"line\", i)\n",
	)

	var (
		_, streamed, _ = _Captured(
			t,
			"run",
			path,
		)
		code, held, rest = _Captured(
			t,
			"run",
			"-hold",
			path,
		)
	)

	if code != 0 || !strings.HasPrefix(
		rest,
		HOLDING,
	) {
		t.Fatalf(
			"exit %d stderr %q",
			code,
			rest,
		)
	}

	if strings.Count(
		streamed,
		"\n",
	) != 3000 {
		t.Fatalf(
			"streamed %d lines, want 3000",
			strings.Count(
				streamed,
				"\n",
			),
		)
	}

	if STAMP.ReplaceAllString(
		held,
		"",
	) != STAMP.ReplaceAllString(
		streamed,
		"",
	) {
		t.Fatal("held lines differ from streamed lines")
	}
}

// TestHold_LogsFile proves -logs with -hold: the file is the spool, holds every
// line, standard output at the end is the file, and no temporary file is named.
// Without -hold the file is the same as standard output too, behind its buffer.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_LogsFile(t *testing.T) {
	path := _Scripted(
		t,
		"def main():\n    for i in range(3000):\n        print(\"line\", i)\n",
	)

	for _, hold := range []bool{true, false} {
		var (
			dir = filepath.Join(
				t.TempDir(),
				"logs",
			)
			args = []string{"run", "-logs", dir}
		)

		if hold {
			args = append(
				args,
				"-hold",
			)
		}

		args = append(
			args,
			path,
		)

		code, out, rest := _Captured(
			t,
			args...,
		)
		if code != 0 || rest != "" {
			t.Fatalf(
				"hold %v: exit %d stderr %q",
				hold,
				code,
				rest,
			)
		}

		raw, err := os.ReadFile(filepath.Join(
			dir,
			"held.star.log",
		))
		if err != nil {
			t.Fatal(err)
		}

		if string(raw) != out || strings.Count(
			out,
			"\n",
		) != 3000 {
			t.Fatalf(
				"hold %v: file %d bytes, standard output %d, want equal",
				hold,
				len(raw),
				len(out),
			)
		}
	}
}

// TestHold_GraphAndLines proves -graph with -hold writes the patches to the
// file and prints the lines, and the run exits 0. The first patch makes the run
// running, and the last gives it its end status.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the file holds patches (C119, Q304)
func TestHold_GraphAndLines(t *testing.T) {
	var (
		graph = filepath.Join(
			t.TempDir(),
			"end.jsonl",
		)
		code, out, rest = _Captured(
			t,
			"run",
			"-hold",
			"-graph",
			graph,
			_Scripted(
				t,
				"def main():\n    print(\"x\")\n",
			),
		)
	)

	if code != 0 || !strings.HasPrefix(
		rest,
		HOLDING,
	) || !strings.Contains(
		out,
		"msg=x",
	) {
		t.Fatalf(
			"exit %d stdout %q stderr %q",
			code,
			out,
			rest,
		)
	}

	lines := _Lines(
		t,
		graph,
	)
	if len(lines) < 2 ||
		lines[0] != RUNNING_LINE ||
		lines[len(lines)-1] != SUCCEEDED_LINE {
		t.Fatalf(
			"graph file holds %q, want patches from running to succeeded",
			lines,
		)
	}
}

// TestHold_GraphIsWrittenAtTheEnd proves what -hold changes for -graph (Q304):
// while the script sleeps the file is there and empty, and the patches are in it
// once the run is over, in the order a stream would have written them. The same
// run without -hold has patches in the file while the script sleeps.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q304)
func TestHold_GraphIsWrittenAtTheEnd(t *testing.T) {
	var (
		streamed = ""
		held     = ""
	)

	for _, hold := range []bool{false, true} {
		var (
			dir    = t.TempDir()
			marker = filepath.Join(
				dir,
				"asleep",
			)
			graph = filepath.Join(
				dir,
				"g.jsonl",
			)
			args = []string{"run", "-graph", graph}
		)

		if hold {
			args = append(
				args,
				"-hold",
			)
		}

		cmd, _, stderr := _Started(
			t,
			append(
				args,
				_Scripted(
					t,
					fmt.Sprintf(
						PRINTS_THEN_NAPS_IN,
						marker,
					),
				),
			)...,
		)

		_Asleep(
			t,
			marker,
		)

		raw, err := os.ReadFile(graph)
		if err != nil {
			t.Fatal(err)
		}

		if (len(raw) == 0) != hold {
			t.Fatalf(
				"hold %v: the file holds %d bytes while the script sleeps",
				hold,
				len(raw),
			)
		}

		code, rest := _Ended(
			t,
			cmd,
			stderr,
		)
		if code != 0 {
			t.Fatalf(
				"hold %v: exit %d stderr %q",
				hold,
				code,
				rest,
			)
		}

		raw, err = os.ReadFile(graph)
		if err != nil {
			t.Fatal(err)
		}

		if hold {
			held = string(raw)
		} else {
			streamed = string(raw)
		}
	}

	if held == "" || held != streamed {
		t.Fatalf(
			"held patches\n%s\nstreamed patches\n%s",
			held,
			streamed,
		)
	}
}

// TestHold_StoppedPrintsWhatWasHeld proves SIGINT, SIGTERM and SIGHUP alike,
// during a held run, exit 4, print the line printed before them, and remove the
// temporary file (Q299, B256). SIGHUP used to end the process with 129 and leave
// the spool in the temporary directory.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: SIGINT and SIGHUP join SIGTERM; waits for a marker file
//     (C119, B256, Q299)
func TestHold_StoppedPrintsWhatWasHeld(t *testing.T) {
	for _, item := range []struct {
		name   string
		signal syscall.Signal
	}{
		{
			"SIGTERM",
			syscall.SIGTERM,
		},
		{
			"SIGINT",
			syscall.SIGINT,
		},
		{
			"SIGHUP",
			syscall.SIGHUP,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					marker = filepath.Join(
						t.TempDir(),
						"asleep",
					)
					cmd, stdout, stderr = _Started(
						t,
						"run",
						"-hold",
						_Scripted(
							t,
							fmt.Sprintf(
								PRINTS_THEN_SLEEPS_IN,
								marker,
							),
						),
					)
					name = _Named(
						t,
						stderr,
					)
				)

				_Asleep(
					t,
					marker,
				)

				err := syscall.Kill(
					cmd.Process.Pid,
					item.signal,
				)
				if err != nil {
					t.Fatal(err)
				}

				code, rest := _Ended(
					t,
					cmd,
					stderr,
				)
				if code != STOPPED_CODE {
					t.Fatalf(
						"exit %d stderr %q, want %d",
						code,
						rest,
						STOPPED_CODE,
					)
				}

				if !strings.Contains(
					stdout(),
					"msg=before",
				) {
					t.Fatalf(
						"standard output %q, want the held line",
						stdout(),
					)
				}

				_, err = os.Stat(name)
				if !errors.Is(
					err,
					os.ErrNotExist,
				) {
					t.Fatalf(
						"the temporary file is left: %v",
						err,
					)
				}
			},
		)
	}
}

// TestHold_HangUpIgnoredStaysIgnored proves a run started with SIGHUP ignored,
// as nohup starts one, is not stopped by it: the script goes on to its end and
// the exit is 0. os/signal would un-ignore the signal if the command asked to
// catch it. The shell sets the disposition, which the exec keeps.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B256, Q299)
func TestHold_HangUpIgnoredStaysIgnored(t *testing.T) {
	var (
		marker = filepath.Join(
			t.TempDir(),
			"asleep",
		)
		script = _Scripted(
			t,
			fmt.Sprintf(
				PRINTS_THEN_NAPS_IN,
				marker,
			),
		)
		ctx, cancel = context.WithTimeout(
			t.Context(),
			RUN_LIMIT,
		)
		cmd = exec.CommandContext(
			ctx,
			"/bin/sh",
			"-c",
			`trap '' HUP; exec "$0" "$@"`,
			BUILT,
			"run",
			script,
		)
	)

	defer cancel()

	_Grouped(cmd)

	var stdout bytes.Buffer

	cmd.Stdout = &stdout

	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	_Asleep(
		t,
		marker,
	)

	err = syscall.Kill(
		cmd.Process.Pid,
		syscall.SIGHUP,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Wait()
	if err != nil || !strings.Contains(
		stdout.String(),
		"msg=second",
	) {
		t.Fatalf(
			"wait %v, standard output %q, want the run to end well",
			err,
			stdout.String(),
		)
	}
}

// TestHold_FailedScriptPrintsWhatWasHeld proves a script that prints and then
// fails exits 1, says it failed on standard error, and still prints its line.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_FailedScriptPrintsWhatWasHeld(t *testing.T) {
	code, out, rest := _Captured(
		t,
		"run",
		"-hold",
		_Scripted(
			t,
			"def main():\n    print(\"before\")\n    fail(\"boom\")\n",
		),
	)

	if code != FAILED_CODE || !strings.Contains(
		out,
		"msg=before",
	) || !strings.Contains(
		rest,
		"script failed",
	) {
		t.Fatalf(
			"exit %d stdout %q stderr %q",
			code,
			out,
			rest,
		)
	}
}

// TestHold_NotWithConvert proves -hold beside convert is refused with exit 2,
// as a flag that verb does not have (Q305), and leaves no file behind.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: renamed from TestHold_NotWithTranslate; by verb (C119, Q305)
func TestHold_NotWithConvert(t *testing.T) {
	code, _, rest := _Captured(
		t,
		"convert",
		"-hold",
		_Scripted(
			t,
			"def main():\n    pass\n",
		),
	)

	if code != EXIT_FLAGS || !strings.Contains(
		rest,
		"-hold",
	) {
		t.Fatalf(
			"exit %d stderr %q, want %d and a mention of -hold",
			code,
			rest,
			EXIT_FLAGS,
		)
	}
}

// TestHold_HugeLogs proves 200,000 held lines all arrive. The bound on memory
// is proved where it is cheap to measure, in TestSpool_MemoryIsBounded.
//
// Revisions:
//   - 2026-11-03 01:00: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_HugeLogs(t *testing.T) {
	const (
		LINES = 200000
	)

	code, out, rest := _Captured(
		t,
		"run",
		"-hold",
		_Scripted(
			t,
			"def main():\n    for i in range(200000):\n"+
				"        print(\"a line of some length to fill it\", i)\n",
		),
	)

	if code != 0 || !strings.HasPrefix(
		rest,
		HOLDING,
	) || strings.Count(
		out,
		"\n",
	) != LINES {
		t.Fatalf(
			"exit %d, %d lines, stderr %q, want %d lines",
			code,
			strings.Count(
				out,
				"\n",
			),
			rest,
			LINES,
		)
	}
}

// TestHold_StartUpExitsLeaveNoSpool proves a start-up that ends before the run
// leaves no temporary spool: a graph path that cannot be made (exit 3), plugins
// that cannot be loaded beside a -graph that is held (3), and arguments that
// are not JSON (2). TMPDIR is a new directory, which must be empty afterwards.
//
// Revisions:
//   - 2026-11-03 21:50: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the changes case becomes a held -graph with plugins that fail
//     (C119, Q304)
func TestHold_StartUpExitsLeaveNoSpool(t *testing.T) {
	var (
		script = _Scripted(
			t,
			"def main():\n    print(\"x\")\n",
		)
		missing = filepath.Join(
			t.TempDir(),
			"no",
			"such",
			"x",
		)
	)

	cases := []struct {
		name string
		args []string
		code int
	}{
		{
			"graph",
			[]string{"-graph", missing},
			3,
		},
		{
			"held graph, plugins that fail",
			[]string{"-graph", filepath.Join(
				t.TempDir(),
				"g.jsonl",
			), "-plugins", UNPATTERNED},
			3,
		},
		{
			"args",
			[]string{"-args", "{bad"},
			2,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				tmp := t.TempDir()

				t.Setenv(
					"TMPDIR",
					tmp,
				)

				code, _, rest := _Captured(
					t,
					append(
						[]string{"run", "-hold"},
						append(
							tc.args,
							script,
						)...,
					)...,
				)
				if code != tc.code {
					t.Fatalf(
						"exit %d stderr %q, want %d",
						code,
						rest,
						tc.code,
					)
				}

				entries, err := os.ReadDir(tmp)
				if err != nil || len(entries) != 0 {
					t.Fatalf(
						"TMPDIR holds %d entries (%v), want none",
						len(entries),
						err,
					)
				}
			},
		)
	}
}

// TestHold_ClosedStandardOutput proves a reader that closes the pipe early
// ends the run in order, exit 4 and not a death by SIGPIPE: with -hold the
// temporary spool is removed, and with -logs the file holds whole lines to the
// last one written, each line in it complete. The script prints 200,000 lines,
// so the run is still going when the reader leaves.
//
// Revisions:
//   - 2026-11-03 21:50: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestHold_ClosedStandardOutput(t *testing.T) {
	cases := []struct {
		name string
		hold bool
		logs bool
	}{
		{
			"hold",
			true,
			false,
		},
		{
			"logs",
			false,
			true,
		},
		{
			"hold and logs",
			true,
			true,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					tmp  = t.TempDir()
					logs = filepath.Join(
						t.TempDir(),
						"logs",
					)
					script = _Scripted(
						t,
						"def main():\n    for i in range(200000):\n"+
							"        print(\"line\", i)\n",
					)
					args = []string{"run"}
				)

				t.Setenv(
					"TMPDIR",
					tmp,
				)

				if tc.hold {
					args = append(
						args,
						"-hold",
					)
				}

				if tc.logs {
					args = append(
						args,
						"-logs",
						logs,
					)
				}

				args = append(
					args,
					script,
				)

				code := _EndedEarly(
					t,
					args,
				)
				if code != STOPPED_CODE {
					t.Fatalf(
						"exit %d, want %d",
						code,
						STOPPED_CODE,
					)
				}

				entries, err := os.ReadDir(tmp)
				if err != nil || len(entries) != 0 {
					t.Fatalf(
						"TMPDIR holds %d entries (%v), want none",
						len(entries),
						err,
					)
				}

				if !tc.logs {
					return
				}

				raw, err := os.ReadFile(filepath.Join(
					logs,
					"held.star.log",
				))
				if err != nil || len(raw) == 0 || raw[len(raw)-1] != '\n' {
					t.Fatalf(
						"log file has %d bytes (%v), want whole lines",
						len(raw),
						err,
					)
				}

				for _, line := range strings.Split(
					strings.TrimSuffix(
						string(raw),
						"\n",
					),
					"\n",
				) {
					if !strings.HasPrefix(
						line,
						"time=",
					) || !strings.Contains(
						line,
						"msg=",
					) {
						t.Fatalf(
							"log line %q is not whole",
							line,
						)
					}
				}
			},
		)
	}
}

// _EndedEarly runs lark with args, reads one byte of its standard output,
// closes the pipe, and returns the exit code.
//
// Revisions:
//   - 2026-11-03 21:50: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _EndedEarly(
	t *testing.T,
	args []string,
) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		t.Context(),
		RUN_LIMIT,
	)

	defer cancel()

	var (
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			args...,
		)
		exit *exec.ExitError
	)

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	cmd.Stdout = write

	_Grouped(cmd)

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	_ = write.Close()

	_, _ = read.Read(make(
		[]byte,
		1,
	))
	_ = read.Close()

	err = cmd.Wait()
	if errors.As(
		err,
		&exit,
	) {
		return exit.ExitCode()
	}

	if err != nil {
		t.Fatal(err)
	}

	return 0
}

// _Asleep waits until the marker file at path is there, which the script writes
// once it has printed its first line and just before it calls napping. An event,
// not a delay: a loaded machine starts the child late, and the wait follows it.
//
// Revisions:
//   - 2026-11-03 22:30: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: waits for a marker file, not for the changes file (C119, Q304)
func _Asleep(
	t *testing.T,
	path string,
) {
	t.Helper()

	for {
		_, err := os.Stat(path)
		if err == nil {
			return
		}

		select {
		case <-t.Context().Done():
			t.Fatal("the script never started napping")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
