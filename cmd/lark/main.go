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
	FAILED    = 1
	NO_SCRIPT = 2
	UNREADAB  = 3
)

// ErrNoScript is returned when no script was named.
var ErrNoScript = errors.New("lark: no script given")

// main runs the script named by -s and prints what its entry point returned.
//
// Exits 0 on success, 1 when the script failed, 2 when no script was named and
// 3 when the file could not be read. The last two are this command's problem
// and open with "lark:"; the first is the script's and opens with "script
// failed:". A sample that demonstrates a failure therefore exits 1 and says so
// in its own words, without this command knowing anything about samples.
//
// Everything goes to standard error except the value the script returned, so
// stdout can be piped.
//
// Revisions:
//   - 2026-09-19 23:40: initial creation
//   - 2026-09-19 23:47: tells a failed script apart from a command that could
//     not run one, by exit code and by the word the message opens with
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
		os.Exit(UNREADAB)
	}

	value, err := _Run(context.Background(), *script, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "script failed: %v\n", err)
		os.Exit(FAILED)
	}

	fmt.Println(value)
}

// _Run compiles src as the script at path and calls its entry point.
//
// The compiler is built without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
//   - 2026-09-19 23:47: takes the source, so reading the file is the caller's
//     problem and can be reported as one
func _Run(ctx context.Context, path string, src []byte) (string, error) {
	artifact, err := runtime.NewCompiler().Compile(path, src)
	if err != nil {
		return "", err
	}

	value, err := artifact.Run(ctx)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}
