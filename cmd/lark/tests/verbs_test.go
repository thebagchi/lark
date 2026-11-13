package tests

// This file tests the command line of the verbs (Q305-Q309) through Parse and
// HelpOf, and the bounded read of the input (B253, Q23) through Read. The
// binary is run for the same lines in table_test.go and help_test.go; these
// tests reach the parts a binary cannot show: the Asked it builds, a reading of
// the free memory that a test chooses, and a reader that never ends.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253, Q23, Q305, Q307, Q308, Q309)

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

// LIMITED is the shell text that limits the address space of the command it
// starts to about 2.5 GB, and gives it an endless standard input. 1.5 GB is not
// enough for the binary to start.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
const (
	LIMITED = `ulimit -v 2500000 && exec "$0" "$@" < /dev/zero`
)

// TestParse_ReadsTheVerbs proves Parse reads the verb, the kind of the input
// (by extension, or by -as for standard input), the path a failure names, and
// the flags of the verb, and refuses a line a verb cannot take with the
// sentinel the doc of Refusal names.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305, Q307, Q308, Q309)
//   - 2026-11-10 17:15: one-word names for the 4 sentinels it names (C121, Q349)
func TestParse_ReadsTheVerbs(t *testing.T) {
	for _, item := range []struct {
		name string
		args []string
		want *command.Asked
		fail error
	}{
		{
			"run a script",
			[]string{"run", "a.star"},
			&command.Asked{
				Verb: command.RUN,
				Noun: command.SCRIPT,
				File: "a.star",
				Path: "a.star",
			},
			nil,
		},
		{
			"anything but .bin and .json is a script",
			[]string{"run", "a.txt"},
			&command.Asked{
				Verb: command.RUN,
				Noun: command.SCRIPT,
				File: "a.txt",
				Path: "a.txt",
			},
			nil,
		},
		{
			"a .json is a flow",
			[]string{"run", "dir/a.json"},
			&command.Asked{
				Verb: command.RUN,
				Noun: command.FLOW,
				File: "dir/a.json",
				Path: "dir/a.json",
			},
			nil,
		},
		{
			"a .bin is a bundle",
			[]string{"run", "a.bin"},
			&command.Asked{
				Verb: command.RUN,
				Noun: command.BUNDLE,
				File: "a.bin",
				Path: "a.bin",
			},
			nil,
		},
		{
			"flags may follow the file",
			[]string{"run", "a.star", "-hold", "-logs", "d"},
			&command.Asked{
				Verb: command.RUN,
				Noun: command.SCRIPT,
				File: "a.star",
				Path: "a.star",
				Hold: true,
				Logs: "d",
			},
			nil,
		},
		{
			"every flag of run",
			[]string{
				"run", "-args", "{}", "-plugins", "p", "-memory", "-hold",
				"-logs", "l", "-graph", "-", "a.star",
			},
			&command.Asked{
				Verb:     command.RUN,
				Noun:     command.SCRIPT,
				File:     "a.star",
				Path:     "a.star",
				Supplied: "{}",
				Plugins:  "p",
				Memory:   true,
				Hold:     true,
				Logs:     "l",
				Graph:    "-",
			},
			nil,
		},
		{
			"standard input is a script",
			[]string{"run", "-"},
			&command.Asked{
				Verb:  command.RUN,
				Noun:  command.SCRIPT,
				File:  "-",
				Path:  "<stdin>",
				Stdin: true,
			},
			nil,
		},
		{
			"standard input as a flow",
			[]string{"run", "-as", "flow", "-"},
			&command.Asked{
				Verb:  command.RUN,
				Noun:  command.FLOW,
				File:  "-",
				Path:  "<stdin>",
				Stdin: true,
				As:    "flow",
			},
			nil,
		},
		{
			"standard input as a bundle",
			[]string{"run", "-as", "bundle", "-"},
			&command.Asked{
				Verb:  command.RUN,
				Noun:  command.BUNDLE,
				File:  "-",
				Path:  "<stdin>",
				Stdin: true,
				As:    "bundle",
			},
			nil,
		},
		{
			"compile a flow",
			[]string{"compile", "-o", "b.bin", "-plugins", "p", "a.json"},
			&command.Asked{
				Verb:    command.COMPILE,
				Noun:    command.FLOW,
				File:    "a.json",
				Path:    "a.json",
				Out:     "b.bin",
				Plugins: "p",
			},
			nil,
		},
		{
			"convert a script",
			[]string{"convert", "a.star"},
			&command.Asked{
				Verb: command.CONVERT,
				Noun: command.SCRIPT,
				File: "a.star",
				Path: "a.star",
			},
			nil,
		},
		{
			"version",
			[]string{"version"},
			&command.Asked{
				Verb: command.VERSION,
			},
			nil,
		},
		{
			"help of a verb",
			[]string{"help", "run"},
			&command.Asked{
				Verb:  command.HELP,
				Topic: "run",
			},
			nil,
		},
		{
			"the flag -h after a verb is the help of the verb",
			[]string{"compile", "-h"},
			&command.Asked{
				Verb:  command.HELP,
				Topic: "compile",
			},
			nil,
		},
		{
			"-h as the first word is the help",
			[]string{"-h"},
			&command.Asked{
				Verb: command.HELP,
			},
			nil,
		},
		{
			"no words",
			nil,
			&command.Asked{},
			command.ErrVerb,
		},
		{
			"a first word that is no verb",
			[]string{"a.star"},
			&command.Asked{},
			nil,
		},
		{
			"two files",
			[]string{"run", "a.star", "b.star"},
			&command.Asked{},
			command.ErrArgs,
		},
		{
			"compile has no -o",
			[]string{"compile", "a.star"},
			&command.Asked{},
			command.ErrOutput,
		},
		{
			"-as beside a file",
			[]string{"convert", "-as", "flow", "a.json"},
			&command.Asked{},
			command.ErrAs,
		},
		{
			"a bundle to convert",
			[]string{"convert", "a.bin"},
			&command.Asked{},
			command.ErrBundle,
		},
		{
			"a flag of another verb",
			[]string{"convert", "-hold", "a.star"},
			&command.Asked{},
			nil,
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := command.Parse(item.args)

				if item.want.Verb == "" {
					if err == nil || (item.fail != nil && !errors.Is(
						err,
						item.fail,
					)) {
						t.Fatalf(
							"got %v, want the refusal %v",
							err,
							item.fail,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				got.Files = nil

				if !reflect.DeepEqual(
					got,
					item.want,
				) {
					t.Fatalf(
						"got %+v, want %+v",
						*got,
						*item.want,
					)
				}
			},
		)
	}
}

// TestHelpOf_PartsOfTheText proves HelpOf cuts the help of a verb out of the
// text by its paragraphs, and answers nothing for a word that is no verb.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
func TestHelpOf_PartsOfTheText(t *testing.T) {
	const TEXT = `
intro

Usage:
  lark run [flags] FILE   runs
  lark version            prints

FILE: a file
more of it

run:
  -x   a flag

Exit codes: 0 ok

See README.md
`

	text := strings.TrimPrefix(
		TEXT,
		"\n",
	)

	for _, item := range []struct {
		verb string
		want string
	}{
		{
			command.RUN,
			"Usage:\n  lark run [flags] FILE   runs\n\n" +
				"FILE: a file\nmore of it\n\nrun:\n  -x   a flag\n\n" +
				"Exit codes: 0 ok\n",
		},
		{
			command.VERSION,
			"Usage:\n  lark version            prints\n\nExit codes: 0 ok\n",
		},
		{
			"zebra",
			"",
		},
	} {
		got := command.HelpOf(
			text,
			item.verb,
		)
		if got != item.want {
			t.Errorf(
				"HelpOf(%q) = %q, want %q",
				item.verb,
				got,
				item.want,
			)
		}
	}
}

// _Roomy is a room check over a reading of the memory that a test chooses: the
// real rule of memwatch (Fit), with Total less Used the free bytes. It counts
// the calls.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
type _Roomy struct {
	free  uint64
	calls int
}

// Room is the check Read is given.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
func (r *_Roomy) Room(
	who string,
	size uint64,
) error {
	r.calls++

	return memwatch.Fit(
		who,
		size,
		func() (*common.MemoryReading, error) {
			return &common.MemoryReading{
				Total: r.free,
			}, nil
		},
	)
}

// _Counted is a reader that gives zeros for ever and counts what it gave.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
type _Counted struct {
	gave int
}

// Read fills p with zeros.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
func (c *_Counted) Read(p []byte) (int, error) {
	clear(p)

	c.gave += len(p)

	return len(p), nil
}

// TestRead_BoundsTheInput proves B253: the script read stops when its buffer
// would not fit the free memory, with an error that wraps memwatch.ErrRoom,
// for standard input and for a file that is no regular file, and a reader that
// never ends is read only as far as the memory allows. A regular file is
// checked once by its size before any byte is read. An input that fits is read
// whole.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253, Q23)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestRead_BoundsTheInput(t *testing.T) {
	const FREE = 8 << 20

	t.Run(
		"standard input that never ends",
		func(t *testing.T) {
			var (
				room = &_Roomy{
					free: FREE,
				}
				zeros = new(_Counted)
			)

			_, err := command.Read(
				&command.Asked{
					Noun:  command.SCRIPT,
					Path:  "<stdin>",
					Stdin: true,
				},
				zeros,
				room.Room,
			)
			if !errors.Is(
				err,
				memwatch.ErrRoom,
			) {
				t.Fatalf(
					"got %v, want %v",
					err,
					memwatch.ErrRoom,
				)
			}

			if zeros.gave > FREE {
				t.Fatalf(
					"read %d bytes of a memory of %d",
					zeros.gave,
					FREE,
				)
			}
		},
	)

	t.Run(
		"a device that never ends",
		func(t *testing.T) {
			_, err := os.Stat("/dev/zero")
			if err != nil {
				t.Skip("the system has no /dev/zero")
			}

			room := &_Roomy{
				free: FREE,
			}

			_, err = command.Read(
				&command.Asked{
					Noun: command.SCRIPT,
					File: "/dev/zero",
					Path: "/dev/zero",
				},
				nil,
				room.Room,
			)
			if !errors.Is(
				err,
				memwatch.ErrRoom,
			) {
				t.Fatalf(
					"got %v, want %v",
					err,
					memwatch.ErrRoom,
				)
			}
		},
	)

	t.Run(
		"a regular file is checked by its size, once",
		func(t *testing.T) {
			var (
				big = filepath.Join(
					t.TempDir(),
					"big.star",
				)
				room = &_Roomy{
					free: FREE,
				}
			)

			made, err := os.Create(big)
			if err != nil {
				t.Fatal(err)
			}

			err = made.Truncate(4 * FREE)
			if err != nil {
				t.Skipf(
					"no sparse file: %v",
					err,
				)
			}

			err = made.Close()
			if err != nil {
				t.Fatal(err)
			}

			_, err = command.Read(
				&command.Asked{
					Noun: command.SCRIPT,
					File: big,
					Path: big,
				},
				nil,
				room.Room,
			)
			if !errors.Is(
				err,
				memwatch.ErrRoom,
			) || room.calls != 1 {
				t.Fatalf(
					"got %v after %d checks, want %v after one",
					err,
					room.calls,
					memwatch.ErrRoom,
				)
			}
		},
	)

	t.Run(
		"an input that fits is read whole",
		func(t *testing.T) {
			const TEXT = "def main():\n    pass\n"

			var (
				small = filepath.Join(
					t.TempDir(),
					"small.star",
				)
				room = &_Roomy{
					free: FREE,
				}
				err = os.WriteFile(
					small,
					[]byte(TEXT),
					SHARED,
				)
			)
			if err != nil {
				t.Fatal(err)
			}

			for name, asked := range map[string]*command.Asked{
				"file": {
					Noun: command.SCRIPT,
					File: small,
					Path: small,
				},
				"stdin": {
					Noun:  command.SCRIPT,
					Path:  "<stdin>",
					Stdin: true,
				},
			} {
				got, err := command.Read(
					asked,
					strings.NewReader(TEXT),
					room.Room,
				)
				if err != nil || string(got) != TEXT {
					t.Fatalf(
						"%s: got %q (%v), want %q",
						name,
						got,
						err,
						TEXT,
					)
				}
			}
		},
	)

	t.Run(
		"a file that is not there keeps the error of the os",
		func(t *testing.T) {
			_, err := command.Read(
				&command.Asked{
					Noun: command.SCRIPT,
					File: "nowhere.star",
					Path: "nowhere.star",
				},
				nil,
				(&_Roomy{
					free: FREE,
				}).Room,
			)
			if !errors.Is(
				err,
				os.ErrNotExist,
			) {
				t.Fatalf(
					"got %v, want %v",
					err,
					os.ErrNotExist,
				)
			}
		},
	)

	t.Run(
		"a reader that fails",
		func(t *testing.T) {
			_, err := command.Read(
				&command.Asked{
					Noun:  command.SCRIPT,
					Path:  "<stdin>",
					Stdin: true,
				},
				&_Failing{},
				(&_Roomy{
					free: FREE,
				}).Room,
			)
			if !errors.Is(
				err,
				io.ErrClosedPipe,
			) {
				t.Fatalf(
					"got %v, want %v",
					err,
					io.ErrClosedPipe,
				)
			}
		},
	)
}

// _Failing is a reader that fails at once.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
type _Failing struct{}

// Read fails with io.ErrClosedPipe.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
func (*_Failing) Read(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

// TestInput_B253RefusesInsteadOfCrashing runs the reproducer of B253 through the
// binary, under ulimit -v 2500000 (about 2.5 GB of address space, which the
// guard counts, Q69; a limit of 1.5 GB ends the binary at start, with or without
// the guard): lark run /dev/zero, and a run on standard input that never
// ends. Each must end with exit 3 and "not enough free memory" on standard
// error, and never with Go's "fatal error: out of memory". A file whose size is
// far above the memory is refused by its size, without a read.
//
// Check: give Read no room check, and the first two fail with the fatal error.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253, Q23)
func TestInput_B253RefusesInsteadOfCrashing(t *testing.T) {
	_, err := os.Stat("/dev/zero")
	if err != nil {
		t.Skip("the system has no /dev/zero")
	}

	var (
		big = filepath.Join(
			t.TempDir(),
			"big.star",
		)
		cases = map[string][]string{
			"a device":       {"run", "/dev/zero"},
			"standard input": {"run", "-"},
		}
	)

	for name, args := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var stderr strings.Builder

				cmd := exec.CommandContext(
					t.Context(),
					"sh",
					append(
						[]string{
							"-c",
							LIMITED,
							BUILT,
						},
						args...,
					)...,
				)
				cmd.Stderr = &stderr
				cmd.WaitDelay = KILL_WAIT

				_Grouped(cmd)

				code := _ExitOf(
					t,
					cmd.Run(),
				)
				if code != EXIT_NO_FILE || !strings.Contains(
					stderr.String(),
					"not enough free memory",
				) || strings.Contains(
					stderr.String(),
					"fatal error",
				) {
					t.Fatalf(
						"exit %d, want %d, the refusal, no crash\n%.400s",
						code,
						EXIT_NO_FILE,
						stderr.String(),
					)
				}
			},
		)
	}

	made, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}

	err = made.Truncate(1 << 43)
	if err != nil {
		t.Skipf(
			"no sparse file: %v",
			err,
		)
	}

	err = made.Close()
	if err != nil {
		t.Fatal(err)
	}

	code, said := _Run(
		t,
		"run",
		big,
	)
	if code != EXIT_NO_FILE || !strings.Contains(
		said,
		"not enough free memory",
	) {
		t.Fatalf(
			"a file of 8 TiB: exit %d, stderr %.400q, want %d with the refusal",
			code,
			said,
			EXIT_NO_FILE,
		)
	}
}

// _ExitOf is the exit code in err, or the failure of the test when err is not
// the end of a command.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
func _ExitOf(
	t *testing.T,
	err error,
) int {
	t.Helper()

	if err == nil {
		return 0
	}

	var exit *exec.ExitError
	if !errors.As(
		err,
		&exit,
	) {
		t.Fatal(err)
	}

	return exit.ExitCode()
}
