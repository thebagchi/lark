package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

const (
	// VERSION_FLAG is the -version flag.
	//
	// Revisions:
	//   - 2026-11-04 23:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	VERSION_FLAG = "version"
)

// _PrintVersion prints the -version line and a newline on standard output and
// exits 0. It is the end of a command line that asked for the version.
//
// Revisions:
//   - 2026-11-04 22:51: initial creation (C67)
//   - 2026-11-10 11:32: calls Version of command, which moved to internal/command (C120b, Q315)
func _PrintVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}

	// A failed write has nothing left to try before the exit.
	_, _ = fmt.Fprintln(
		os.Stdout,
		command.Version(info),
	)

	os.Exit(0)
}
