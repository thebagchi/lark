package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// EXIT_FAILED, EXIT_NO_FILE and EXIT_STOPPED are the codes lark exits
	// with when the script failed, when a file it was given could not be
	// read or written, and when a signal stopped the run.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-10 16:54: the doc names reading a file (C119, Q304)
	EXIT_FAILED  = 1
	EXIT_NO_FILE = 3
	EXIT_STOPPED = 4

	// SUCCEEDS, FAILS, WAITS and BROKEN are the scripts these tests run. WAITS
	// prints a line and then sleeps for a long time, so a test stops it.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SUCCEEDS = "def helper():\n    return 1\n\ndef main():\n    helper()\n    print(1)\n"
	FAILS    = "def bad():\n    fail(\"boom\")\n\ndef main():\n    bad()\n"
	WAITS    = "def main():\n    print(\"ready\")\n    sleep(60000)\n"
	BROKEN   = "def main(:\n"

	// READY is the line WAITS prints, and PATIENCE how long a test waits for
	// something another process does.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	READY    = "ready"
	PATIENCE = 20 * time.Second
	POLL     = 20 * time.Millisecond

	// RUNNING_LINE is the first line of every -graph file: the patch that makes
	// the run running.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q304)
	RUNNING_LINE = `[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]`

	// SUCCEEDED_LINE is the last line of the -graph file of a run that succeeded:
	// the patch that gives the run its end status.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q304)
	SUCCEEDED_LINE = `[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]`
)

// _Outcome is how one run of the lark binary ended.
//
// Revisions:
//   - 2026-11-02 21:22: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Outcome struct {
	code   int
	stdout string
	stderr string
}

// _Operation is one operation of a JSON Patch, as the -graph file spells it.
//
// Revisions:
//   - 2026-11-02 21:22: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: the doc names the -graph file (C119, Q304)
type _Operation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// _Script writes text as a script in a new directory and returns its path.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Script(
	t *testing.T,
	text string,
) string {
	t.Helper()

	var (
		path = filepath.Join(
			t.TempDir(),
			"s.star",
		)
		err error
	)

	err = os.WriteFile(
		path,
		[]byte(text),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// _Ran runs the binary with args and waits for it.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ran(
	t *testing.T,
	args ...string,
) *_Outcome {
	t.Helper()

	var (
		out, errs bytes.Buffer
		cmd       = exec.Command(
			BUILT,
			args...,
		)
	)

	cmd.Stdout = &out
	cmd.Stderr = &errs

	var (
		err  = cmd.Run()
		code = 0
	)
	if err != nil {
		var exited *exec.ExitError

		if !errors.As(
			err,
			&exited,
		) {
			t.Fatal(err)
		}

		code = exited.ExitCode()
	}

	return &_Outcome{
		code:   code,
		stdout: out.String(),
		stderr: errs.String(),
	}
}

// _Encoded is graph as protojson writes it, laid out compact by encoding/json,
// and one newline. It is what the patches of a -graph file build.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: describes the graph the patches build, there being no graph file
//     (C119, Q304)
func _Encoded(
	t *testing.T,
	graph *workflowpb.Graph,
) []byte {
	t.Helper()

	raw, err := protojson.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}

	var compact bytes.Buffer

	err = json.Compact(
		&compact,
		raw,
	)
	if err != nil {
		t.Fatal(err)
	}

	compact.WriteByte('\n')

	return compact.Bytes()
}

// _Library is the graph the library returns for the script at path.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func _Library(
	t *testing.T,
	path string,
) *workflowpb.Graph {
	t.Helper()

	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	built, err := runtime.Compile(&common.Source{
		Entry: path,
		Text:  text,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The error is the script's own, which a failing script has.
	result, _ := runtime.Eval(
		context.Background(),
		built,
	)

	return result.Graph
}

// _Names is what the directory holds, sorted, so a test sees a leftover file.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Names(
	t *testing.T,
	dir string,
) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	names := make(
		[]string,
		0,
		len(entries),
	)

	for _, entry := range entries {
		names = append(
			names,
			entry.Name(),
		)
	}

	return names
}

// TestGraph_IsTheLibrarysGraph runs a script that succeeds, one that fails and
// one that cannot be compiled, and checks the -graph file each leaves.
//
// A run writes one JSON Patch a line, whatever its end: patched onto {} in
// order, the lines must equal the graph Eval returns, as JSON, and the
// directory must hold the -graph file and nothing else. A script that does not
// compile never runs, so there is no change to write: the file is there, opened
// before the script was read, and empty.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the file holds patches, not the end graph (C119, Q304)
//   - 2026-11-10 17:15: names Eval, formerly Evaluate, in the text (C121, Q338)
func TestGraph_IsTheLibrarysGraph(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		code   int
		status string
		file   bool
	}{
		{
			"a script that succeeds",
			SUCCEEDS,
			0,
			"STATUS_SUCCEEDED",
			true,
		},
		{
			"a script that fails",
			FAILS,
			EXIT_FAILED,
			"STATUS_FAILED",
			true,
		},
		{
			"a script that does not compile",
			BROKEN,
			EXIT_FAILED,
			"",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					path = _Script(
						t,
						tc.text,
					)
					out   = t.TempDir()
					graph = filepath.Join(
						out,
						"end.jsonl",
					)
					got = _Ran(
						t,
						"run",
						"-graph",
						graph,
						path,
					)
				)

				if got.code != tc.code {
					t.Fatalf(
						"exit %d, want %d: %s",
						got.code,
						tc.code,
						got.stderr,
					)
				}

				if names := _Names(
					t,
					out,
				); !reflect.DeepEqual(
					names,
					[]string{"end.jsonl"},
				) {
					t.Fatalf(
						"directory holds %v, want only end.jsonl",
						names,
					)
				}

				lines := _Lines(
					t,
					graph,
				)

				if !tc.file {
					if len(lines) != 0 {
						t.Fatalf(
							"%d lines, want none",
							len(lines),
						)
					}

					return
				}

				var want any

				err := json.Unmarshal(
					_Encoded(
						t,
						_Library(
							t,
							path,
						),
					),
					&want,
				)
				if err != nil {
					t.Fatal(err)
				}

				patched := _Patched(
					t,
					lines,
				)
				if !reflect.DeepEqual(
					patched,
					want,
				) {
					t.Fatalf(
						"patched\n%v\nlibrary\n%v",
						patched,
						want,
					)
				}

				status := patched.(map[string]any)["status"]
				if status != tc.status {
					t.Fatalf(
						"status %v, want %s",
						status,
						tc.status,
					)
				}
			},
		)
	}
}

// TestGraph_StatusChangesAreLines proves the status of the run is a patch like
// any other (Q304): the first line sets it to STATUS_RUNNING, the last to the
// status the run ended with (a failed run adds its cause in the same line), and
// nothing but patches is in the file.
//
// Revisions:
//   - 2026-11-10 16:54: initial creation, in place of TestChanges_EndWhereTheGraphEnds
//     (C119, Q304)
func TestGraph_StatusChangesAreLines(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		code int
		last string
	}{
		{
			"a script that succeeds",
			SUCCEEDS,
			0,
			`[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]`,
		},
		{
			"a script that fails",
			FAILS,
			EXIT_FAILED,
			`[{"op":"add","path":"/status","value":"STATUS_FAILED"},`,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					path = _Script(
						t,
						tc.text,
					)
					graph = filepath.Join(
						t.TempDir(),
						"changes.jsonl",
					)
					got = _Ran(
						t,
						"run",
						"-graph",
						graph,
						path,
					)
				)

				if got.code != tc.code {
					t.Fatalf(
						"exit %d, want %d: %s",
						got.code,
						tc.code,
						got.stderr,
					)
				}

				lines := _Lines(
					t,
					graph,
				)
				if len(lines) < 2 {
					t.Fatalf(
						"%d lines, want two or more",
						len(lines),
					)
				}

				if lines[0] != RUNNING_LINE {
					t.Fatalf(
						"first line %s, want the status set to running",
						lines[0],
					)
				}

				if !strings.HasPrefix(
					lines[len(lines)-1],
					tc.last,
				) {
					t.Fatalf(
						"last line %s, want %s",
						lines[len(lines)-1],
						tc.last,
					)
				}
			},
		)
	}
}

// TestGraph_ReplacesWhatWasThere checks that a second run empties a file the
// first run left, and writes its own patches to it: a file much longer than the
// patches ends with the last patch, not with the old text.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the file is emptied when the command starts (C119, Q304)
func TestGraph_ReplacesWhatWasThere(t *testing.T) {
	var (
		path = _Script(
			t,
			SUCCEEDS,
		)
		graph = filepath.Join(
			t.TempDir(),
			"end.jsonl",
		)
	)

	err := os.WriteFile(
		graph,
		[]byte("old and much longer than the graph is, "+strings.Repeat(
			"x",
			4096,
		)),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := _Ran(
		t,
		"run",
		"-graph",
		graph,
		path,
	)
	if got.code != 0 {
		t.Fatalf(
			"exit %d: %s",
			got.code,
			got.stderr,
		)
	}

	var want any

	err = json.Unmarshal(
		_Encoded(
			t,
			_Library(
				t,
				path,
			),
		),
		&want,
	)
	if err != nil {
		t.Fatal(err)
	}

	patched := _Patched(
		t,
		_Lines(
			t,
			graph,
		),
	)
	if !reflect.DeepEqual(
		patched,
		want,
	) {
		t.Fatalf(
			"patched\n%v\nlibrary\n%v",
			patched,
			want,
		)
	}
}

// _Patched is the document the lines of a -graph file build from {}, each
// line a JSON Patch of add and remove operations.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: the doc names the -graph file (C119, Q304)
func _Patched(
	t *testing.T,
	lines []string,
) any {
	t.Helper()

	var doc any = map[string]any{}

	for _, line := range lines {
		var ops []_Operation

		err := json.Unmarshal(
			[]byte(line),
			&ops,
		)
		if err != nil {
			t.Fatalf(
				"line %q is not a JSON Patch: %v",
				line,
				err,
			)
		}

		for _, op := range ops {
			parts := strings.Split(
				op.Path,
				"/",
			)[1:]
			doc = _Applied(
				t,
				doc,
				parts,
				&op,
			)
		}
	}

	return doc
}

// _Applied is doc with one add or remove applied at the pointer parts.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Applied(
	t *testing.T,
	doc any,
	parts []string,
	op *_Operation,
) any {
	t.Helper()

	last := len(parts) == 1

	switch held := doc.(type) {
	case map[string]any:
		switch {
		case !last:
			held[parts[0]] = _Applied(
				t,
				held[parts[0]],
				parts[1:],
				op,
			)
		case op.Op == "remove":
			delete(
				held,
				parts[0],
			)
		default:
			held[parts[0]] = op.Value
		}

		return held

	case []any:
		if last && parts[0] == "-" {
			return append(
				held,
				op.Value,
			)
		}

		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Fatal(err)
		}

		switch {
		case !last:
			held[idx] = _Applied(
				t,
				held[idx],
				parts[1:],
				op,
			)
		case op.Op == "remove":
			held = append(
				held[:idx],
				held[idx+1:]...,
			)
		default:
			held = append(
				held[:idx],
				append(
					[]any{op.Value},
					held[idx:]...,
				)...,
			)
		}

		return held

	default:
		t.Fatalf(
			"path %v goes into %T",
			parts,
			doc,
		)

		return nil
	}
}

// _Lines is the lines of the file at path.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Lines(
	t *testing.T,
	path string,
) []string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(raw) == 0 {
		return nil
	}

	if raw[len(raw)-1] != '\n' {
		t.Fatalf(
			"the file ends inside a line: %q",
			raw[len(raw)-20:],
		)
	}

	return strings.Split(
		strings.TrimSuffix(
			string(raw),
			"\n",
		),
		"\n",
	)
}

// TestStop_WritesTheCancelledPatchesAndFlushesEachLine starts a script that
// sleeps, waits until it has printed, and reads the -graph file while the run is
// still going: every line there is whole, and there is one. It then sends
// SIGTERM and expects exit 4, and patches that build a CANCELLED graph, and no
// file but the -graph file.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: renamed from TestStop_WritesTheCancelledGraphAndFlushesEachLine;
//     one file of patches in place of the graph and the changes (C119, Q304)
func TestStop_WritesTheCancelledPatchesAndFlushesEachLine(t *testing.T) {
	var (
		path = _Script(
			t,
			WAITS,
		)
		out   = t.TempDir()
		graph = filepath.Join(
			out,
			"changes.jsonl",
		)
		cmd = exec.Command(
			BUILT,
			"run",
			"-graph",
			graph,
			path,
		)
	)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	var (
		scanner = bufio.NewScanner(pipe)
		ready   = make(
			chan bool,
			1,
		)
	)

	go func() {
		for scanner.Scan() {
			if strings.Contains(
				scanner.Text(),
				READY,
			) {
				ready <- true

				return
			}
		}

		ready <- false
	}()

	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("the script ended before it printed")
		}
	case <-time.After(PATIENCE):
		_ = cmd.Process.Kill()

		t.Fatal("the script did not print")
	}

	// The print is after the call that made the first change, so the line is
	// in the file already if each line is written as it is made.
	if lines := _Lines(
		t,
		graph,
	); len(lines) == 0 {
		_ = cmd.Process.Kill()

		t.Fatal("nothing in the -graph file while the run goes")
	}

	err = cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for scanner.Scan() {
		}
	}()

	done := make(
		chan error,
		1,
	)

	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
	case <-time.After(PATIENCE):
		_ = cmd.Process.Kill()

		t.Fatal("lark did not stop")
	}

	var exited *exec.ExitError
	if !errors.As(
		err,
		&exited,
	) || exited.ExitCode() != EXIT_STOPPED {
		t.Fatalf(
			"exit %v, want %d",
			err,
			EXIT_STOPPED,
		)
	}

	lines := _Lines(
		t,
		graph,
	)

	if status := _Patched(
		t,
		lines,
	).(map[string]any)["status"]; status != "STATUS_CANCELLED" {
		t.Fatalf(
			"status %v, want STATUS_CANCELLED",
			status,
		)
	}

	if names := _Names(
		t,
		out,
	); !reflect.DeepEqual(
		names,
		[]string{"changes.jsonl"},
	) {
		t.Fatalf(
			"directory holds %v",
			names,
		)
	}
}

// TestOutput_RefusesAPathItCannotWrite gives -graph a file in a directory that
// does not exist, and a directory. Each must exit 3 with a message that opens
// "lark: graph ", before the script has printed anything, and leave nothing
// behind.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: -graph alone; the changes file is gone (C119, Q304)
func TestOutput_RefusesAPathItCannotWrite(t *testing.T) {
	var (
		path = _Script(
			t,
			SUCCEEDS,
		)
		out     = t.TempDir()
		missing = filepath.Join(
			out,
			"no",
			"such",
			"file.json",
		)
	)

	cases := []struct {
		name string
		flag string
		file string
		noun string
	}{
		{
			"graph in a missing directory",
			"-graph",
			missing,
			"graph",
		},
		{
			"graph that is a directory",
			"-graph",
			out,
			"graph",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := _Ran(
					t,
					"run",
					tc.flag,
					tc.file,
					path,
				)

				if got.code != EXIT_NO_FILE {
					t.Fatalf(
						"exit %d, want %d: %s",
						got.code,
						EXIT_NO_FILE,
						got.stderr,
					)
				}

				if !strings.HasPrefix(
					got.stderr,
					"lark: "+tc.noun+" ",
				) {
					t.Fatalf(
						"message %q does not open \"lark: %s \"",
						got.stderr,
						tc.noun,
					)
				}

				if got.stdout != "" {
					t.Fatalf(
						"the script ran: %q",
						got.stdout,
					)
				}

				if names := _Names(
					t,
					out,
				); len(names) != 0 {
					t.Fatalf(
						"directory holds %v",
						names,
					)
				}
			},
		)
	}
}

// TestOutput_NeedsARun checks that -graph beside convert or compile is refused
// with exit 2 as a flag the verb does not have, since only run makes a run
// (Q305), and that nothing is made.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: by verb, in place of beside -translate and -bundle (C119, Q305)
func TestOutput_NeedsARun(t *testing.T) {
	var (
		path = _Script(
			t,
			SUCCEEDS,
		)
		out = t.TempDir()
	)

	cases := map[string][]string{
		"convert": {"convert", "-graph", filepath.Join(
			out,
			"g.jsonl",
		), path},
		"compile": {"compile", "-o", filepath.Join(
			out,
			"b.bin",
		), "-graph", filepath.Join(
			out,
			"g.jsonl",
		), path},
	}

	for name, args := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got := _Ran(
					t,
					args...,
				)

				if got.code != EXIT_FLAGS {
					t.Fatalf(
						"exit %d, want %d: %s",
						got.code,
						EXIT_FLAGS,
						got.stderr,
					)
				}

				if !strings.HasPrefix(
					got.stderr,
					REFUSED+": -graph",
				) {
					t.Fatalf(
						"message %q, want the refusal of -graph",
						got.stderr,
					)
				}

				if names := _Names(
					t,
					out,
				); len(names) != 0 {
					t.Fatalf(
						"directory holds %v",
						names,
					)
				}
			},
		)
	}
}
