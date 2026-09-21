// Command lark runs a Starlark script.
//
// It is the smallest host this runtime supports: it compiles the script named
// on the command line, together with every module that script loads, and calls
// its entry point.
//
//	lark -s samples/concurrent.star
//
// Modules resolve beside the file that loaded them, so a script in samples/
// loading "strings.star" reads samples/strings.star.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/thebagchi/lark/runtime"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/json"
	_ "github.com/thebagchi/lark/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/runtime/plugin/math"
	_ "github.com/thebagchi/lark/runtime/plugin/state"
	_ "github.com/thebagchi/lark/runtime/plugin/time"
)

const (
	SCRIPT_FLAG  = "s"
	SCRIPT_USAGE = "path to the Starlark script to run"

	// A script that fails is not the same event as a command that cannot do
	// its job, and a caller acts on them differently: the first means the
	// script is wrong, the second means the invocation is. They are told apart
	// by exit code and by the word the message opens with.
	FAILED     = 1
	NO_SCRIPT  = 2
	UNREADABLE = 3
)

// ErrNoScript is returned when no script was named.
var ErrNoScript = errors.New("lark: no script given")

// main runs the script named by -s.
//
// Exits 0 on success, 1 when the script failed, 2 when no script was named and
// 3 when the file could not be read. The last two are this command's problem
// and open with "lark:"; the first is the script's and opens with "script
// failed:". A sample that demonstrates a failure therefore exits 1 and says so
// in its own words, without this command knowing anything about samples.
//
// A script's output is what it prints, which goes to standard output so it can
// be piped. Everything this command says goes to standard error. The
// interpreter's own default is standard error for both, so the printer is
// supplied here rather than assumed.
//
// What the entry point returned is not printed. A script does its job and
// exits; its results are the lines it printed, which is what a host collects as
// the run's log. Printing the value too would put a trailing None under every
// script that correctly returns nothing.
//
// Revisions:
//   - 2026-09-19 23:40: initial creation
//   - 2026-09-19 23:47: tells a failed script apart from a command that could
//     not run one, by exit code and by the word the message opens with
//   - 2026-09-21 00:26: no longer prints what the entry point returned
//   - 2026-09-21 08:09: names the third exit code in full
func main() {
	script := flag.String(SCRIPT_FLAG, "", SCRIPT_USAGE)

	flag.Parse()

	if *script == "" {
		fmt.Fprintln(os.Stderr, ErrNoScript)
		flag.Usage()
		os.Exit(NO_SCRIPT)
	}

	src, err := os.ReadFile(*script)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(UNREADABLE)
	}

	err = _Run(context.Background(), *script, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "script failed: %v\n", err)
		os.Exit(FAILED)
	}
}

// _Run compiles src as the script at path and calls its entry point, with
// what it prints going to standard output.
//
// The compiler is built without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
//   - 2026-09-19 23:47: takes the source, so reading the file is the caller's
//     problem and can be reported as one
//   - 2026-09-21 00:26: reports only whether the run failed, since the value is
//     no longer printed
//   - 2026-09-21 08:09: sends what the script prints to standard output, which
//     the doc had claimed and the interpreter's default did not do
func _Run(ctx context.Context, path string, src []byte) error {
	artifact, err := runtime.NewCompiler().Compile(path, src)
	if err != nil {
		return err
	}

	printing := runtime.WithPrinter(ctx, func(msg string) {
		_, err := fmt.Fprintln(os.Stdout, msg)
		if err != nil {
			// The script's line is lost and the script does not know. Standard
			// error is this command's own channel, so that is where it is said.
			fmt.Fprintf(os.Stderr, "lark: print: %v\n", err)
		}
	})

	_, err = artifact.Run(printing)
	if err != nil {
		return err
	}

	return nil
}
