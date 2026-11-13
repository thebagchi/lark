package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	// NEAR_MISS is the warning for the -args key "hosts", a near miss of the
	// declared "host".
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 21:23: doc moved to UNUSED_SCRIPT and this one written (C113, B234)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NEAR_MISS = `lark: argument "hosts" is not declared by the script ` +
		`(did you mean "host"?)`

	// UNUSED_SCRIPT declares host and port, which main prints, so the test
	// holds before and after unused declarations are dropped (Q164).
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 21:23: doc moved here from NEAR_MISS (C113, B234)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	UNUSED_SCRIPT = "server = arg(\"host\", \"localhost\")\n" +
		"number = arg(\"port\", 8080)\n\n" +
		"def main():\n    print(server, number)\n"
)

// _Written writes UNUSED_SCRIPT to a new directory and returns its path.
//
// Revisions:
//   - 2026-11-05 19:30: initial creation (C71)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Written(t *testing.T) string {
	t.Helper()

	var path = filepath.Join(
		t.TempDir(),
		"unused.star",
	)

	err := os.WriteFile(
		path,
		[]byte(UNUSED_SCRIPT),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// _Translated is the flow of the script at path, written beside it.
//
// Revisions:
//   - 2026-11-05 19:30: initial creation (C71)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: converts with the convert verb (C119, Q306)
func _Translated(
	t *testing.T,
	path string,
) string {
	t.Helper()

	out, err := exec.CommandContext(
		t.Context(),
		BUILT,
		"convert",
		path,
	).Output()
	if err != nil {
		t.Fatal(err)
	}

	flow := filepath.Join(
		filepath.Dir(path),
		"unused.json",
	)

	err = os.WriteFile(
		flow,
		out,
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	return flow
}

// _Said runs the command and returns its exit code, what it printed, and what
// it wrote to standard error.
//
// Revisions:
//   - 2026-11-05 19:30: initial creation (C71)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Said(
	t *testing.T,
	args ...string,
) (int, string, string) {
	t.Helper()

	var (
		out, errs bytes.Buffer
		exit      *exec.ExitError
		code      int
		cmd       = exec.CommandContext(
			t.Context(),
			BUILT,
			args...,
		)
	)

	cmd.Stdout = &out
	cmd.Stderr = &errs

	err := cmd.Run()
	if errors.As(
		err,
		&exit,
	) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return code, out.String(), errs.String()
}

// TestArgs_WarnsOfAKeyNoArgDeclares proves the command names each key no arg()
// declares on standard error, once, with a hint for a near miss, drops it, and
// runs to exit 0 (Q136, Q186); that a declared key gets no warning and its
// value arrives; and that a 5000-byte key is cut (Q4). A bundle run has no
// source, so the library refuses the key and the exit is 1 (Q170, Q185).
//
// Revisions:
//   - 2026-11-05 19:30: initial creation (C71)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run and compile verbs (C119, Q305)
func TestArgs_WarnsOfAKeyNoArgDeclares(t *testing.T) {
	var (
		script = _Written(t)
		flow   = _Translated(
			t,
			script,
		)
		bundle = filepath.Join(
			filepath.Dir(script),
			"unused.bin",
		)
	)

	code, stderr := _Run(
		t,
		"compile",
		"-o",
		bundle,
		script,
	)
	if code != 0 {
		t.Fatalf(
			"bundle: exit %d (stderr %q)",
			code,
			stderr,
		)
	}

	var (
		long = strings.Repeat(
			"k",
			5000,
		)
		tests = []struct {
			name  string
			args  []string
			code  int
			lines []string
			has   []string
			not   []string
		}{
			{
				name: "script, declared key",
				args: []string{"run", "-args", `{"host": "db"}`, script},
				has:  []string{"db 8080"},
				not:  []string{"not declared"},
			},
			{
				name: "script, unknown key",
				args: []string{"run", "-args", `{"colour": "red"}`, script},
				lines: []string{
					`lark: argument "colour" is not declared by the script`,
				},
				has: []string{"localhost 8080"},
			},
			{
				name: "script, near miss",
				args: []string{"run", "-args", `{"hosts": "db"}`, script},
				lines: []string{
					NEAR_MISS,
				},
				has: []string{"localhost 8080"},
			},
			{
				name: "script, two unknown, one declared",
				args: []string{
					"run", "-args",
					`{"zeta": 1, "alpha": 2, "port": 9}`, script,
				},
				lines: []string{
					`lark: argument "alpha" is not declared by the script`,
					`lark: argument "zeta" is not declared by the script`,
				},
				has: []string{"localhost 9"},
			},
			{
				name: "script, long key",
				args: []string{"run", "-args", `{"` + long + `": 1}`, script},
				has:  []string{"localhost 8080"},
			},
			{
				name: "flow, unknown key",
				args: []string{"run", "-args", `{"hosts": "db"}`, flow},
				lines: []string{
					NEAR_MISS,
				},
				has: []string{"localhost 8080"},
			},
			{
				name: "flow, declared key",
				args: []string{"run", "-args", `{"host": "db"}`, flow},
				has:  []string{"db 8080"},
				not:  []string{"not declared"},
			},
			{
				name: "bundle, declared key",
				args: []string{"run", "-args", `{"host": "db"}`, bundle},
				has:  []string{"db 8080"},
			},
			{
				name: "bundle, unknown key is refused",
				args: []string{"run", "-args", `{"hosts": "db"}`, bundle},
				code: 1,
				has:  []string{`"hosts"`, `(did you mean "host"?)`},
			},
		}
	)

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				code, printed, stderr := _Said(
					t,
					tc.args...,
				)
				if code != tc.code {
					t.Fatalf(
						"exit %d, want %d (stderr %q)",
						code,
						tc.code,
						stderr,
					)
				}

				var warned []string

				for line := range strings.SplitSeq(
					stderr,
					"\n",
				) {
					if strings.HasPrefix(
						line,
						"lark: argument ",
					) {
						warned = append(
							warned,
							line,
						)
					}
				}

				if tc.name == "script, long key" {
					if len(warned) != 1 || len(warned[0]) > 1000 ||
						!strings.HasPrefix(
							warned[0],
							"lark: argument \""+strings.Repeat(
								"k",
								16,
							),
						) {
						t.Fatalf(
							"want one bounded warning, got %d (%d)",
							len(warned),
							len(stderr),
						)
					}
				} else if tc.code == 0 &&
					strings.Join(
						warned,
						"\n",
					) != strings.Join(
						tc.lines,
						"\n",
					) {
					t.Fatalf(
						"warnings %q, want %q",
						warned,
						tc.lines,
					)
				}

				for _, want := range tc.has {
					if !strings.Contains(
						printed+stderr,
						want,
					) {
						t.Fatalf(
							"stderr %q lacks %s",
							stderr,
							strconv.Quote(want),
						)
					}
				}

				for _, bad := range tc.not {
					if strings.Contains(
						stderr,
						bad,
					) {
						t.Fatalf(
							"stderr %q has %s",
							stderr,
							strconv.Quote(bad),
						)
					}
				}
			},
		)
	}
}

// TestArgs_InlineArgCallIsDeclared proves a top-level arg() call that is not
// assigned declares its key: no warning, the value arrives (Q193); and that a
// misspelling of it warns with the hint, for a script and for a flow.
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71, Q193)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run verb (C119, Q305)
func TestArgs_InlineArgCallIsDeclared(t *testing.T) {
	var (
		script = filepath.Join(
			t.TempDir(),
			"inline.star",
		)
		text = "print(arg(\"greeting\", \"hi\"))\n\ndef main():\n    pass\n"
	)

	err := os.WriteFile(
		script,
		[]byte(text),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	flow := _Translated(
		t,
		script,
	)

	for _, mode := range [][]string{
		{"script", script},
		{"flow", flow},
	} {
		t.Run(
			mode[0],
			func(t *testing.T) {
				code, printed, stderr := _Said(
					t,
					"run",
					"-args",
					`{"greeting": "yo"}`,
					mode[1],
				)
				if code != 0 || stderr != "" || !strings.Contains(
					printed,
					"yo",
				) {
					t.Fatalf(
						"exit %d, stderr %q, printed %q",
						code,
						stderr,
						printed,
					)
				}

				code, printed, stderr = _Said(
					t,
					"run",
					"-args",
					`{"greetings": "yo"}`,
					mode[1],
				)
				if code != 0 || !strings.Contains(
					printed,
					"hi",
				) || !strings.Contains(
					stderr,
					`argument "greetings" is not declared by the script `+
						`(did you mean "greeting"?)`,
				) {
					t.Fatalf(
						"exit %d, stderr %q, printed %q",
						code,
						stderr,
						printed,
					)
				}
			},
		)
	}
}
