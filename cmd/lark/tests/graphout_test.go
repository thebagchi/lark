package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	// MAIN_FAILS fails in main itself, so no other function is to blame.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MAIN_FAILS = "def main():\n    fail(\"boom\")\n"

	// PRINTS prints one line, then ends well.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PRINTS = "def main():\n    print(\"visible\")\n"

	// LONG_NAME is how long a directory name is in the bounded-path case.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LONG_NAME = 200
)

// TestGraphPath_ErrorNamesTheUsersPath proves a graph that cannot be written
// names the path the user gave, quoted, with the error (B116, Q4), and cuts a
// long path at 100 bytes with a marker.
//
// Revisions:
//   - 2026-11-05 21:50: initial creation (C80)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run verb; there is no temporary file to hide
//     (C119, Q304)
func TestGraphPath_ErrorNamesTheUsersPath(t *testing.T) {
	var (
		path = _Script(
			t,
			SUCCEEDS,
		)
		missing = filepath.Join(
			t.TempDir(),
			"no",
			"g.jsonl",
		)
		long = filepath.Join(
			t.TempDir(),
			strings.Repeat(
				"d",
				LONG_NAME,
			),
			"g.jsonl",
		)
	)

	t.Run(
		"missing directory",
		func(t *testing.T) {
			var (
				got = _Ran(
					t,
					"run",
					"-graph",
					missing,
					path,
				)
				want = "lark: graph " + strconv.Quote(missing) +
					": no such file or directory\n"
			)

			if got.stderr != want || got.code != EXIT_NO_FILE {
				t.Fatalf(
					"exit %d, stderr %q, want exit %d, stderr %q",
					got.code,
					got.stderr,
					EXIT_NO_FILE,
					want,
				)
			}
		},
	)

	t.Run(
		"long path",
		func(t *testing.T) {
			got := _Ran(
				t,
				"run",
				"-graph",
				long,
				path,
			)

			if !strings.Contains(
				got.stderr,
				"... ("+strconv.Itoa(len(long))+" bytes): no such file",
			) || len(got.stderr) > 250 {
				t.Fatalf(
					"stderr %q is not the bounded form",
					got.stderr,
				)
			}
		},
	)
}

// TestGraphStdout_JSONAloneOnStandardOutput proves -graph - writes the same
// patch lines a file would, and only those, to standard output, with what the
// script prints on standard error, including held lines (Q191, Q304).
//
// Revisions:
//   - 2026-11-05 21:50: initial creation (C80)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the lines are patches, streamed or held (C119, Q304)
func TestGraphStdout_JSONAloneOnStandardOutput(t *testing.T) {
	var (
		path = _Script(
			t,
			PRINTS,
		)
		file = filepath.Join(
			t.TempDir(),
			"g.jsonl",
		)
	)

	asFile := _Ran(
		t,
		"run",
		"-graph",
		file,
		path,
	)
	if asFile.code != 0 {
		t.Fatalf(
			"file run: %+v",
			asFile,
		)
	}

	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{
			"streamed",
			nil,
		},
		{
			"held",
			[]string{"-hold"},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := _Ran(
					t,
					append(
						append(
							[]string{"run", "-graph", "-"},
							tc.args...,
						),
						path,
					)...,
				)

				if got.code != 0 {
					t.Fatalf(
						"exit %d: %s",
						got.code,
						got.stderr,
					)
				}

				if got.stdout != string(want) {
					t.Fatalf(
						"stdout %q, want the patches %q",
						got.stdout,
						want,
					)
				}

				if !strings.Contains(
					got.stderr,
					"visible",
				) {
					t.Fatalf(
						"the print is not on stderr: %q",
						got.stderr,
					)
				}
			},
		)
	}
}

// TestGraphStdout_FailedRunStillWrites proves a failed run writes its patches to
// standard output, the last of which sets the status to failed (and adds the
// cause), exits 1, and leaves no file named -.
//
// Revisions:
//   - 2026-11-05 21:50: initial creation (C80)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the output is patches (C119, Q304)
func TestGraphStdout_FailedRunStillWrites(t *testing.T) {
	var (
		path = _Script(
			t,
			FAILS,
		)
		got = _Ran(
			t,
			"run",
			"-graph",
			"-",
			path,
		)
		lines = strings.Split(
			strings.TrimSuffix(
				got.stdout,
				"\n",
			),
			"\n",
		)
	)

	if got.code != EXIT_FAILED || !strings.HasPrefix(
		lines[len(lines)-1],
		`[{"op":"add","path":"/status","value":"STATUS_FAILED"},`,
	) {
		t.Fatalf(
			"exit %d, stdout %q",
			got.code,
			got.stdout,
		)
	}
}

// TestGraphPath_FullDeviceNamesTheRealError proves a -graph file that opens but
// cannot take a write reports the write's own error, no space left on the
// device, and not "permission denied" (B255). The command used to make a
// temporary file beside the path, which a user cannot do in /dev. The exit is 3
// and the message is one "lark:" line, streamed or held.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B255)
func TestGraphPath_FullDeviceNamesTheRealError(t *testing.T) {
	_, err := os.Stat("/dev/full")
	if err != nil {
		t.Skip("the system has no /dev/full")
	}

	path := _Script(
		t,
		SUCCEEDS,
	)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			"streamed",
			[]string{"run", "-graph", "/dev/full", path},
		},
		{
			"held",
			[]string{"run", "-hold", "-graph", "/dev/full", path},
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := _Ran(
					t,
					tc.args...,
				)

				if got.code != EXIT_NO_FILE || !strings.HasSuffix(
					got.stderr,
					"lark: graph \"/dev/full\": no space left on device\n",
				) || strings.Contains(
					got.stderr,
					"permission denied",
				) {
					t.Fatalf(
						"exit %d, stderr %q, want exit %d, the real error",
						got.code,
						got.stderr,
						EXIT_NO_FILE,
					)
				}
			},
		)
	}
}

// TestCause_NamedOnlyForANestedFailure proves a failed run prints "cause: F"
// as the last stderr line when F is the failing function, F is not main, and
// the failure text does not already name F (Q190).
//
// Revisions:
//   - 2026-11-05 21:50: initial creation (C80)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestCause_NamedOnlyForANestedFailure(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			"nested function",
			_Script(
				t,
				FAILS,
			),
			"cause: bad\n",
		},
		{
			"main itself",
			_Script(
				t,
				MAIN_FAILS,
			),
			"",
		},
		{
			"text already names it",
			"../../../samples/failkinds.star",
			"",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := _Ran(
					t,
					"run",
					tc.path,
				)

				if got.code != EXIT_FAILED {
					t.Fatalf(
						"exit %d: %s",
						got.code,
						got.stderr,
					)
				}

				has := strings.Contains(
					got.stderr,
					"cause: ",
				)
				if tc.want == "" && has {
					t.Fatalf(
						"unexpected cause line: %q",
						got.stderr,
					)
				}

				if tc.want != "" && !strings.HasSuffix(
					got.stderr,
					"\n"+tc.want,
				) {
					t.Fatalf(
						"stderr %q does not end with %q",
						got.stderr,
						tc.want,
					)
				}
			},
		)
	}
}
