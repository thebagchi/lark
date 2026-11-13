package tests

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	// TABLE_SOURCE is this file, which holds the recorded table as GOLDEN.
	// A test runs in the directory of its package, so the bare name finds it.
	// TABLE_UPDATE is the variable that rewrites the table, and TABLE_TIMEOUT
	// how many seconds a case may run.
	//
	// Revisions:
	//   - 2026-11-04 06:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:34: the table is GOLDEN in this file, not a file (C114, Q282)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_SOURCE  = "table_test.go"
	TABLE_UPDATE  = "LARK_TABLE_UPDATE"
	TABLE_TIMEOUT = 120

	// TABLE_ARGS is a JSON object a case supplies as arguments.
	//
	// Revisions:
	//   - 2026-11-04 06:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_ARGS = `{"host": "db", "port": 1}`

	// TABLE_READY is the line a script prints before it sleeps, so a case
	// knows when to send its signal.
	//
	// Revisions:
	//   - 2026-11-04 06:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_READY = "ready"

	// TABLE_HEAD opens the block of one case in the output of a case.
	//
	// Revisions:
	//   - 2026-11-04 06:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 17:05: the head names no file (C114, Q281)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_HEAD = "=== "
)

var (
	// TABLE_TIME matches the time a log line opens with, and TABLE_SPOOL the
	// name of a temporary spool file.
	//
	// Revisions:
	//   - 2026-11-04 06:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_TIME  = regexp.MustCompile(`time=\S+`)
	TABLE_SPOOL = regexp.MustCompile(`lark-[0-9]+\.log`)
	TABLE_TEMP  = regexp.MustCompile(`\.[0-9a-f]{12}\.tmp`)

	// TABLE_VERSION matches the -version line, whose version and Go version
	// differ by checkout and toolchain.
	//
	// Revisions:
	//   - 2026-11-04 23:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	TABLE_VERSION = regexp.MustCompile(`(?m)^lark \S+ go\S+$`)
)

// _TableCase is one command line of the table. The case runs in a new
// directory that holds the fixtures, with relative paths, so its output does
// not depend on the machine.
//
// show lists the files, relative to the directory, whose text the block prints
// after the run. signal, when set, is sent once the script has printed
// TABLE_READY. closed runs the command with a standard output whose reader is
// gone. stdin, when set, is the text on standard input.
//
// Revisions:
//   - 2026-11-04 06:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _TableCase struct {
	name   string
	args   []string
	show   []string
	stdin  string
	signal os.Signal
	closed bool
}

// TestCLI_TableIsUnchanged proves the command answers each command line below
// with the same standard output, standard error, exit code and files as the
// recorded table GOLDEN, at the end of this file. The table was recorded from
// the command before the C57 refactor. Set LARK_TABLE_UPDATE=1 to record it
// again: the test then rewrites GOLDEN in place.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-09 11:34: the table is GOLDEN in this file (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCLI_TableIsUnchanged(t *testing.T) {
	var (
		cases  = _TableCases()
		golden = _TableRead()
		blocks = make(
			map[string]string,
			len(cases),
		)
		order = make(
			[]string,
			0,
			len(cases),
		)
	)

	for _, item := range cases {
		order = append(
			order,
			item.name,
		)

		t.Run(
			item.name,
			func(t *testing.T) {
				got := _TableBlock(
					t,
					&item,
				)

				blocks[item.name] = got

				if os.Getenv(TABLE_UPDATE) != "" {
					return
				}

				if golden[item.name] != got {
					t.Fatalf(
						"block differs\nwant:\n%s\ngot:\n%s",
						golden[item.name],
						got,
					)
				}
			},
		)
	}

	if os.Getenv(TABLE_UPDATE) != "" {
		_TableWrite(
			t,
			order,
			blocks,
		)

		return
	}

	for name := range golden {
		_, ran := blocks[name]
		if !ran {
			t.Errorf(
				"recorded case %q was not run",
				name,
			)
		}
	}
}

// _TableCases is the command lines of the table: each verb with each flag that
// belongs to it, and each refusal and exit code from 0 to 5 that a test can
// reach without memory pressure (5 is TestFinish and TestAbandoning).
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: the command lines are the verbs of the new command line; the cases that
//     keep a name keep their meaning (C119, Q305)
//   - 2026-11-11 00:48: the case of -plugins [x is named for a directory with a bracket
//     (C122, B265)
//   - 2026-11-13 10:16: a case for a flow of plain values (C132, Q422)
//   - 2026-11-13 20:19: cases for a flow that states nothing (C133, B296, Q453)
func _TableCases() []_TableCase {
	return []_TableCase{
		{
			name: "run script",
			args: []string{"run", "hi.star"},
		},
		{
			name: "positional script",
			args: []string{"hi.star"},
		},
		{
			name: "positional script then a flag",
			args: []string{"run", "hi.star", "-hold"},
		},
		{
			name: "positional script between flags",
			args: []string{"run", "-logs", "logs", "hi.star", "-hold"},
			show: []string{"logs/hi.star.log"},
		},
		{
			name: "positional script after flags",
			args: []string{"convert", "hi.star"},
		},
		{
			name: "positional script missing",
			args: []string{"run", "nope.star"},
		},
		{
			name: "positional script fails",
			args: []string{"run", "fail.star"},
		},
		{
			name: "positional and script",
			args: []string{"run", "hi.star", "hi.star"},
		},
		{
			name: "positional and flow",
			args: []string{"run", "hi.star", "hi.json"},
		},
		{
			name: "positional and run",
			args: []string{"run", "hi.star", "hi.bin"},
		},
		{
			name: "two positionals",
			args: []string{"run", "hi.star", "fail.star"},
		},
		{
			name: "two positionals around a flag",
			args: []string{"run", "hi.star", "-hold", "fail.star"},
		},
		{
			name: "positional after the end of flags",
			args: []string{"run", "--", "hi.star"},
		},
		{
			name:  "script from stdin",
			args:  []string{"run", "-"},
			stdin: "def main():\n    print(\"in\")\n",
		},
		{
			name:  "script from stdin by -as",
			args:  []string{"run", "-as", "script", "-"},
			stdin: "def main():\n    print(\"in\")\n",
		},
		{
			name:  "script from stdin fails",
			args:  []string{"run", "-"},
			stdin: "def main():\n    fail(\"boom\")\n",
		},
		{
			name:  "script from stdin does not compile",
			args:  []string{"run", "-"},
			stdin: "def main(:\n",
		},
		{
			name:  "script from stdin translated",
			args:  []string{"convert", "-"},
			stdin: "def main():\n    print(\"in\")\n",
		},
		{
			name:  "script from stdin with logs",
			args:  []string{"run", "-logs", "logs", "-"},
			stdin: "def main():\n    print(\"in\")\n",
			show:  []string{"logs/stdin.log"},
		},
		{
			name: "script from empty stdin",
			args: []string{"run", "-"},
		},
		{
			name:  "flow dash is a file",
			args:  []string{"run", "-as", "flow", "-"},
			stdin: "{}",
		},
		{
			name:  "flow from stdin converted",
			args:  []string{"convert", "-as", "flow", "-"},
			stdin: "{}",
		},
		{
			name:  "bundle from stdin",
			args:  []string{"run", "-as", "bundle", "-"},
			stdin: "not a bundle",
		},
		{
			name:  "bundle from stdin compiled",
			args:  []string{"compile", "-o", "out.bin", "-as", "bundle", "-"},
			stdin: "not a bundle",
		},
		{
			name:  "script from stdin compiled",
			args:  []string{"compile", "-o", "out.bin", "-"},
			stdin: "def main():\n    print(\"in\")\n",
		},
		{
			name: "as of a kind that is none",
			args: []string{"run", "-as", "yaml", "-"},
		},
		{
			name: "as beside a file",
			args: []string{"run", "-as", "flow", "hi.json"},
		},
		{
			name: "version",
			args: []string{"version"},
		},
		{
			name: "version beside a script",
			args: []string{"version", "hi.star"},
		},
		{
			name: "version as a flag",
			args: []string{"-version"},
		},
		{
			name: "run script double dash",
			args: []string{"run", "--logs", "logs", "hi.star"},
			show: []string{"logs/hi.star.log"},
		},
		{
			name: "translate script",
			args: []string{"convert", "hi.star"},
		},
		{
			name: "run flow",
			args: []string{"run", "hi.json"},
		},
		{
			name: "translate flow",
			args: []string{"convert", "hi.json"},
		},
		{
			name: "logs",
			args: []string{"run", "-logs", "logs", "hi.star"},
			show: []string{"logs/hi.star.log"},
		},
		{
			name: "logs in a nested directory",
			args: []string{"run", "-logs", "a/b/logs", "hi.star"},
			show: []string{"a/b/logs/hi.star.log"},
		},
		{
			name: "logs of a flow",
			args: []string{"run", "-logs", "logs", "hi.json"},
			show: []string{"logs/hi.json.log"},
		},
		{
			name: "bundle",
			args: []string{"compile", "-o", "out.bin", "hi.star"},
		},
		{
			name: "bundle of a flow",
			args: []string{"compile", "-o", "out.bin", "hi.json"},
		},
		{
			name: "help",
			args: []string{"-h"},
		},
		{
			name: "help double dash",
			args: []string{"--help"},
		},
		{
			name: "help verb",
			args: []string{"help"},
		},
		{
			name: "help of run",
			args: []string{"help", "run"},
		},
		{
			name: "help of compile",
			args: []string{"help", "compile"},
		},
		{
			name: "help of convert",
			args: []string{"help", "convert"},
		},
		{
			name: "help of version",
			args: []string{"help", "version"},
		},
		{
			name: "help of help",
			args: []string{"help", "help"},
		},
		{
			name: "help of a word that is no verb",
			args: []string{"help", "zebra"},
		},
		{
			name: "help of two verbs",
			args: []string{"help", "run", "compile"},
		},
		{
			name: "flag -h after a verb",
			args: []string{"run", "-h"},
		},
		{
			name: "one letter script",
			args: []string{"-s", "hi.star"},
		},
		{
			name: "one letter translate",
			args: []string{"run", "-t", "hi.star"},
		},
		{
			name: "one letter logs",
			args: []string{"run", "-l", "logs", "hi.star"},
		},
		{
			name: "one letter bundle",
			args: []string{"run", "-b", "out.bin", "hi.star"},
		},
		{
			name: "one letter args",
			args: []string{"run", "-a", "{}", "hi.star"},
		},
		{
			name: "one letter plugins",
			args: []string{"run", "-p", "plugins", "hi.star"},
		},
		{
			name: "gone memory ceiling",
			args: []string{"run", "-m", "64", "hi.star"},
		},
		{
			name: "gone flag script",
			args: []string{"run", "-script", "hi.star"},
		},
		{
			name: "gone flag translate",
			args: []string{"run", "-translate", "hi.star"},
		},
		{
			name: "gone flag bundle",
			args: []string{"run", "-bundle", "out.bin", "hi.star"},
		},
		{
			name: "gone flag changes",
			args: []string{"run", "-changes", "c.jsonl", "hi.star"},
		},
		{
			name: "unknown flag",
			args: []string{"run", "hi.star", "-nope"},
		},
		{
			name: "string flag without a value",
			args: []string{"run", "-logs"},
		},
		{
			name: "bool flag with a bad value",
			args: []string{"run", "hi.star", "-hold=maybe"},
		},
		{
			name: "stray argument",
			args: []string{"run", "hi.star", "extra"},
		},
		{
			name: "unknown verb",
			args: []string{"rn", "hi.star"},
		},
		{
			name: "unknown verb nothing close",
			args: []string{"zebra", "hi.star"},
		},
		{
			name: "args default",
			args: []string{"run", "args.star"},
		},
		{
			name: "args given",
			args: []string{"run", "-args", TABLE_ARGS, "args.star"},
		},
		{
			name: "args empty object",
			args: []string{"run", "-args", "{}", "hi.star"},
		},
		{
			name: "args not json",
			args: []string{"run", "-args", "{nope", "hi.star"},
		},
		{
			name: "args an array",
			args: []string{"run", "-args", "[1]", "hi.star"},
		},
		{
			name: "plugins empty",
			args: []string{"run", "-plugins", "plugins", "hi.star"},
		},
		{
			name: "plugins directory with a bracket",
			args: []string{"run", "-plugins", "[x", "hi.star"},
		},
		{
			name: "plugins of compile",
			args: _Words("compile -plugins plugins -o out.bin hi.star"),
		},
		{
			name: "plugins of convert",
			args: []string{"convert", "-plugins", "plugins", "hi.star"},
		},
		{
			name: "graph",
			args: []string{"run", "-graph", "end.jsonl", "hi.star"},
			show: []string{"end.jsonl"},
		},
		{
			name: "graph of a failed run",
			args: []string{"run", "-graph", "end.jsonl", "fail.star"},
			show: []string{"end.jsonl"},
		},
		{
			name: "graph to standard output",
			args: []string{"run", "-graph", "-", "hi.star"},
		},
		{
			name: "graph to standard output of a failed run",
			args: []string{"run", "-graph", "-", "fail.star"},
		},
		{
			name: "held lines and graph to standard output",
			args: []string{"run", "-hold", "-graph", "-", "hi.star"},
		},
		{
			name: "failure in a nested function names its cause",
			args: []string{"run", "nested.star"},
		},
		{
			name: "graph not written when the script does not compile",
			args: []string{"run", "-graph", "end.jsonl", "bad.star"},
			show: []string{"end.jsonl"},
		},
		{
			name: "graph replaces a file",
			args: []string{"run", "-graph", "old.json", "hi.star"},
			show: []string{"old.json"},
		},
		{
			name: "graph in a missing directory",
			args: []string{"run", "-graph", "nodir/end.jsonl", "hi.star"},
		},
		{
			name: "graph over a directory",
			args: []string{"run", "-graph", "plugins", "hi.star"},
		},
		{
			name: "hold",
			args: []string{"run", "-hold", "hi.star"},
		},
		{
			name: "hold with logs",
			args: []string{"run", "-hold", "-logs", "logs", "hi.star"},
			show: []string{"logs/hi.star.log"},
		},
		{
			name: "hold with graph",
			args: []string{"run", "-hold", "-graph", "end.jsonl", "hi.star"},
			show: []string{"end.jsonl"},
		},
		{
			name: "hold a failed script",
			args: []string{"run", "-hold", "fail.star"},
		},
		{
			name: "memory on a short run",
			args: []string{"run", "-memory", "hi.star"},
		},
		{
			name: "memory with graph",
			args: []string{"run", "-memory", "-graph", "end.jsonl", "hi.star"},
			show: []string{"end.jsonl"},
		},
		{
			name: "failed script",
			args: []string{"run", "fail.star"},
		},
		{
			name: "failed flow compile",
			args: []string{"run", "junk.json"},
		},
		{
			name: "failed flow of plain values",
			args: []string{"run", "plain.json"},
		},
		{
			name: "failed flow with a constant that states nothing",
			args: []string{"run", "emptyconst.json"},
		},
		{
			name: "failed translate of a flow with an argument that has no name",
			args: []string{"convert", "emptyarg.json"},
		},
		{
			name: "failed flow with an operand that has no source",
			args: []string{"run", "emptyoperand.json"},
		},
		{
			name: "failed translate of a script",
			args: []string{"convert", "bad.star"},
		},
		{
			name: "failed translate of a flow",
			args: []string{"convert", "junk.json"},
		},
		{
			name: "failed bundle",
			args: []string{"compile", "-o", "out.bin", "bad.star"},
		},
		{
			name: "script does not compile",
			args: []string{"run", "bad.star"},
		},
		{
			name: "bundle is garbage",
			args: []string{"run", "junk.bin"},
		},
		{
			name: "script file missing",
			args: []string{"run", "none.star"},
		},
		{
			name: "flow file missing",
			args: []string{"run", "none.json"},
		},
		{
			name: "bundle file missing",
			args: []string{"run", "none.bin"},
		},
		{
			name: "script file is a directory",
			args: []string{"run", "plugins"},
		},
		{
			name: "logs path is a file",
			args: []string{"run", "-logs", "hi.star", "hi.star"},
		},
		{
			name: "bundle path is a directory",
			args: []string{"compile", "-o", "plugins", "hi.star"},
		},
		{
			name: "bundle path in a missing directory",
			args: []string{"compile", "-o", "nodir/out.bin", "hi.star"},
		},
		{
			name: "no verb",
			args: nil,
		},
		{
			name: "verb without input",
			args: []string{"run"},
		},
		{
			name: "compile without a bundle path",
			args: []string{"compile", "hi.star"},
		},
		{
			name: "compile without input",
			args: []string{"compile", "-o", "out.bin"},
		},
		{
			name: "convert without input",
			args: []string{"convert"},
		},
		{
			name: "run with a bundle",
			args: []string{"run", "-graph", "end.jsonl", "hi.bin"},
			show: []string{"end.jsonl"},
		},
		{
			name: "convert a bundle",
			args: []string{"convert", "hi.bin"},
		},
		{
			name: "compile a bundle",
			args: []string{"compile", "-o", "out.bin", "hi.bin"},
		},
		{
			name: "graph with convert",
			args: []string{"convert", "-graph", "end.jsonl", "hi.star"},
		},
		{
			name: "graph with compile",
			args: _Words("compile -o out.bin -graph end.jsonl hi.star"),
		},
		{
			name: "hold with convert",
			args: []string{"convert", "-hold", "hi.star"},
		},
		{
			name: "hold with compile",
			args: []string{"compile", "-o", "out.bin", "-hold", "hi.star"},
		},
		{
			name:   "stopped by terminate",
			args:   _Words("run -graph end.jsonl -logs logs sleepy.star"),
			show:   []string{"end.jsonl", "logs/sleepy.star.log"},
			signal: syscall.SIGTERM,
		},
		{
			name:   "stopped by interrupt",
			args:   []string{"run", "sleepy.star"},
			signal: syscall.SIGINT,
		},
		{
			name:   "stopped by hangup",
			args:   _Words("run -graph end.jsonl -logs logs sleepy.star"),
			show:   []string{"end.jsonl", "logs/sleepy.star.log"},
			signal: syscall.SIGHUP,
		},
		{
			name:   "stopped flow",
			args:   []string{"run", "sleepy.json"},
			signal: syscall.SIGTERM,
		},
		{
			name:   "closed output ends by the signal",
			args:   []string{"run", "hi.star"},
			closed: true,
		},
		{
			name:   "closed output with logs is a stop",
			args:   []string{"run", "-logs", "logs", "hi.star"},
			show:   []string{"logs/hi.star.log"},
			closed: true,
		},
		{
			name:   "closed output while held is a stop",
			args:   []string{"run", "-hold", "hi.star"},
			closed: true,
		},
	}
}

// _Words is the arguments in text, split at spaces, for a command line with no
// space inside an argument.
//
// Revisions:
//   - 2026-11-04 00:04: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Words(text string) []string {
	return strings.Fields(text)
}

// _TableBlock runs one case in a new directory and returns its block of the
// golden file: the command line, the exit code, the two streams, the files the
// run made and the text of the files the case shows.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableBlock(
	t *testing.T,
	item *_TableCase,
) string {
	t.Helper()

	var (
		dir    = t.TempDir()
		before = _TableFixtures(
			t,
			dir,
		)
	)

	stdout, stderr, code := _TableRun(
		t,
		dir,
		item,
	)

	var out strings.Builder

	out.WriteString(TABLE_HEAD + item.name + "\n")
	out.WriteString("args: " + strconv.Quote(strings.Join(
		item.args,
		" ",
	)) + "\n")
	out.WriteString("exit: " + code + "\n")
	out.WriteString("--- stdout\n" + _TableClean(
		stdout,
		dir,
	))
	out.WriteString("--- stderr\n" + _TableClean(
		stderr,
		dir,
	))
	out.WriteString("--- new files\n")

	for _, name := range _TableNew(
		t,
		dir,
		before,
	) {
		out.WriteString(_TableClean(
			name,
			dir,
		) + "\n")
	}

	for _, name := range item.show {
		raw, err := os.ReadFile(filepath.Join(
			dir,
			name,
		))
		if err != nil {
			out.WriteString("--- " + name + "\nmissing\n")

			continue
		}

		out.WriteString("--- " + name + "\n" + _TableClean(
			string(raw),
			dir,
		))
	}

	return out.String()
}

// _TableRun starts the command for item in dir and returns its two streams
// and its exit code, or the signal text when a signal ended it. TMPDIR is
// dir/tmp, so a spool file is found there and nowhere else.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableRun(
	t *testing.T,
	dir string,
	item *_TableCase,
) (string, string, string) {
	t.Helper()

	var (
		ctx, cancel = context.WithTimeout(
			t.Context(),
			TABLE_TIMEOUT*time.Second,
		)
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			item.args...,
		)
		stderr strings.Builder
	)

	defer cancel()

	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"TMPDIR="+filepath.Join(
			dir,
			"tmp",
		),
	)
	cmd.Stderr = &stderr
	cmd.WaitDelay = KILL_WAIT

	_Grouped(cmd)

	var (
		stdout string
		err    error
	)

	switch {
	case item.signal != nil:
		stdout, err = _TableSignalled(
			t,
			cmd,
			item.signal,
		)

	case item.closed:
		err = _TableClosed(
			t,
			cmd,
		)

	default:
		var buffer strings.Builder

		if item.stdin != "" {
			cmd.Stdin = strings.NewReader(item.stdin)
		}

		cmd.Stdout = &buffer
		err = cmd.Run()
		stdout = buffer.String()
	}

	return stdout, stderr.String(), _TableCode(
		t,
		err,
	)
}

// _TableSignalled runs cmd, sends sig when its standard output has shown
// TABLE_READY, and returns what it printed.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableSignalled(
	t *testing.T,
	cmd *exec.Cmd,
	sig os.Signal,
) (string, error) {
	t.Helper()

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	var (
		reader = bufio.NewReader(pipe)
		seen   strings.Builder
	)

	for {
		line, err := reader.ReadString('\n')
		seen.WriteString(line)

		if strings.Contains(
			line,
			TABLE_READY,
		) {
			break
		}

		if err != nil {
			t.Fatalf(
				"no %q line: %v",
				TABLE_READY,
				err,
			)
		}
	}

	err = cmd.Process.Signal(sig)
	if err != nil {
		t.Fatal(err)
	}

	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	seen.Write(rest)

	return seen.String(), cmd.Wait()
}

// _TableClosed runs cmd with a standard output whose reader is already gone.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableClosed(
	t *testing.T,
	cmd *exec.Cmd,
) error {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	err = reader.Close()
	if err != nil {
		t.Fatal(err)
	}

	cmd.Stdout = writer

	err = cmd.Run()

	closed := writer.Close()
	if closed != nil {
		t.Fatal(closed)
	}

	return err
}

// _TableCode is the exit code of a run as text, or the text of the signal that
// ended it.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableCode(
	t *testing.T,
	err error,
) string {
	t.Helper()

	var exit *exec.ExitError

	switch {
	case err == nil:
		return "0"

	case errors.As(
		err,
		&exit,
	):
		if exit.ExitCode() < 0 {
			return exit.String()
		}

		return strconv.Itoa(exit.ExitCode())

	default:
		t.Fatal(err)

		return ""
	}
}

// _TableFixtures writes the files every case may name into dir and returns
// their names.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: makes the fixtures with the convert and compile verbs (C119, Q305,
//     Q306)
//   - 2026-11-13 10:16: the fixture plain.json, a flow of plain values (C132, Q422)
//   - 2026-11-13 20:19: the fixtures of a flow that states nothing (C133, B296, Q453)
func _TableFixtures(
	t *testing.T,
	dir string,
) map[string]bool {
	t.Helper()

	args, err := os.ReadFile(filepath.Join(
		"..",
		"..",
		"..",
		"samples",
		"args.star",
	))
	if err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"hi.star":     "def main():\n    print(\"hi\")\n    print(\"two\")\n",
		"fail.star":   "def main():\n    print(\"before\")\n    fail(\"boom\")\n",
		"bad.star":    "def main(:\n",
		"nested.star": "def deep():\n    fail(\"boom\")\n\ndef main():\n    deep()\n",
		"sleepy.star": "def main():\n    print(\"" + TABLE_READY +
			"\")\n    sleep(60000)\n",
		"junk.json": "not json\n",
		"plain.json": `{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], ` +
			`"main": {"statement": [{"call": {"function": "f", ` +
			`"args": ["west"]}}]}}` + "\n",
		"emptyconst.json": `{"constants": {"A": {}}, "text": "pass"}` + "\n",
		"emptyarg.json":   `{"args": {"a": {}}, "text": "pass"}` + "\n",
		"emptyoperand.json": `{"functions": [{"name": "f", "params": ["p"], ` +
			`"body": "pass"}], "main": {"statement": [{"call": {"function": "f", ` +
			`"operands": [{}]}}]}}` + "\n",
		"junk.bin":  "not a bundle\n",
		"old.json":  "old graph\n",
		"args.star": string(args),
	}

	for name, text := range files {
		err = os.WriteFile(
			filepath.Join(
				dir,
				name,
			),
			[]byte(text),
			SHARED,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"plugins", "tmp"} {
		err = os.Mkdir(
			filepath.Join(
				dir,
				name,
			),
			PRIVATE,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, made := range []struct {
		out  string
		args []string
	}{
		{
			"hi.json",
			[]string{"convert", "hi.star"},
		},
		{
			"sleepy.json",
			[]string{"convert", "sleepy.star"},
		},
		{
			"",
			[]string{"compile", "-o", "hi.bin", "hi.star"},
		},
	} {
		cmd := exec.Command(
			BUILT,
			made.args...,
		)
		cmd.Dir = dir

		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}

		if made.out == "" {
			continue
		}

		err = os.WriteFile(
			filepath.Join(
				dir,
				made.out,
			),
			out,
			SHARED,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	return _TableListed(
		t,
		dir,
	)
}

// _TableListed is every path under dir, relative to it.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableListed(
	t *testing.T,
	dir string,
) map[string]bool {
	t.Helper()

	var (
		listed = make(map[string]bool)
		err    error
	)

	err = filepath.WalkDir(
		dir,
		func(
			path string,
			entry fs.DirEntry,
			err error,
		) error {
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(
				dir,
				path,
			)
			if err != nil {
				return err
			}

			listed[rel] = true

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return listed
}

// _TableNew is the paths in dir that are not in before, sorted and cleaned.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableNew(
	t *testing.T,
	dir string,
	before map[string]bool,
) []string {
	t.Helper()

	var made []string

	for name := range _TableListed(
		t,
		dir,
	) {
		if !before[name] {
			made = append(
				made,
				name,
			)
		}
	}

	sort.Strings(made)

	return made
}

// _TableClean takes out of text what differs between runs: the directory, the
// no-break space that protobuf puts at random in its messages, the
// time of a log line, the name of a spool file.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-04 00:10: turns a no-break space into a space, as protobuf-go adds one at random
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableClean(
	text string,
	dir string,
) string {
	text = strings.ReplaceAll(
		text,
		dir,
		"<dir>",
	)
	text = strings.ReplaceAll(
		text,
		"\u00a0",
		" ",
	)
	text = TABLE_TIME.ReplaceAllString(
		text,
		"time=<time>",
	)
	text = TABLE_SPOOL.ReplaceAllString(
		text,
		"lark-<n>.log",
	)

	text = TABLE_TEMP.ReplaceAllString(
		text,
		".<random>.tmp",
	)

	text = TABLE_VERSION.ReplaceAllString(
		text,
		"lark <version> <go>",
	)

	return text
}

// _TableRead is the recorded blocks, by case name. It returns none when the
// table is being recorded again.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-09 11:34: reads the GOLDEN table, not a file (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableRead() map[string]string {
	blocks := make(map[string]string)

	if os.Getenv(TABLE_UPDATE) != "" {
		return blocks
	}

	for _, row := range GOLDEN {
		blocks[row.name] = TABLE_HEAD + row.name + "\n" + strings.TrimPrefix(
			row.block,
			"\n",
		)
	}

	return blocks
}

// _TableWrite records the blocks in the order of the cases, as the GOLDEN
// table in TABLE_SOURCE. It changes only the text from the var line of the
// table to the END marker: the doc comment above the var and every byte
// outside the two markers stay as they are. It formats the new table with
// go/format. A row holds the block without its head line, which the name
// restates.
//
// Revisions:
//   - 2026-11-03 23:50: initial creation (C57)
//   - 2026-11-09 20:19: writes the GOLDEN table in the source, four-line rows (C114, Q282, Q293)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableWrite(
	t *testing.T,
	order []string,
	blocks map[string]string,
) {
	t.Helper()

	var table strings.Builder

	table.WriteString("var (\n\tGOLDEN = []struct{ name, block string }{\n")

	for _, name := range order {
		fmt.Fprintf(
			&table,
			"{\n%s,\n%s,\n},\n",
			_TableLiteral(name),
			_TableBlockLiteral(strings.TrimPrefix(
				blocks[name],
				TABLE_HEAD+name+"\n",
			)),
		)
	}

	table.WriteString("\t}\n)\n")

	formatted, err := format.Source([]byte(table.String()))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(TABLE_SOURCE)
	if err != nil {
		t.Fatal(err)
	}

	// The last markers in the file are the real ones; the strings above hold
	// the same text.
	var (
		source = string(raw)
		begin  = strings.LastIndex(
			source,
			"\n// BEGIN GOLDEN:",
		)
		end = strings.LastIndex(
			source,
			"\n// END GOLDEN",
		)
	)

	if begin < 0 || end < begin {
		t.Fatalf(
			"%s has no GOLDEN markers",
			TABLE_SOURCE,
		)
	}

	start := strings.Index(
		source[begin:end],
		"\nvar (\n\tGOLDEN = ",
	)
	if start < 0 {
		t.Fatalf(
			"%s has no GOLDEN table",
			TABLE_SOURCE,
		)
	}

	start += begin + 1

	err = os.WriteFile(
		TABLE_SOURCE,
		[]byte(source[:start]+string(formatted)+"\n"+source[end+1:]),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}
}

// _TableRaw says whether text can stand in a Go raw string unchanged: it holds
// no backtick, no carriage return (the compiler drops it from a raw string),
// no byte-order mark, no invalid UTF-8 and no control character but a tab and
// a newline.
//
// Revisions:
//   - 2026-11-09 11:34: initial creation (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableRaw(text string) bool {
	if !utf8.ValidString(text) {
		return false
	}

	for _, r := range text {
		if r == '`' || r == '\r' || r == '\uFEFF' || r == 0x7f ||
			(r < ' ' && r != '\t' && r != '\n') {
			return false
		}
	}

	return true
}

// _TableBlockLiteral is the Go source of one block. A raw block opens with a
// newline after the backtick and ends with its own last newline, so the closing
// backtick starts a line; the writer puts a comma after it and the row's brace
// on the next line. _TableRead drops the opening newline.
//
// Revisions:
//   - 2026-11-09 17:05: initial creation (C114, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableBlockLiteral(text string) string {
	if text != "" && strings.HasSuffix(
		text,
		"\n",
	) && _TableRaw(text) {
		return "`\n" + text + "`"
	}

	return _TableLiteral(text)
}

// _TableLiteral is the Go source of text: a raw string when _TableRaw allows,
// else a quoted string.
//
// Revisions:
//   - 2026-11-09 11:34: initial creation (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _TableLiteral(text string) string {
	if _TableRaw(text) {
		return "`" + text + "`"
	}

	return strconv.Quote(text)
}

// BEGIN GOLDEN: written by LARK_TABLE_UPDATE=1 go test; edit by hand only to review.

// GOLDEN is the recorded output of every case of the table, in the order of
// _TableCases: the name of the case, and its block without the head line
// (TABLE_HEAD and the name). The block holds the command line, the exit code,
// the two streams and the files the case made.
//
// Revisions:
//   - 2026-11-04 06:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-09 20:19: the table replaces testdata/cli_table.golden: a var group under one nolint:lll, a row of four lines (C114, Q282, Q289, Q290, Q293)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-11 00:48: the plugins directory [x is a directory that is missing, not a pattern (C122, B265)
//   - 2026-11-13 13:04: a row for a flow of plain values (C132, Q422)
//   - 2026-11-13 19:28: two rows name <stdin> before a Check refusal, three rows for a flow that
//     states nothing (C133, B296, Q448, Q453)
//
//nolint:lll // a golden row
var (
	GOLDEN = []struct{ name, block string }{
		{
			`run script`,
			`
args: "run hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`positional script`,
			`
args: "hi.star"
exit: 2
--- stdout
--- stderr
lark: unknown verb "hi.star"
try lark help
--- new files
`,
		},
		{
			`positional script then a flag`,
			`
args: "run hi.star -hold"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
--- new files
`,
		},
		{
			`positional script between flags`,
			`
args: "run -logs logs hi.star -hold"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
logs
logs/hi.star.log
--- logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`positional script after flags`,
			`
args: "convert hi.star"
exit: 0
--- stdout
{
  "text": "print(\"hi\")\nprint(\"two\")"
}
--- stderr
--- new files
`,
		},
		{
			`positional script missing`,
			`
args: "run nope.star"
exit: 3
--- stdout
--- stderr
lark: open nope.star: no such file or directory
--- new files
`,
		},
		{
			`positional script fails`,
			`
args: "run fail.star"
exit: 1
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=before
--- stderr
script failed: fail.star:3: fail: boom
--- new files
`,
		},
		{
			`positional and script`,
			`
args: "run hi.star hi.star"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`positional and flow`,
			`
args: "run hi.star hi.json"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`positional and run`,
			`
args: "run hi.star hi.bin"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`two positionals`,
			`
args: "run hi.star fail.star"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`two positionals around a flag`,
			`
args: "run hi.star -hold fail.star"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`positional after the end of flags`,
			`
args: "run -- hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`script from stdin`,
			`
args: "run -"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=in
--- stderr
--- new files
`,
		},
		{
			`script from stdin by -as`,
			`
args: "run -as script -"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=in
--- stderr
--- new files
`,
		},
		{
			`script from stdin fails`,
			`
args: "run -"
exit: 1
--- stdout
--- stderr
script failed: <stdin>:2: fail: boom
--- new files
`,
		},
		{
			`script from stdin does not compile`,
			`
args: "run -"
exit: 1
--- stdout
--- stderr
script failed: <stdin>:1:11: got ':', want ')'
--- new files
`,
		},
		{
			`script from stdin translated`,
			`
args: "convert -"
exit: 0
--- stdout
{
  "text": "print(\"in\")"
}
--- stderr
--- new files
`,
		},
		{
			`script from stdin with logs`,
			`
args: "run -logs logs -"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=in
--- stderr
--- new files
logs
logs/stdin.log
--- logs/stdin.log
time=<time> level=INFO thread=thread_0 function=main msg=in
`,
		},
		{
			`script from empty stdin`,
			`
args: "run -"
exit: 1
--- stdout
--- stderr
script failed: <stdin>: no entry point (define def main() with no required parameters; a file without it is a module, for load)
--- new files
`,
		},
		{
			`flow dash is a file`,
			`
args: "run -as flow -"
exit: 1
--- stdout
--- stderr
flow failed: <stdin>: main: function has no body: no form
--- new files
`,
		},
		{
			`flow from stdin converted`,
			`
args: "convert -as flow -"
exit: 1
--- stdout
--- stderr
flow failed: <stdin>: main: function has no body: no form
--- new files
`,
		},
		{
			`bundle from stdin`,
			`
args: "run -as bundle -"
exit: 1
--- stdout
--- stderr
bundle failed: <stdin>: unmarshal bundle: proto: cannot parse invalid wire-format data
--- new files
`,
		},
		{
			`bundle from stdin compiled`,
			`
args: "compile -o out.bin -as bundle -"
exit: 2
--- stdout
--- stderr
lark: a bundle is only run (compile and convert need a script or a flow)
try lark help
--- new files
`,
		},
		{
			`script from stdin compiled`,
			`
args: "compile -o out.bin -"
exit: 0
--- stdout
--- stderr
lark: wrote "out.bin"
--- new files
out.bin
`,
		},
		{
			`as of a kind that is none`,
			`
args: "run -as yaml -"
exit: 2
--- stdout
--- stderr
lark: wrong -as (it is script, flow or bundle)
try lark help
--- new files
`,
		},
		{
			`as beside a file`,
			`
args: "run -as flow hi.json"
exit: 2
--- stdout
--- stderr
lark: wrong -as (it goes with FILE of -; a file's kind is its extension)
try lark help
--- new files
`,
		},
		{
			`version`,
			`
args: "version"
exit: 0
--- stdout
lark <version> <go>
--- stderr
--- new files
`,
		},
		{
			`version beside a script`,
			`
args: "version hi.star"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (version takes none)
try lark help
--- new files
`,
		},
		{
			`version as a flag`,
			`
args: "-version"
exit: 2
--- stdout
--- stderr
lark: unknown verb "-version" (did you mean version?)
try lark help
--- new files
`,
		},
		{
			`run script double dash`,
			`
args: "run --logs logs hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
logs
logs/hi.star.log
--- logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`translate script`,
			`
args: "convert hi.star"
exit: 0
--- stdout
{
  "text": "print(\"hi\")\nprint(\"two\")"
}
--- stderr
--- new files
`,
		},
		{
			`run flow`,
			`
args: "run hi.json"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`translate flow`,
			`
args: "convert hi.json"
exit: 0
--- stdout
def main():
    print("hi")
    print("two")
    pass

--- stderr
--- new files
`,
		},
		{
			`logs`,
			`
args: "run -logs logs hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
logs
logs/hi.star.log
--- logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`logs in a nested directory`,
			`
args: "run -logs a/b/logs hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
a
a/b
a/b/logs
a/b/logs/hi.star.log
--- a/b/logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`logs of a flow`,
			`
args: "run -logs logs hi.json"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
logs
logs/hi.json.log
--- logs/hi.json.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`bundle`,
			`
args: "compile -o out.bin hi.star"
exit: 0
--- stdout
--- stderr
lark: wrote "out.bin"
--- new files
out.bin
`,
		},
		{
			`bundle of a flow`,
			`
args: "compile -o out.bin hi.json"
exit: 0
--- stdout
--- stderr
lark: wrote "out.bin"
--- new files
out.bin
`,
		},
		{
			`help`,
			`
args: "-h"
exit: 0
--- stdout
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
--- stderr
--- new files
`,
		},
		{
			`help double dash`,
			`
args: "--help"
exit: 0
--- stdout
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
--- stderr
--- new files
`,
		},
		{
			`help verb`,
			`
args: "help"
exit: 0
--- stdout
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
--- stderr
--- new files
`,
		},
		{
			`help of run`,
			`
args: "help run"
exit: 0
--- stdout
Usage:
  lark run [flags] FILE       run a script, flow or bundle

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

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`help of compile`,
			`
args: "help compile"
exit: 0
--- stdout
Usage:
  lark compile -o OUT FILE    compile a script or flow into a bundle

FILE: .bin is a bundle, .json a flow, anything else a script.
FILE of - reads standard input as a script; -as flow or
-as bundle reads it as that kind.

compile:
  -o FILE        the bundle to write (required)
  -plugins DIR   plugins the script uses

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`help of convert`,
			`
args: "help convert"
exit: 0
--- stdout
Usage:
  lark convert FILE           print a script as a flow, a flow as a script

FILE: .bin is a bundle, .json a flow, anything else a script.
FILE of - reads standard input as a script; -as flow or
-as bundle reads it as that kind.

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`help of version`,
			`
args: "help version"
exit: 0
--- stdout
Usage:
  lark version                print the versions of lark and Go

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`help of help`,
			`
args: "help help"
exit: 0
--- stdout
Usage:
  lark help [verb]            print this help, or one verb's help

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`help of a word that is no verb`,
			`
args: "help zebra"
exit: 2
--- stdout
--- stderr
lark: unknown verb "zebra"
try lark help
--- new files
`,
		},
		{
			`help of two verbs`,
			`
args: "help run compile"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (help takes one verb at most)
try lark help
--- new files
`,
		},
		{
			`flag -h after a verb`,
			`
args: "run -h"
exit: 0
--- stdout
Usage:
  lark run [flags] FILE       run a script, flow or bundle

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

Exit codes: 0 ok, 1 failed, 2 wrong command line,
3 file not read or made, 4 stopped (SIGINT, SIGTERM, SIGHUP,
or closed standard output under -logs or -hold), 5 memory watch
--- stderr
--- new files
`,
		},
		{
			`one letter script`,
			`
args: "-s hi.star"
exit: 2
--- stdout
--- stderr
lark: unknown verb "-s"
try lark help
--- new files
`,
		},
		{
			`one letter translate`,
			`
args: "run -t hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -t
try lark help
--- new files
`,
		},
		{
			`one letter logs`,
			`
args: "run -l logs hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -l
try lark help
--- new files
`,
		},
		{
			`one letter bundle`,
			`
args: "run -b out.bin hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -b
try lark help
--- new files
`,
		},
		{
			`one letter args`,
			`
args: "run -a {} hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -a
try lark help
--- new files
`,
		},
		{
			`one letter plugins`,
			`
args: "run -p plugins hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -p
try lark help
--- new files
`,
		},
		{
			`gone memory ceiling`,
			`
args: "run -m 64 hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -m
try lark help
--- new files
`,
		},
		{
			`gone flag script`,
			`
args: "run -script hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -script
try lark help
--- new files
`,
		},
		{
			`gone flag translate`,
			`
args: "run -translate hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -translate
try lark help
--- new files
`,
		},
		{
			`gone flag bundle`,
			`
args: "run -bundle out.bin hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -bundle
try lark help
--- new files
`,
		},
		{
			`gone flag changes`,
			`
args: "run -changes c.jsonl hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -changes
try lark help
--- new files
`,
		},
		{
			`unknown flag`,
			`
args: "run hi.star -nope"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -nope
try lark help
--- new files
`,
		},
		{
			`string flag without a value`,
			`
args: "run -logs"
exit: 2
--- stdout
--- stderr
flag needs an argument: -logs
try lark help
--- new files
`,
		},
		{
			`bool flag with a bad value`,
			`
args: "run hi.star -hold=maybe"
exit: 2
--- stdout
--- stderr
invalid boolean value "maybe" for -hold: parse error
try lark help
--- new files
`,
		},
		{
			`stray argument`,
			`
args: "run hi.star extra"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`unknown verb`,
			`
args: "rn hi.star"
exit: 2
--- stdout
--- stderr
lark: unknown verb "rn"
try lark help
--- new files
`,
		},
		{
			`unknown verb nothing close`,
			`
args: "zebra hi.star"
exit: 2
--- stdout
--- stderr
lark: unknown verb "zebra"
try lark help
--- new files
`,
		},
		{
			`args default`,
			`
args: "run args.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=localhost:8080
time=<time> level=INFO thread=thread_0 function=main msg="retries 3"
time=<time> level=INFO thread=thread_0 function=main msg="verbose False"
time=<time> level=INFO thread=thread_0 function=main msg="tags [\"staging\"]"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 1"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 2"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 3"
--- stderr
--- new files
`,
		},
		{
			`args given`,
			`
args: "run -args {\"host\": \"db\", \"port\": 1} args.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=db:1
time=<time> level=INFO thread=thread_0 function=main msg="retries 3"
time=<time> level=INFO thread=thread_0 function=main msg="verbose False"
time=<time> level=INFO thread=thread_0 function=main msg="tags [\"staging\"]"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 1"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 2"
time=<time> level=INFO thread=thread_0 function=attempt msg="attempt 3"
--- stderr
--- new files
`,
		},
		{
			`args empty object`,
			`
args: "run -args {} hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`args not json`,
			`
args: "run -args {nope hi.star"
exit: 2
--- stdout
--- stderr
lark: arguments: invalid character 'n' looking for beginning of object key string
try lark help
--- new files
`,
		},
		{
			`args an array`,
			`
args: "run -args [1] hi.star"
exit: 2
--- stdout
--- stderr
lark: arguments: json: cannot unmarshal array into Go value of type map[string]interface {}
try lark help
--- new files
`,
		},
		{
			`plugins empty`,
			`
args: "run -plugins plugins hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`plugins directory with a bracket`,
			`
args: "run -plugins [x hi.star"
exit: 3
--- stdout
--- stderr
lark: open [x: no such file or directory
--- new files
`,
		},
		{
			`plugins of compile`,
			`
args: "compile -plugins plugins -o out.bin hi.star"
exit: 0
--- stdout
--- stderr
lark: wrote "out.bin"
--- new files
out.bin
`,
		},
		{
			`plugins of convert`,
			`
args: "convert -plugins plugins hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -plugins
try lark help
--- new files
`,
		},
		{
			`graph`,
			`
args: "run -graph end.jsonl hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
end.jsonl
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
`,
		},
		{
			`graph of a failed run`,
			`
args: "run -graph end.jsonl fail.star"
exit: 1
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=before
--- stderr
script failed: fail.star:3: fail: boom
--- new files
end.jsonl
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_FAILED"}]
[{"op":"add","path":"/status","value":"STATUS_FAILED"},{"op":"add","path":"/cause","value":{"failure":"fail.star:3: fail: boom","function":"main"}}]
`,
		},
		{
			`graph to standard output`,
			`
args: "run -graph - hi.star"
exit: 0
--- stdout
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
--- stderr
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- new files
`,
		},
		{
			`graph to standard output of a failed run`,
			`
args: "run -graph - fail.star"
exit: 1
--- stdout
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_FAILED"}]
[{"op":"add","path":"/status","value":"STATUS_FAILED"},{"op":"add","path":"/cause","value":{"failure":"fail.star:3: fail: boom","function":"main"}}]
--- stderr
time=<time> level=INFO thread=thread_0 function=main msg=before
script failed: fail.star:3: fail: boom
--- new files
`,
		},
		{
			`held lines and graph to standard output`,
			`
args: "run -hold -graph - hi.star"
exit: 0
--- stdout
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- new files
`,
		},
		{
			`failure in a nested function names its cause`,
			`
args: "run nested.star"
exit: 1
--- stdout
--- stderr
script failed: nested.star:2: fail: boom
cause: deep
--- new files
`,
		},
		{
			`graph not written when the script does not compile`,
			`
args: "run -graph end.jsonl bad.star"
exit: 1
--- stdout
--- stderr
script failed: bad.star:1:11: got ':', want ')'
--- new files
end.jsonl
--- end.jsonl
`,
		},
		{
			`graph replaces a file`,
			`
args: "run -graph old.json hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
--- old.json
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
`,
		},
		{
			`graph in a missing directory`,
			`
args: "run -graph nodir/end.jsonl hi.star"
exit: 3
--- stdout
--- stderr
lark: graph "nodir/end.jsonl": no such file or directory
--- new files
`,
		},
		{
			`graph over a directory`,
			`
args: "run -graph plugins hi.star"
exit: 3
--- stdout
--- stderr
lark: graph "plugins": is a directory
--- new files
`,
		},
		{
			`hold`,
			`
args: "run -hold hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
--- new files
`,
		},
		{
			`hold with logs`,
			`
args: "run -hold -logs logs hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
logs
logs/hi.star.log
--- logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
`,
		},
		{
			`hold with graph`,
			`
args: "run -hold -graph end.jsonl hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
--- new files
end.jsonl
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
`,
		},
		{
			`hold a failed script`,
			`
args: "run -hold fail.star"
exit: 1
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=before
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
script failed: fail.star:3: fail: boom
--- new files
`,
		},
		{
			`memory on a short run`,
			`
args: "run -memory hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
`,
		},
		{
			`memory with graph`,
			`
args: "run -memory -graph end.jsonl hi.star"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
end.jsonl
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
`,
		},
		{
			`failed script`,
			`
args: "run fail.star"
exit: 1
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=before
--- stderr
script failed: fail.star:3: fail: boom
--- new files
`,
		},
		{
			`failed flow compile`,
			`
args: "run junk.json"
exit: 1
--- stdout
--- stderr
flow failed: junk.json: proto: syntax error (line 1:1): invalid value not
--- new files
`,
		},
		{
			`failed flow of plain values`,
			`
args: "run plain.json"
exit: 1
--- stdout
--- stderr
flow failed: plain.json: proto: syntax error (line 1:124): unexpected token "west" (a value is an object named for its kind, such as {"string": "x"} or {"int": "3"})
--- new files
`,
		},
		{
			`failed flow with a constant that states nothing`,
			`
args: "run emptyconst.json"
exit: 1
--- stdout
--- stderr
flow failed: emptyconst.json: "A": constant has no value and no call: no form
--- new files
`,
		},
		{
			`failed translate of a flow with an argument that has no name`,
			`
args: "convert emptyarg.json"
exit: 1
--- stdout
--- stderr
flow failed: emptyarg.json: "a": argument has no name: no form
--- new files
`,
		},
		{
			`failed flow with an operand that has no source`,
			`
args: "run emptyoperand.json"
exit: 1
--- stdout
--- stderr
flow failed: emptyoperand.json: call of "f": operand has no source: no form
--- new files
`,
		},
		{
			`failed translate of a script`,
			`
args: "convert bad.star"
exit: 1
--- stdout
--- stderr
script failed: bad.star: bad.star:1:11: got ':', want ')'
--- new files
`,
		},
		{
			`failed translate of a flow`,
			`
args: "convert junk.json"
exit: 1
--- stdout
--- stderr
flow failed: junk.json: proto: syntax error (line 1:1): invalid value not
--- new files
`,
		},
		{
			`failed bundle`,
			`
args: "compile -o out.bin bad.star"
exit: 1
--- stdout
--- stderr
script failed: bad.star:1:11: got ':', want ')'
--- new files
`,
		},
		{
			`script does not compile`,
			`
args: "run bad.star"
exit: 1
--- stdout
--- stderr
script failed: bad.star:1:11: got ':', want ')'
--- new files
`,
		},
		{
			`bundle is garbage`,
			`
args: "run junk.bin"
exit: 1
--- stdout
--- stderr
bundle failed: junk.bin: unmarshal bundle: proto: cannot parse invalid wire-format data
--- new files
`,
		},
		{
			`script file missing`,
			`
args: "run none.star"
exit: 3
--- stdout
--- stderr
lark: open none.star: no such file or directory
--- new files
`,
		},
		{
			`flow file missing`,
			`
args: "run none.json"
exit: 3
--- stdout
--- stderr
lark: open none.json: no such file or directory
--- new files
`,
		},
		{
			`bundle file missing`,
			`
args: "run none.bin"
exit: 3
--- stdout
--- stderr
lark: open none.bin: no such file or directory
--- new files
`,
		},
		{
			`script file is a directory`,
			`
args: "run plugins"
exit: 3
--- stdout
--- stderr
lark: read plugins: is a directory
--- new files
`,
		},
		{
			`logs path is a file`,
			`
args: "run -logs hi.star hi.star"
exit: 3
--- stdout
--- stderr
lark: log hi.star: mkdir hi.star: not a directory
--- new files
`,
		},
		{
			`bundle path is a directory`,
			`
args: "compile -o plugins hi.star"
exit: 3
--- stdout
--- stderr
lark: bundle "plugins": is a directory
--- new files
`,
		},
		{
			`bundle path in a missing directory`,
			`
args: "compile -o nodir/out.bin hi.star"
exit: 3
--- stdout
--- stderr
lark: bundle "nodir/out.bin": no such file or directory
--- new files
`,
		},
		{
			`no verb`,
			`
args: ""
exit: 2
--- stdout
--- stderr
lark: give a verb (run, compile, convert, version or help)
try lark help
--- new files
`,
		},
		{
			`verb without input`,
			`
args: "run"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`compile without a bundle path`,
			`
args: "compile hi.star"
exit: 2
--- stdout
--- stderr
lark: compile needs -o FILE
try lark help
--- new files
`,
		},
		{
			`compile without input`,
			`
args: "compile -o out.bin"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`convert without input`,
			`
args: "convert"
exit: 2
--- stdout
--- stderr
lark: wrong number of arguments (give exactly one FILE)
try lark help
--- new files
`,
		},
		{
			`run with a bundle`,
			`
args: "run -graph end.jsonl hi.bin"
exit: 0
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=hi
time=<time> level=INFO thread=thread_0 function=main msg=two
--- stderr
--- new files
end.jsonl
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_SUCCEEDED"}]
[{"op":"add","path":"/status","value":"STATUS_SUCCEEDED"}]
`,
		},
		{
			`convert a bundle`,
			`
args: "convert hi.bin"
exit: 2
--- stdout
--- stderr
lark: a bundle is only run (compile and convert need a script or a flow)
try lark help
--- new files
`,
		},
		{
			`compile a bundle`,
			`
args: "compile -o out.bin hi.bin"
exit: 2
--- stdout
--- stderr
lark: a bundle is only run (compile and convert need a script or a flow)
try lark help
--- new files
`,
		},
		{
			`graph with convert`,
			`
args: "convert -graph end.jsonl hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -graph
try lark help
--- new files
`,
		},
		{
			`graph with compile`,
			`
args: "compile -o out.bin -graph end.jsonl hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -graph
try lark help
--- new files
`,
		},
		{
			`hold with convert`,
			`
args: "convert -hold hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -hold
try lark help
--- new files
`,
		},
		{
			`hold with compile`,
			`
args: "compile -o out.bin -hold hi.star"
exit: 2
--- stdout
--- stderr
flag provided but not defined: -hold
try lark help
--- new files
`,
		},
		{
			`stopped by terminate`,
			`
args: "run -graph end.jsonl -logs logs sleepy.star"
exit: 4
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=ready
--- stderr
script stopped
--- new files
end.jsonl
logs
logs/sleepy.star.log
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_CANCELLED"}]
[{"op":"add","path":"/status","value":"STATUS_CANCELLED"}]
--- logs/sleepy.star.log
time=<time> level=INFO thread=thread_0 function=main msg=ready
`,
		},
		{
			`stopped by interrupt`,
			`
args: "run sleepy.star"
exit: 4
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=ready
--- stderr
script stopped
--- new files
`,
		},
		{
			`stopped by hangup`,
			`
args: "run -graph end.jsonl -logs logs sleepy.star"
exit: 4
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=ready
--- stderr
script stopped
--- new files
end.jsonl
logs
logs/sleepy.star.log
--- end.jsonl
[{"op":"add","path":"/status","value":"STATUS_RUNNING"}]
[{"op":"add","path":"/functions","value":[{"name":"main","status":"STATUS_RUNNING"}]},{"op":"add","path":"/functions/0/threads","value":["thread_0"]}]
[{"op":"remove","path":"/functions/0/threads"},{"op":"add","path":"/functions/0/status","value":"STATUS_CANCELLED"}]
[{"op":"add","path":"/status","value":"STATUS_CANCELLED"}]
--- logs/sleepy.star.log
time=<time> level=INFO thread=thread_0 function=main msg=ready
`,
		},
		{
			`stopped flow`,
			`
args: "run sleepy.json"
exit: 4
--- stdout
time=<time> level=INFO thread=thread_0 function=main msg=ready
--- stderr
flow stopped
--- new files
`,
		},
		{
			`closed output ends by the signal`,
			`
args: "run hi.star"
exit: signal: broken pipe
--- stdout
--- stderr
--- new files
`,
		},
		{
			`closed output with logs is a stop`,
			`
args: "run -logs logs hi.star"
exit: 4
--- stdout
--- stderr
script stopped
--- new files
logs
logs/hi.star.log
--- logs/hi.star.log
time=<time> level=INFO thread=thread_0 function=main msg=hi
`,
		},
		{
			`closed output while held is a stop`,
			`
args: "run -hold hi.star"
exit: 4
--- stdout
--- stderr
lark: lines are held until the run ends: <dir>/tmp/lark-<n>.log
script stopped
--- new files
`,
		},
	}
)

// END GOLDEN
