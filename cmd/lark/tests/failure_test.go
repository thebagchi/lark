package tests

// This file tests the failure paths of the host: Hosting leaves nothing when it
// fails, Abandoning prints one line and exits 5 and writes the -graph patches,
// Finish ends a memory limit with 5, and Refusal refuses the arguments a verb
// cannot take. The binary reaches them only on a real failure, and these tests need
// the failure at a chosen point (CLAUDE.md 1.8.1; C65a).
//
// Revisions:
//   - 2026-11-08 22:49: header states why the file is internal (C65a)
//   - 2026-11-10 01:37: moved to tests/ from main_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)
//   - 2026-11-10 11:45: the graph file holds patches; Refusal is by verb (C119, Q304, Q305)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

const (
	// LONG is a directory name long enough that a socket beneath it is past
	// what a unix socket path may hold, and SOCKET_MODE the mode it is made with.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LONG        = 120
	SOCKET_MODE = 0o700

	// UNPATTERNED is a plugin directory whose name the search cannot read as a
	// pattern, so loading from it fails.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	UNPATTERNED = "[x"
)

// TestHosting_LeavesNothingWhenItFails fails plugin hosting in the two ways
// that come after the socket's directory is made - the listener refusing its
// socket, and the plugins failing to load - and expects that directory gone
// either way.
//
// Internal, because Hosting is main's: the package has no surface a test
// outside it could reach, and the directory left behind is the invariant.
//
// Revisions:
//   - 2026-10-03 20:28: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHosting_LeavesNothingWhenItFails(t *testing.T) {
	var (
		short = t.TempDir()
		long  = filepath.Join(
			t.TempDir(),
			strings.Repeat(
				"d",
				LONG,
			),
		)
	)

	err := os.Mkdir(
		long,
		SOCKET_MODE,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, temp, plugins string }{
		{
			"a socket path too long to listen on",
			long,
			short,
		},
		{
			"a plugin directory the search cannot read",
			short,
			filepath.Join(
				short,
				UNPATTERNED,
			),
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				t.Setenv(
					"TMPDIR",
					item.temp,
				)

				_, _, err := command.Hosting(item.plugins)
				if err == nil {
					t.Fatal("hosting started")
				}

				left, err := os.ReadDir(item.temp)
				if err != nil {
					t.Fatal(err)
				}

				for _, entry := range left {
					t.Errorf(
						"left %s behind",
						filepath.Join(
							item.temp,
							entry.Name(),
						),
					)
				}
			},
		)
	}
}

// TestAbandoning_PrintsOneLineAndExitsFive proves what the command does for a
// run that will not end after the memory watch tripped: one line, "lark: memory
// limit reached", and exit code 5. The first case calls the function with a
// fake exit; the second runs it in a child process and reads the real exit
// code, so no real memory pressure is needed.
//
// Internal, because Abandoning is main's and the package has no surface a test
// outside it could reach.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: passes the settle function (C27)
//   - 2026-11-02 23:47: the exit code in this text is 5 (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestAbandoning_PrintsOneLineAndExitsFive(t *testing.T) {
	const (
		CHILD = "LARK_ABANDON_CHILD"
		LINE  = "lark: memory limit reached\n"
	)

	if os.Getenv(CHILD) == "1" {
		command.Abandoning(
			os.Stderr,
			os.Exit,
			func() {
				// Nothing to settle in this case.
			},
		)(memwatch.ErrLimit)

		return
	}

	t.Run(
		"fake exit",
		func(t *testing.T) {
			var (
				out  strings.Builder
				code = -1
			)

			command.Abandoning(
				&out,
				func(c int) {
					code = c
				},
				func() {
					// Nothing to settle in this case.
				},
			)(memwatch.ErrLimit)

			if code != command.MEMORY || out.String() != LINE {
				t.Fatalf(
					"exit %d, said %q; want 5, %q",
					code,
					out.String(),
					LINE,
				)
			}
		},
	)

	t.Run(
		"child process",
		func(t *testing.T) {
			cmd := exec.Command(
				os.Args[0],
				"-test.run=^TestAbandoning_PrintsOneLineAndExitsFive$",
			)

			cmd.Env = append(
				os.Environ(),
				CHILD+"=1",
			)

			var stderr strings.Builder

			cmd.Stderr = &stderr

			var exit *exec.ExitError

			err := cmd.Run()
			if !errors.As(
				err,
				&exit,
			) || exit.ExitCode() != command.MEMORY {
				t.Fatalf(
					"got %v, want exit 5",
					err,
				)
			}

			if stderr.String() != LINE {
				t.Fatalf(
					"stderr %q, want %q",
					stderr.String(),
					LINE,
				)
			}
		},
	)
}

// TestAbandoning_LeavesThePatches proves the patches are in the -graph file when
// the memory watch abandons a run, through the code the command runs. A child
// process opens the -graph file and the options with _Prepared's own pieces
// (Opened, MustWatch, Watch), runs a sleeping script with Running, which stores
// the run, and calls Abandon, the function MustWatch hands the watch. The child
// exits 5 with the one line; the file holds one JSON Patch a line, the first of
// which makes the run RUNNING, and names the function the run was in.
//
// Internal, because the output, the held run and Abandon are main's.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: renamed from TestAbandoning_WritesTheGraphFile; reads patches in
//     place of a graph (C119, Q304)
func TestAbandoning_LeavesThePatches(t *testing.T) {
	const (
		CHILD    = "LARK_ABANDON_GRAPH_CHILD"
		SLEEPING = "def napping():\n    sleep(60000)\n\ndef main():\n    napping()\n"
	)

	if dir := os.Getenv(CHILD); dir != "" {
		_AbandonedChild(
			dir,
			SLEEPING,
			false,
		)

		return
	}

	if memwatch.Check() != nil {
		t.Skip("the system offers no memory watch")
	}

	var (
		dir = _AbandonedRun(
			t,
			CHILD,
			"-test.run=^TestAbandoning_LeavesThePatches$",
		)
		raw, err = os.ReadFile(filepath.Join(
			dir,
			"end.jsonl",
		))
	)
	if err != nil {
		t.Fatal(err)
	}

	_PatchLines(
		t,
		raw,
		"napping",
	)

	_Only(
		t,
		dir,
		"end.jsonl",
	)
}

// TestAbandoning_WritesTheHeldPatches proves a run held by -hold, and then
// abandoned by the memory watch, still writes the patches it held to the -graph
// file, and leaves no spool of them in the temporary directory. The child sets
// TMPDIR to a directory of its own, which must be empty afterwards.
//
// Internal, for the reason TestAbandoning_LeavesThePatches gives.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q304)
func TestAbandoning_WritesTheHeldPatches(t *testing.T) {
	const (
		CHILD    = "LARK_ABANDON_HELD_CHILD"
		SLEEPING = "def napping():\n    sleep(60000)\n\ndef main():\n    napping()\n"
	)

	if dir := os.Getenv(CHILD); dir != "" {
		_AbandonedChild(
			dir,
			SLEEPING,
			true,
		)

		return
	}

	if memwatch.Check() != nil {
		t.Skip("the system offers no memory watch")
	}

	var (
		dir = _AbandonedRun(
			t,
			CHILD,
			"-test.run=^TestAbandoning_WritesTheHeldPatches$",
		)
		raw, err = os.ReadFile(filepath.Join(
			dir,
			"end.jsonl",
		))
	)
	if err != nil {
		t.Fatal(err)
	}

	_PatchLines(
		t,
		raw,
		"napping",
	)

	_Only(
		t,
		_Spooling(dir),
		"",
	)
}

// TestAbandoning_BeforeTheRunIsHeld proves the abandon is safe when it fires
// before Running has stored the run: the child exits 5 with the one line, and
// the -graph file is there and empty.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the file is opened at once and stays empty (C119, Q304)
func TestAbandoning_BeforeTheRunIsHeld(t *testing.T) {
	const (
		CHILD = "LARK_ABANDON_EARLY_CHILD"
	)

	if dir := os.Getenv(CHILD); dir != "" {
		_AbandonedChild(
			dir,
			"",
			false,
		)

		return
	}

	dir := _AbandonedRun(
		t,
		CHILD,
		"-test.run=^TestAbandoning_BeforeTheRunIsHeld$",
	)

	_Only(
		t,
		dir,
		"end.jsonl",
	)

	info, err := os.Stat(filepath.Join(
		dir,
		"end.jsonl",
	))
	if err != nil {
		t.Fatal(err)
	}

	if info.Size() != 0 {
		t.Fatalf(
			"the -graph file holds %d bytes, want none",
			info.Size(),
		)
	}
}

// _Spooling is the directory the abandon children keep as their TMPDIR, beside
// dir and not in it, so that dir holds the files under test alone. The parent
// of dir is the test's own and is removed with it.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q304)
func _Spooling(dir string) string {
	return dir + ".tmp"
}

// _PatchLines fails the test unless raw is JSON Patch lines, one array of
// operations a line, the first of which sets the status to STATUS_RUNNING, and
// the text names want.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q304)
func _PatchLines(
	t *testing.T,
	raw []byte,
	want string,
) {
	t.Helper()

	lines := strings.Split(
		strings.TrimSuffix(
			string(raw),
			"\n",
		),
		"\n",
	)

	for _, line := range lines {
		var patch []map[string]any

		err := json.Unmarshal(
			[]byte(line),
			&patch,
		)
		if err != nil || len(patch) == 0 {
			t.Fatalf(
				"line %q is not a JSON Patch: %v",
				line,
				err,
			)
		}
	}

	if lines[0] != `[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]` {
		t.Fatalf(
			"first line %q, want the status set to running",
			lines[0],
		)
	}

	if !strings.Contains(
		string(raw),
		want,
	) {
		t.Fatalf(
			"patches %s do not name %s",
			raw,
			want,
		)
	}
}

// TestMustWatch_DoesNotTurnTrackingOn proves the options -memory gives a
// run leave the live graph off: the watch needs none, so the lock of the live
// graph is not paid for it. A -graph file asks for the changes by its own
// option, which MustWatch does not give. It reads the real options MustWatch
// returns, starts a run with them, and reads the run's Status once the script
// has printed (Q106). A live graph would hold main by then. A graph that is not
// live holds no function until the run ends.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-03 23:59: builds a _Run in place of a _Ready and a _Held (C57)
//   - 2026-11-07 23:04: starts a run and reads Status, in place of reading the
//     options through artifact.Apply, which is internal now (C91w6, Q106)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the abandon takes no snapshot any more (C119, Q304)
//   - 2026-11-10 23:25: renamed from TestMustWatch_DoesNotTurnTheLiveGraphOn (C121, Q350)
func TestMustWatch_DoesNotTurnTrackingOn(t *testing.T) {
	if memwatch.Check() != nil {
		t.Skip("the system offers no memory watch")
	}

	var (
		asked = &command.Asked{
			Graph: filepath.Join(
				t.TempDir(),
				"end.jsonl",
			),
			Memory: true,
		}
		output, err = command.Opened(asked)
	)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		_ = output.Close()
	}()

	options := command.MustWatch(
		asked,
		&command.Run{
			Output: output,
		},
	)
	if len(options) == 0 {
		t.Fatal("-memory gave the run no option")
	}

	built, err := runtime.Compile(&common.Source{
		Entry: "waiting.star",
		Text: []byte("def main():\n" +
			"    print(\"ready\")\n" +
			"    sleep(60)\n"),
	})
	if err != nil {
		t.Fatal(err)
	}

	var (
		tape      = new(_Tape)
		ctx, stop = context.WithCancel(t.Context())
	)

	defer stop()

	started, err := runtime.Start(
		ctx,
		built,
		append(
			options,
			runtime.WithLog(tape),
		)...,
	)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(
		tape._Text(),
		"ready",
	) {
		if time.Now().After(deadline) {
			t.Fatal("the script did not print within 10 s")
		}

		time.Sleep(time.Millisecond)
	}

	if got := started.Status().GetFunctions(); len(got) != 0 {
		t.Fatalf(
			"-memory turned the live graph on: Status holds %d functions",
			len(got),
		)
	}

	stop()

	_, _ = started.Wait()
}

// _Tape is a writer a test reads while a run writes to it.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Tape struct {
	lock sync.Mutex
	text strings.Builder
}

// Write appends p, under the lock.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (tape *_Tape) Write(p []byte) (int, error) {
	tape.lock.Lock()
	defer tape.lock.Unlock()

	return tape.text.Write(p)
}

// _Text is what has been written so far.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (tape *_Tape) _Text() string {
	tape.lock.Lock()
	defer tape.lock.Unlock()

	return tape.text.String()
}

// _AbandonedRun runs this test binary as a child for the test named by run,
// with env set to a new directory, expects exit 5 and the one line, and
// returns the directory.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _AbandonedRun(
	t *testing.T,
	env string,
	run string,
) string {
	t.Helper()

	var (
		dir = t.TempDir()
		cmd = exec.Command(
			os.Args[0],
			run,
		)
		stderr strings.Builder
		exit   *exec.ExitError
	)

	cmd.Env = append(
		os.Environ(),
		env+"="+dir,
	)
	cmd.Stderr = &stderr

	err := cmd.Run()
	if !errors.As(
		err,
		&exit,
	) || exit.ExitCode() != command.MEMORY {
		t.Fatalf(
			"got %v, want exit 5; stderr %q",
			err,
			stderr.String(),
		)
	}

	if stderr.String() != "lark: memory limit reached\n" {
		t.Fatalf(
			"stderr %q, want the one line",
			stderr.String(),
		)
	}

	return dir
}

// _Only fails the test unless dir holds the one file name, or no file when name
// is empty.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Only(
	t *testing.T,
	dir string,
	name string,
) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := 0
	if name != "" {
		want = 1
	}

	if len(entries) != want || (want == 1 && entries[0].Name() != name) {
		t.Fatalf(
			"directory holds %d files, want %d (%q)",
			len(entries),
			want,
			name,
		)
	}
}

// _AbandonedChild is the child of the tests above. It opens the -graph file in
// dir and the options through the command's own pieces, as _Prepared does, with
// the changes handed to the run. With a script it runs script through Running and
// waits until the output has been handed the change that names the function the
// script calls; without it nothing runs. Then it calls Abandon with the real
// exit. With held it asks for -hold, and keeps the spool of the patches in
// dir/tmp.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-03 23:59: builds a _Run in place of a _Ready and a _Held (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: hands the run the changes, and takes held (C119, Q304)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
//   - 2026-11-11 22:42: waits for the change that names the function to reach the output, in
//     place of polling the snapshot (C127, B275)
func _AbandonedChild(
	dir string,
	script string,
	held bool,
) {
	var (
		tmp = _Spooling(dir)
		err = os.Mkdir(
			tmp,
			PRIVATE,
		)
	)
	if err != nil {
		panic(err)
	}

	err = os.Setenv(
		"TMPDIR",
		tmp,
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
			Hold:   held,
			Memory: true,
		}
		output, oer = command.Opened(asked)
		run         = &command.Run{
			Output: output,
		}
	)
	if oer != nil {
		panic(oer)
	}

	var (
		watch  = output.Watch()
		seen   = make(chan struct{})
		signal sync.Once
	)

	run.Options = append(
		command.MustWatch(
			asked,
			run,
		),
		runtime.WithLog(io.Discard),
		runtime.WithChanges(func(change *workflowpb.Change) {
			watch(change)

			if strings.Contains(
				change.String(),
				"napping",
			) {
				signal.Do(func() {
					close(seen)
				})
			}
		}),
	)

	if script != "" {
		go func() {
			_ = command.Running(
				context.Background(),
				asked,
				[]byte(script),
				run,
			)
		}()

		// The change that names napping has been written when watch returns.
		// A snapshot that holds the function is not that state: the graph is
		// updated first and watch is called after, on the stepping thread.
		select {
		case <-seen:
		case <-time.After(30 * time.Second):
			panic("the run did not report napping within 30 s")
		}
	}

	command.Abandon(
		run,
		os.Stderr,
		os.Exit,
	)(memwatch.ErrLimit)
}

// TestFinish_AMemoryEndExitsFive proves a run the memory watch ended inside the
// grace, so that Wait returned an error wrapping memwatch.ErrLimit,
// exits 5 with the one line "lark: memory limit reached", not 1 with "script
// failed". A child process runs Finish with such an error.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestFinish_AMemoryEndExitsFive(t *testing.T) {
	const (
		CHILD = "LARK_FINISH_CHILD"
	)

	if os.Getenv(CHILD) == "1" {
		command.Finish(
			context.Background(),
			func() error {
				return nil
			},
			fmt.Errorf(
				"run: %w",
				memwatch.ErrLimit,
			),
			command.SCRIPT,
		)

		return
	}

	cmd := exec.Command(
		os.Args[0],
		"-test.run=^TestFinish_AMemoryEndExitsFive$",
	)

	var (
		stderr strings.Builder
		exit   *exec.ExitError
	)

	cmd.Env = append(
		os.Environ(),
		CHILD+"=1",
	)
	cmd.Stderr = &stderr

	err := cmd.Run()
	if !errors.As(
		err,
		&exit,
	) || exit.ExitCode() != command.MEMORY {
		t.Fatalf(
			"got %v, want exit 5; stderr %q",
			err,
			stderr.String(),
		)
	}

	if stderr.String() != "lark: memory limit reached\n" {
		t.Fatalf(
			"stderr %q, want the one line",
			stderr.String(),
		)
	}
}

// TestRefusal_ByVerb proves Refusal refuses what a verb cannot take, in the
// order its doc gives: the number of arguments (ErrArgs), -as (ErrAs), the
// missing -o of compile (ErrOutput), and a bundle given to compile or convert
// (ErrBundle) (Q305, Q308, Q309). Parse calls Refusal, so the table of
// TestParse_ReadsTheVerbs covers the same through the command line; this one
// calls it with an Asked, for the pairs a command line cannot make.
//
// Revisions:
//   - 2026-11-05 00:23: initial creation (C81)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: renamed from TestRefusal_CountsFilesGivenWithoutAFlag; by verb in
//     place of by flag (C119, Q305, Q308, Q309)
//   - 2026-11-10 17:15: uses ErrOutput and ErrArgs and ErrBundle, formerly ErrNoOutput and
//     ErrOneInput and ErrOnlyRun (C121, Q349)
func TestRefusal_ByVerb(t *testing.T) {
	for _, item := range []struct {
		name  string
		asked *command.Asked
		want  error
	}{
		{
			"run, one file",
			&command.Asked{
				Verb:  command.RUN,
				Files: []string{"a.star"},
				Noun:  command.SCRIPT,
			},
			nil,
		},
		{
			"run, two files",
			&command.Asked{
				Verb:  command.RUN,
				Files: []string{"a.star", "b.star"},
			},
			command.ErrArgs,
		},
		{
			"convert, no file",
			&command.Asked{
				Verb: command.CONVERT,
			},
			command.ErrArgs,
		},
		{
			"version, a file",
			&command.Asked{
				Verb:  command.VERSION,
				Files: []string{"a.star"},
			},
			command.ErrArgs,
		},
		{
			"help, two words",
			&command.Asked{
				Verb:  command.HELP,
				Files: []string{"run", "compile"},
			},
			command.ErrArgs,
		},
		{
			"help, one verb",
			&command.Asked{
				Verb:  command.HELP,
				Files: []string{"run"},
				Topic: "run",
			},
			nil,
		},
		{
			"-as beside a file",
			&command.Asked{
				Verb:  command.RUN,
				Files: []string{"a.star"},
				Noun:  command.SCRIPT,
				As:    command.AS_FLOW,
			},
			command.ErrAs,
		},
		{
			"-as with a kind that is none",
			&command.Asked{
				Verb:  command.RUN,
				Files: []string{"-"},
				Noun:  command.SCRIPT,
				Stdin: true,
				As:    "yaml",
			},
			command.ErrAs,
		},
		{
			"compile without -o",
			&command.Asked{
				Verb:  command.COMPILE,
				Files: []string{"a.star"},
				Noun:  command.SCRIPT,
			},
			command.ErrOutput,
		},
		{
			"compile a bundle",
			&command.Asked{
				Verb:  command.COMPILE,
				Files: []string{"a.bin"},
				Noun:  command.BUNDLE,
				Out:   "b.bin",
			},
			command.ErrBundle,
		},
		{
			"convert a bundle",
			&command.Asked{
				Verb:  command.CONVERT,
				Files: []string{"a.bin"},
				Noun:  command.BUNDLE,
			},
			command.ErrBundle,
		},
		{
			"run a bundle",
			&command.Asked{
				Verb:  command.RUN,
				Files: []string{"a.bin"},
				Noun:  command.BUNDLE,
			},
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				got := command.Refusal(item.asked)
				if !errors.Is(
					got,
					item.want,
				) || (item.want == nil && got != nil) {
					t.Fatalf(
						"got %v, want %v",
						got,
						item.want,
					)
				}
			},
		)
	}
}
