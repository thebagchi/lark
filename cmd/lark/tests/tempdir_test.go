package tests

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

// TestMain_AChildProcessBuildsAndLeavesNothing proves a child process of the
// tests, this binary run again, does not build the lark command: it uses the
// binary its parent built, and leaves no lark-flags directory in the temporary
// directory. A child ends with os.Exit inside a test, so a directory it made
// was never removed, and a run of the tests left one for each child (B270).
//
// The child is given a temporary directory of its own, so the count is of what
// this child made and is not disturbed by another run of the tests.
//
// Revisions:
//   - 2026-11-11 00:44: initial creation (C122, B270)
func TestMain_AChildProcessBuildsAndLeavesNothing(t *testing.T) {
	const CHILD = "LARK_ABANDON_CHILD"

	if os.Getenv(CHILD) == "1" {
		t.Skip("this is the child")
	}

	var (
		temp = t.TempDir()
		cmd  = exec.Command(
			os.Args[0],
			"-test.run=^TestAbandoning_PrintsOneLineAndExitsFive$",
		)
		exit *exec.ExitError
	)

	cmd.Env = append(
		os.Environ(),
		CHILD+"=1",
		"TMPDIR="+temp,
	)

	err := cmd.Run()
	if !errors.As(
		err,
		&exit,
	) || exit.ExitCode() != command.MEMORY {
		t.Fatalf(
			"the child gave %v, want exit 5",
			err,
		)
	}

	left, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range left {
		if strings.HasPrefix(
			entry.Name(),
			"lark-flags",
		) {
			t.Errorf(
				"the child left %s behind",
				entry.Name(),
			)
		}
	}

	if os.Getenv(BUILT_ENV) != BUILT {
		t.Errorf(
			"%s is %q, want the binary built once, %q",
			BUILT_ENV,
			os.Getenv(BUILT_ENV),
			BUILT,
		)
	}
}
