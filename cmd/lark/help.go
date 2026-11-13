package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

const (
	// TRY is the line that follows an error on the command line: it names the
	// way to the help, which is not repeated there.
	//
	// Revisions:
	//   - 2026-11-05 18:10: initial creation (C68)
	//   - 2026-11-10 11:45: names the help verb; the text is proposed (C119, Q305)
	TRY = "try lark help"
)

const (
	// HELP is the text of lark help, as the user approved it (Q172, Q310). It is
	// plain ASCII, with no line over 79 columns.
	//
	// It opens with a newline after the backtick and closes on its own line, so
	// the text reads as it prints (Q289). The newline is stripped where the text
	// is served, with strings.TrimPrefix(HELP, "\n"), so the bytes are those of
	// the file this const replaced (Q369).
	//
	// Revisions:
	//   - 2026-11-05 18:10: initial creation (C68)
	//   - 2026-11-08 22:36: the Go API is in go doc, not in README.md (C78, Q266)
	//   - 2026-11-10 11:45: the text of the verbs, as the spec drafts it (C119, Q299, Q310)
	//   - 2026-11-11 00:44: a raw-string const here, in place of help.txt and its embed
	//     (C122, Q369)
	HELP = `
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
)

// _PrintHelp prints the help on standard output and exits 0: a person who asks
// for the help gets it as output, not as an error (Q173). The help is the whole
// text for an empty topic, else the part of it that belongs to the verb topic.
//
// Revisions:
//   - 2026-11-05 18:10: initial creation (C68)
//   - 2026-11-10 11:45: prints the help of one verb for lark help VERB (C119, Q305)
//   - 2026-11-11 00:44: strips the newline the const opens with (C122, Q369)
func _PrintHelp(topic string) {
	text := strings.TrimPrefix(
		HELP,
		"\n",
	)
	if topic != "" {
		text = command.HelpOf(
			text,
			topic,
		)
	}

	// A failed write has nothing left to try before the exit.
	_, _ = fmt.Fprint(
		os.Stdout,
		text,
	)

	os.Exit(0)
}
