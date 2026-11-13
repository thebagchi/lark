package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	// HELP_GOLDEN is the help text the user approved (Q172, Q310), byte for byte.
	//
	// Revisions:
	//   - 2026-11-05 19:05: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:34: holds the text, not the file name (C114, Q284, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-10 16:54: the text of the verbs, Examples and the signals of exit 4
	//     (C119, Q299, Q310, Q336, Q337, Q344)
	HELP_GOLDEN = `
lark runs a Starlark script as a graph of concurrent steps.

Usage:
  lark run [flags] FILE       run a script, flow or bundle
  lark compile -o OUT FILE    compile a script or flow into a bundle
  lark convert FILE           print a script as a flow, a flow as a script
  lark version                print the versions of lark and Go
  lark help [verb]            print this help, or one verb's help

FILE: .bin is a bundle, .json a flow, anything else a script.
FILE of - reads standard input as a script; -as flow or
-as bundle reads it as that kind.

run:
  -args JSON     arguments for arg(), as a JSON object
  -plugins DIR   start each lark-*.bin plugin in DIR
  -memory        end the run at 95% memory in use (exit 5)
  -hold          keep what the script prints until the run ends
  -logs DIR      also keep it in DIR/<script>.log
  -graph FILE    write each graph change to FILE, one JSON
                 Patch a line (- for standard output); live,
                 or all at the end under -hold

compile:
  -o FILE        the bundle to write (required)
  -plugins DIR   plugins the script uses

Examples:
  lark run hello.star
  lark run -args '{"host": "db.internal"}' -logs logs build.star
  echo 'def main(): print("hi")' | lark run -
  lark compile -o build.bin build.star
  lark run build.bin
  lark convert build.star

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch

See README.md for the language and the samples.
For the Go API, run: go doc github.com/thebagchi/lark/v1/runtime
`

	// HELP_WIDTH is the most columns a line of the help may have.
	//
	// Revisions:
	//   - 2026-11-05 19:05: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	HELP_WIDTH = 79

	// TRY is the line that follows every command-line error (C68).
	//
	// Revisions:
	//   - 2026-11-05 19:05: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-10 11:45: names the help verb (C119, Q305)
	TRY = "try lark help\n"

	// WRONG_AS is the refusal of -as beside a FILE that is not standard input.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q308)
	WRONG_AS = "lark: wrong -as (it goes with FILE of -; a file's kind is its extension)\n"

	// ONLY_RUN is the refusal of a bundle given to compile or convert.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q303)
	ONLY_RUN = "lark: a bundle is only run (compile and convert need a script or a flow)\n"
)

// _HelpRun runs the built command with args and returns its standard output,
// its standard error and its exit code.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C68)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _HelpRun(
	t *testing.T,
	args ...string,
) (string, string, int) {
	t.Helper()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd := exec.Command(
		BUILT,
		args...,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var (
		code = 0
		exit *exec.ExitError
	)

	if errors.As(
		err,
		&exit,
	) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	return stdout.String(), stderr.String(), code
}

// TestHelp_PrintsApprovedText proves the help verb and, as a first word, -h,
// -help and the double-dash forms print the approved text (Q172, Q310) on
// standard output, exit 0 (Q173) and leave standard error empty.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C68)
//   - 2026-11-09 11:34: compares with the HELP_GOLDEN text (C114, Q284, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the help verb joins the flags (C119, Q305, Q310)
func TestHelp_PrintsApprovedText(t *testing.T) {
	for _, flag := range []string{"help", "-h", "-help", "--h", "--help"} {
		t.Run(
			flag,
			func(t *testing.T) {
				stdout, stderr, code := _HelpRun(
					t,
					flag,
				)

				if code != 0 {
					t.Fatalf(
						"exit %d, want 0",
						code,
					)
				}

				if stdout != strings.TrimPrefix(
					HELP_GOLDEN,
					"\n",
				) {
					t.Fatalf(
						"standard output differs from HELP_GOLDEN:\n%s",
						stdout,
					)
				}

				if stderr != "" {
					t.Fatalf(
						"standard error %q, want empty",
						stderr,
					)
				}
			},
		)
	}
}

// TestHelp_WidthAndNames proves no line of the approved text is wider than 79
// columns, the text is ASCII, and every verb and every flag the command declares
// is named in it, with SIGHUP among the signals that stop a run (Q299), as the
// reader check of the README says.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C68)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: the verbs, their flags, SIGHUP, Examples and the closed standard
//     output (C119, Q299, Q305, Q336)
func TestHelp_WidthAndNames(t *testing.T) {
	stdout, _, _ := _HelpRun(
		t,
		"help",
	)

	for number, line := range strings.Split(
		stdout,
		"\n",
	) {
		if len(line) > HELP_WIDTH {
			t.Errorf(
				"line %d is %d columns, want at most %d: %q",
				number+1,
				len(line),
				HELP_WIDTH,
				line,
			)
		}

		for _, r := range line {
			if r > '~' {
				t.Errorf(
					"line %d holds non-ASCII %q",
					number+1,
					r,
				)

				break
			}
		}
	}

	for _, name := range []string{
		"lark run ", "lark compile ", "lark convert ", "lark version ", "lark help ",
		"-args ", "-as ", "-graph ", "-hold ", "-logs ", "-memory ", "-o ",
		"-plugins ", "SIGHUP", "Examples:", "closed standard output",
	} {
		if !strings.Contains(
			stdout,
			name,
		) {
			t.Errorf(
				"the help does not name %s",
				name,
			)
		}
	}

	for _, gone := range []string{"-translate", "-bundle", "-changes", "-script", "-flow"} {
		if strings.Contains(
			stdout,
			gone,
		) {
			t.Errorf(
				"the help names %s, a flag the verbs replaced",
				gone,
			)
		}
	}
}

// TestHelp_OfOneVerb proves lark help VERB, and VERB -h, print the part of the
// help that belongs to the verb: its usage line, the paragraph on FILE for the
// verbs that take one, its flags, and the exit codes, and nothing of the other
// verbs. The help of a word that is no verb, or of two, is refused with exit 2.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
func TestHelp_OfOneVerb(t *testing.T) {
	for _, item := range []struct {
		verb string
		has  []string
		not  []string
	}{
		{
			"run",
			[]string{
				"lark run [flags] FILE", "FILE: .bin is a bundle", "  -graph FILE",
				"Exit codes:",
			},
			[]string{"lark compile", "  -o FILE", "lark version"},
		},
		{
			"compile",
			[]string{
				"lark compile -o OUT FILE", "FILE: .bin is a bundle", "  -o FILE",
				"Exit codes:",
			},
			[]string{"lark run", "-graph FILE", "-memory"},
		},
		{
			"convert",
			[]string{
				"lark convert FILE", "FILE: .bin is a bundle", "Exit codes:",
			},
			[]string{"lark run", "-graph FILE", "  -o FILE", "-plugins", "Examples:"},
		},
		{
			"version",
			[]string{"lark version ", "Exit codes:"},
			[]string{"FILE:", "-graph FILE", "lark run"},
		},
		{
			"help",
			[]string{"lark help [verb]", "Exit codes:"},
			[]string{"FILE:", "-graph FILE", "lark run"},
		},
	} {
		for _, args := range [][]string{
			{"help", item.verb},
			{item.verb, "-h"},
		} {
			t.Run(
				strings.Join(
					args,
					" ",
				),
				func(t *testing.T) {
					stdout, stderr, code := _HelpRun(
						t,
						args...,
					)
					if code != 0 || stderr != "" {
						t.Fatalf(
							"exit %d, stderr %q, want 0 and nothing",
							code,
							stderr,
						)
					}

					if !strings.HasPrefix(
						stdout,
						"Usage:\n  lark "+item.verb+" ",
					) || len(stdout) >= len(HELP_GOLDEN) {
						t.Fatalf(
							"standard output %q is not a verb's help",
							stdout,
						)
					}

					for _, want := range item.has {
						if !strings.Contains(
							stdout,
							want,
						) {
							t.Errorf(
								"the help of %s lacks %q",
								item.verb,
								want,
							)
						}
					}

					for _, bad := range item.not {
						if strings.Contains(
							stdout,
							bad,
						) {
							t.Errorf(
								"the help of %s holds %q",
								item.verb,
								bad,
							)
						}
					}
				},
			)
		}
	}

	for _, item := range []struct {
		name string
		args []string
		want string
	}{
		{
			"a word that is no verb",
			[]string{"help", "compil"},
			"lark: unknown verb \"compil\" (did you mean compile?)\n" + TRY,
		},
		{
			"two words",
			[]string{"help", "run", "compile"},
			"lark: wrong number of arguments (help takes one verb at most)\n" + TRY,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				stdout, stderr, code := _HelpRun(
					t,
					item.args...,
				)
				if code != EXIT_FLAGS || stdout != "" || stderr != item.want {
					t.Fatalf(
						"exit %d, stdout %q, stderr %q, want %d and %q",
						code,
						stdout,
						stderr,
						EXIT_FLAGS,
						item.want,
					)
				}
			},
		)
	}
}

// TestHelp_ErrorsAreTwoLines proves a command-line error prints the error line
// and "try lark help" on standard error, nothing on standard output, and exits 2.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C68)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the refusals of the verbs (C119, Q305, Q307, Q308, Q309)
func TestHelp_ErrorsAreTwoLines(t *testing.T) {
	var (
		script = filepath.Join(
			t.TempDir(),
			"hello.star",
		)
		err error
	)

	err = os.WriteFile(
		script,
		[]byte("def main():\n    pass\n"),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		name string
		args []string
		want string
	}{
		{
			"unknown flag with a hint",
			[]string{"run", "-grph", "x"},
			"flag provided but not defined: -grph (did you mean -graph?)\n" + TRY,
		},
		{
			"unknown flag without a hint",
			[]string{"run", "-zzzzzzzz"},
			"flag provided but not defined: -zzzzzzzz\n" + TRY,
		},
		{
			"flag without its value",
			[]string{"run", "-logs"},
			"flag needs an argument: -logs\n" + TRY,
		},
		{
			"no verb",
			nil,
			"lark: give a verb (run, compile, convert, version or help)\n" + TRY,
		},
		{
			"a verb that is none",
			[]string{"hello.star"},
			"lark: unknown verb \"hello.star\"\n" + TRY,
		},
		{
			"no input",
			[]string{"run"},
			"lark: wrong number of arguments (give exactly one FILE)\n" + TRY,
		},
		{
			"two inputs",
			[]string{"run", "a.star", "b.json"},
			"lark: wrong number of arguments (give exactly one FILE)\n" + TRY,
		},
		{
			"compile without -o",
			[]string{"compile", "a.star"},
			"lark: compile needs -o FILE\n" + TRY,
		},
		{
			"-as beside a file",
			[]string{"run", "-as", "flow", "a.json"},
			WRONG_AS + TRY,
		},
		{
			"-as of no kind",
			[]string{"run", "-as", "yaml", "-"},
			"lark: wrong -as (it is script, flow or bundle)\n" + TRY,
		},
		{
			"a bundle to compile",
			[]string{"compile", "-o", "b.bin", "a.bin"},
			ONLY_RUN + TRY,
		},
		{
			"a bundle to convert",
			[]string{"convert", "a.bin"},
			ONLY_RUN + TRY,
		},
		{
			"bad arguments",
			[]string{"run", "-args", "{bad", "@SCRIPT@"},
			"",
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				args := slices.Clone(item.args)
				for i := range args {
					if args[i] == "@SCRIPT@" {
						args[i] = script
					}
				}

				stdout, stderr, code := _HelpRun(
					t,
					args...,
				)

				if code != EXIT_FLAGS {
					t.Fatalf(
						"exit %d, want %d",
						code,
						EXIT_FLAGS,
					)
				}

				if stdout != "" {
					t.Fatalf(
						"standard output %q, want empty",
						stdout,
					)
				}

				lines := strings.SplitAfter(
					stderr,
					"\n",
				)

				if len(lines) != 3 || lines[2] != "" || lines[1] != TRY {
					t.Fatalf(
						"standard error %q, want the error line then %q",
						stderr,
						TRY,
					)
				}

				if item.want != "" && stderr != item.want {
					t.Fatalf(
						"standard error %q, want %q",
						stderr,
						item.want,
					)
				}

				if item.want == "" && !strings.HasPrefix(
					stderr,
					"lark: ",
				) {
					t.Fatalf(
						"standard error %q, want a lark: line",
						stderr,
					)
				}
			},
		)
	}
}
