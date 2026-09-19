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
)

const (
	SCRIPT_FLAG  = "s"
	SCRIPT_USAGE = "path to the Starlark script to run"
	NO_SCRIPT    = 2
	FAILED       = 1
)

// ErrNoScript is returned when no script was named.
var ErrNoScript = errors.New("lark: no script given")

// main runs the script named by -s and prints what its entry point returned.
//
// Exits 2 when no script was named, 1 when the script failed to compile or
// run, and 0 otherwise. A failure is printed to standard error so the value a
// script returns is the only thing on standard output.
//
// Revisions:
//   - 2026-09-19 23:40: initial creation
func main() {
	script := flag.String(SCRIPT_FLAG, "", SCRIPT_USAGE)

	flag.Parse()

	if *script == "" {
		fmt.Fprintln(os.Stderr, ErrNoScript)
		flag.Usage()
		os.Exit(NO_SCRIPT)
	}

	value, err := _Run(context.Background(), *script)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(FAILED)
	}

	fmt.Println(value)
}

// _Run compiles the script at path and calls its entry point.
//
// The compiler is built without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
func _Run(ctx context.Context, path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

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
