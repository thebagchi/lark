package tests

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

const (
	// RUN_LIMIT is the longest a command run by a test may take, and
	// KILL_WAIT how long after the kill the test waits for its output to close.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RUN_LIMIT = 3 * time.Minute
	KILL_WAIT = 5 * time.Second

	// STRESS_LIMIT is the longest a stress run may take in all, and STALL how
	// long the machine's available memory may go without a new low before the
	// run counts as stuck. SAMPLE is how often the test reads it, and FALL how
	// far under the lowest reading a new reading must be to count as a fall,
	// so that noise of a few megabytes is not progress.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	STRESS_LIMIT = 10 * time.Minute
	STALL        = 60 * time.Second
	SAMPLE       = 2 * time.Second
	FALL         = 32 << 20

	// REFUSED is what the flag package says of a flag it was not given, and
	// EXIT_FLAGS the code lark exits with when the flags are wrong.
	//
	// Revisions:
	//   - 2026-11-01 22:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	REFUSED    = "flag provided but not defined"
	EXIT_FLAGS = 2

	// FILL_LIST appends to a list for ever, in the interpreter, on four threads
	// at once: measured at small scale, four threads grow memory about one and
	// a half times as fast as one (1.3 to 1.5 GB a second against 0.8 to 1.0).
	// STEADY_BUILTIN
	// calls the patch_json builtin in a loop, each call copying a chunk and
	// keeping the result, so memory grows steadily (about 500 MB a second
	// measured) and each Go call is short enough to return and poll.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FILL_LIST = "def fill():\n    held = []\n    for i in range(1000000000):\n" +
		"        held.append([0] * 1000000)\n\n" +
		"def main():\n    threads = []\n    for i in range(4):\n" +
		"        threads.append(spawn(fill))\n    join(*threads)\n"
	STEADY_BUILTIN = "def main():\n    chunk = [0] * 100000\n    held = []\n" +
		"    for i in range(100000000):\n" +
		"        held.append(patch_json({\"a\": chunk}," +
		" [{\"op\": \"copy\", \"from\": \"/a\", \"path\": \"/b\"}]))\n"

	// PRIVATE is the mode a test directory is made with, SHARED the mode of a
	// test file.
	//
	// Revisions:
	//   - 2026-11-01 22:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PRIVATE = 0o750
	SHARED  = 0o640
)

const (
	// BUILT_ENV is the variable that carries the path of the built lark binary
	// to a child process of the tests, which runs TestMain again and must not
	// build it a second time.
	//
	// Revisions:
	//   - 2026-11-11 00:44: initial creation (C122, B270)
	BUILT_ENV = "LARK_TEST_BUILT"
)

var (
	// BUILT is the lark binary every test in this file runs, made once by
	// TestMain.
	//
	// Revisions:
	//   - 2026-11-01 22:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BUILT string
)

// TestMain builds the lark command once, so every test runs the real flag
// parsing of the real binary.
//
// The directory it builds into is removed when the tests end. A child process
// of the tests is this binary run again, and it builds nothing: the path of the
// binary its parent made is in its environment, under BUILT_ENV. A child ends
// with os.Exit inside a test, so a directory it made would never be removed
// (B270). A parent killed by a signal still leaves its directory.
//
// Revisions:
//   - 2026-11-01 12:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-11 00:44: a child process takes the binary of its parent, and builds none
//     (C122, B270)
func TestMain(m *testing.M) {
	BUILT = os.Getenv(BUILT_ENV)
	if BUILT != "" {
		os.Exit(m.Run())
	}

	dir, err := os.MkdirTemp(
		"",
		"lark-flags",
	)
	if err != nil {
		panic(err)
	}

	BUILT = filepath.Join(
		dir,
		"lark.bin",
	)

	out, err := exec.Command(
		"go",
		"build",
		"-o",
		BUILT,
		"..",
	).CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)

		panic(string(out))
	}

	err = os.Setenv(
		BUILT_ENV,
		BUILT,
	)
	if err != nil {
		_ = os.RemoveAll(dir)

		panic(err)
	}

	code := m.Run()

	_ = os.RemoveAll(dir)

	os.Exit(code)
}

// TestFlags_NamesAreFull proves that each full flag name is accepted by its
// verb, and that each old one-letter name, with -m, each flag the verbs replaced
// (-script, -translate, -bundle, -changes) and each flag of another verb is
// refused with exit 2.
//
// Cases that need a file made by an earlier case run in order, and a case that
// is refused must say "flag provided but not defined" on standard error.
//
// Revisions:
//   - 2026-11-01 12:00: initial creation
//   - 2026-11-02 22:32: drops the -memory refusal; -memory is now the memory watch flag (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: the cases are verbs and the flags that belong to them; convert
//     takes no -plugins (C119, Q305, Q337)
func TestFlags_NamesAreFull(t *testing.T) {
	var (
		dir    = t.TempDir()
		script = filepath.Join(
			dir,
			"hi.star",
		)
		flow = filepath.Join(
			dir,
			"hi.json",
		)
		bundle = filepath.Join(
			dir,
			"hi.bin",
		)
		logs = filepath.Join(
			dir,
			"logs",
		)
		plugins = filepath.Join(
			dir,
			"plugins",
		)
	)

	err := os.WriteFile(
		script,
		[]byte("def main():\n    print(\"hi\")\n"),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(
		plugins,
		PRIVATE,
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(
		BUILT,
		"convert",
		script,
	).Output()
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		flow,
		out,
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		code int
		said string // what standard error must hold, empty for no check
	}{
		{
			"run",
			[]string{"run", script},
			0,
			"",
		},
		{
			"convert a script",
			[]string{"convert", script},
			0,
			"",
		},
		{
			"run a flow",
			[]string{"run", flow},
			0,
			"",
		},
		{
			"convert a flow",
			[]string{"convert", flow},
			0,
			"",
		},
		{
			"run -logs",
			[]string{"run", "-logs", logs, script},
			0,
			"",
		},
		{
			"compile -o",
			[]string{"compile", "-o", bundle, script},
			0,
			"",
		},
		{
			"run a bundle",
			[]string{"run", bundle},
			0,
			"",
		},
		{
			"run -args",
			[]string{"run", "-args", "{}", script},
			0,
			"",
		},
		{
			"run -plugins",
			[]string{"run", "-plugins", plugins, script},
			0,
			"",
		},
		{
			"compile -plugins",
			[]string{"compile", "-plugins", plugins, "-o", bundle, script},
			0,
			"",
		},
		{
			"-plugins is a flag of run and compile",
			[]string{"convert", "-plugins", plugins, script},
			EXIT_FLAGS,
			REFUSED + ": -plugins",
		},
		{
			"run -graph",
			[]string{"run", "-graph", filepath.Join(
				dir,
				"g.jsonl",
			), script},
			0,
			"",
		},
		{
			"-script is gone",
			[]string{"run", "-script", script},
			EXIT_FLAGS,
			REFUSED + ": -script",
		},
		{
			"-translate is gone",
			[]string{"run", "-translate", script},
			EXIT_FLAGS,
			REFUSED + ": -translate",
		},
		{
			"-bundle is gone",
			[]string{"run", "-bundle", bundle, script},
			EXIT_FLAGS,
			REFUSED + ": -bundle",
		},
		{
			"-changes is gone",
			[]string{"run", "-changes", filepath.Join(
				dir,
				"c.jsonl",
			), script},
			EXIT_FLAGS,
			REFUSED + ": -changes",
		},
		{
			"-o is a flag of compile",
			[]string{"run", "-o", bundle, script},
			EXIT_FLAGS,
			REFUSED + ": -o",
		},
		{
			"-hold is a flag of run",
			[]string{"compile", "-o", bundle, "-hold", script},
			EXIT_FLAGS,
			REFUSED + ": -hold",
		},
		{
			"-args is a flag of run",
			[]string{"convert", "-args", "{}", script},
			EXIT_FLAGS,
			REFUSED + ": -args",
		},
		{
			"-t",
			[]string{"run", "-t", script},
			EXIT_FLAGS,
			REFUSED + ": -t",
		},
		{
			"-l",
			[]string{"run", "-l", logs, script},
			EXIT_FLAGS,
			REFUSED + ": -l",
		},
		{
			"-b",
			[]string{"run", "-b", bundle, script},
			EXIT_FLAGS,
			REFUSED + ": -b",
		},
		{
			"-a",
			[]string{"run", "-a", "{}", script},
			EXIT_FLAGS,
			REFUSED + ": -a",
		},
		{
			"-p",
			[]string{"run", "-p", plugins, script},
			EXIT_FLAGS,
			REFUSED + ": -p",
		},
		{
			"-m",
			[]string{"run", "-m", "64", script},
			EXIT_FLAGS,
			REFUSED + ": -m",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				cmd := exec.Command(
					BUILT,
					item.args...,
				)

				var (
					stderr strings.Builder
					exit   *exec.ExitError
					code   int
				)

				cmd.Stderr = &stderr

				err := cmd.Run()
				if errors.As(
					err,
					&exit,
				) {
					code = exit.ExitCode()
				} else if err != nil {
					t.Fatal(err)
				}

				if code != item.code {
					t.Fatalf(
						"exit %d, want %d\n%s",
						code,
						item.code,
						stderr.String(),
					)
				}

				if item.said != "" && !strings.Contains(
					stderr.String(),
					item.said,
				) {
					t.Fatalf(
						"stderr %q, want it to hold %q",
						stderr.String(),
						item.said,
					)
				}

				if item.code == 0 && strings.Contains(
					stderr.String(),
					REFUSED,
				) {
					t.Fatalf(
						"flag refused: %s",
						stderr.String(),
					)
				}
			},
		)
	}
}

// Run runs the built command with args, and returns its exit code and what it
// wrote to standard error.
//
// The child is killed when the test ends or RUN_LIMIT passes, whichever is
// first, so a test that fails or times out never leaves a lark process running
// (a stress run once left one at 13 GB). RUN_LIMIT is well below go test's
// default 10 minute -timeout, because a test binary that go test kills on that
// timeout cannot kill its child; the limit must fire first.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: kills the child on cleanup and at RUN_LIMIT (C27)
//   - 2026-11-02 23:47: kills the child's whole process group (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Run(
	t *testing.T,
	args ...string,
) (int, string) {
	t.Helper()

	var (
		ctx, cancel = context.WithTimeout(
			t.Context(),
			RUN_LIMIT,
		)
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			args...,
		)
		stderr strings.Builder
		exit   *exec.ExitError
		code   int
	)

	defer cancel()

	cmd.Stderr = &stderr
	cmd.WaitDelay = KILL_WAIT

	_Grouped(cmd)

	err := cmd.Run()

	if errors.Is(
		ctx.Err(),
		context.DeadlineExceeded,
	) {
		t.Fatalf(
			"killed after %v; stderr tail %q",
			RUN_LIMIT,
			stderr.String()[max(
				0,
				stderr.Len()-300,
			):],
		)
	}

	if errors.As(
		err,
		&exit,
	) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return code, stderr.String()
}

// _Available is the MemAvailable line of /proc/meminfo, in bytes, and whether it
// could be read.
//
// Revisions:
//   - 2026-11-02 23:00: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Available() (uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}

	for line := range strings.SplitSeq(
		string(data),
		"\n",
	) {
		name, rest, found := strings.Cut(
			line,
			":",
		)
		if !found || name != "MemAvailable" {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, false
		}

		kilobytes, err := strconv.ParseUint(
			fields[0],
			10,
			64,
		)

		return kilobytes << 10, err == nil
	}

	return 0, false
}

// _RunStress is Run for a run that fills memory: it is killed when the
// machine's available memory has not fallen for STALL, or after STRESS_LIMIT in
// all, and either kill fails the test with the lowest reading and the stderr
// tail. A fixed short limit would fail a slow machine and a limit on progress
// fails only a run that has stopped growing, which is what a real failure
// looks like. The watch itself is not changed to suit the test.
//
// Revisions:
//   - 2026-11-02 23:00: initial creation (C27)
//   - 2026-11-02 23:48: kills the child's whole process group (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _RunStress(
	t *testing.T,
	args ...string,
) (int, string) {
	t.Helper()

	var (
		ctx, cancel = context.WithTimeout(
			t.Context(),
			STRESS_LIMIT,
		)
		cmd = exec.CommandContext(
			ctx,
			BUILT,
			args...,
		)
		stderr  strings.Builder
		exit    *exec.ExitError
		code    int
		stuck   atomic.Bool
		lowest  atomic.Uint64
		stopped = make(chan struct{})
	)

	defer cancel()

	cmd.Stderr = &stderr
	cmd.WaitDelay = KILL_WAIT

	_Grouped(cmd)

	go func() {
		ticker := time.NewTicker(SAMPLE)
		defer ticker.Stop()

		var (
			low  uint64 = math.MaxUint64
			last        = time.Now()
		)

		for {
			select {
			case <-stopped:
				return

			case <-ticker.C:
			}

			now, ok := _Available()
			if !ok {
				continue
			}

			if now+FALL < low {
				low = now
				last = time.Now()

				lowest.Store(low)
			}

			if time.Since(last) > STALL {
				stuck.Store(true)
				cancel()

				return
			}
		}
	}()

	err := cmd.Run()

	close(stopped)

	if stuck.Load() {
		t.Fatalf(
			"no progress: available memory did not fall for %v"+
				" (lowest %d MiB); stderr tail %q",
			STALL,
			lowest.Load()>>20,
			stderr.String()[max(
				0,
				stderr.Len()-300,
			):],
		)
	}

	if errors.Is(
		ctx.Err(),
		context.DeadlineExceeded,
	) {
		t.Fatalf(
			"killed after %v (lowest available %d MiB); stderr tail %q",
			STRESS_LIMIT,
			lowest.Load()>>20,
			stderr.String()[max(
				0,
				stderr.Len()-300,
			):],
		)
	}

	if errors.As(
		err,
		&exit,
	) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return code, stderr.String()
}

// TestMemoryWatch_FlagIsOptIn proves -memory is accepted and a short
// script runs the same with it as without it. Where the system offers
// /proc/meminfo and cgroup v2 the exit is 0 both ways; where it does not, the
// flag exits 2 with "lark:" and what it could not read, and the run without the
// flag still exits 0 - off by default needs nothing from the system.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run verb (C119, Q305)
func TestMemoryWatch_FlagIsOptIn(t *testing.T) {
	var (
		script = filepath.Join(
			t.TempDir(),
			"brief.star",
		)
		err = os.WriteFile(
			script,
			[]byte("def main():\n    print(\"ok\")\n"),
			SHARED,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	code, said := _Run(
		t,
		"run",
		script,
	)
	if code != 0 {
		t.Fatalf(
			"without the flag: exit %d, want 0\n%s",
			code,
			said,
		)
	}

	code, said = _Run(
		t,
		"run",
		"-memory",
		script,
	)

	if memwatch.Check() != nil {
		if code != EXIT_FLAGS || !strings.HasPrefix(
			said,
			"lark: ",
		) ||
			!strings.Contains(
				said,
				"cannot read",
			) {
			t.Fatalf(
				"unsupported system: exit %d, said %q",
				code,
				said,
			)
		}

		return
	}

	if code != 0 {
		t.Fatalf(
			"with the flag: exit %d, want 0\n%s",
			code,
			said,
		)
	}
}

// TestMemoryWatch_StressEndsWithExitFive is the manual check that a real,
// endless allocation ends the run with one line. It is gated: it fills the
// machine's memory to 95 percent, so it does not run in the default test run.
//
// Two scripts. "interpreter loop" appends to a list for ever, and "builtin
// loop" calls patch_json in a loop, so memory grows steadily through Go
// builtins. Both must end with exit 5 (decision Q44) and the one line "lark:
// memory limit reached", whether the run stops inside the grace or is
// abandoned after it. Package artifact's TestMemoryWatch_SmallScaleStress
// covers the builtin that never looks at its context, at small scale.
//
// A shape that doubles a document inside one patch_json call (40 copy
// operations) is not used: each doubling takes twice as long as the one before
// and, near 13 GB, the memory stops growing for minutes while the call is
// busy copying. It never reached the trip in 3 minutes (stress run of
// 2026-11-02 20:50), and it is not a steady grower. See the report, Fix
// round 4.
//
// A stress run is cut by progress, not by a short clock (_RunStress): it fails
// if available memory has not fallen for a minute, or after ten minutes in all.
// Run it with a go test -timeout of at least 25 minutes, for two such runs.
//
// To run it, on a machine where that is acceptable (a spare VM, or WSL2 with
// nothing else of value running):
//
//	LARK_STRESS=1 go test -count=1 -v ./cmd/lark/ \
//	    -run TestMemoryWatch_StressEndsWithExitFive
//
// Success: both cases pass in seconds to minutes and the log shows each exit code and the
// stderr tail. Failure is exit 2 with a Go "fatal error: out of memory", or the
// kernel's kill.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: the builtin case accepts exit 1 or 3, the walker being cooperative (C27)
//   - 2026-11-02 22:32: the builtin case calls patch_json in a loop, not one doubling call (C27)
//   - 2026-11-02 23:00: both shapes expect exit 5, the memory code (C27)
//   - 2026-11-02 23:00: stress runs end on lack of progress; four threads fill memory (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 16:54: runs with the run verb (C119, Q305)
func TestMemoryWatch_StressEndsWithExitFive(t *testing.T) {
	if os.Getenv("LARK_STRESS") != "1" {
		t.Skip("set LARK_STRESS=1 to fill memory to 95 percent; see the doc comment")
	}

	if memwatch.Check() != nil {
		t.Skip("the system offers no memory watch")
	}

	cases := []struct {
		name   string
		text   string
		accept []int
	}{
		{
			"interpreter loop",
			FILL_LIST,
			[]int{5},
		},
		{
			"builtin loop",
			STEADY_BUILTIN,
			[]int{5},
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					script = filepath.Join(
						t.TempDir(),
						"endless.star",
					)
					err = os.WriteFile(
						script,
						[]byte(item.text),
						SHARED,
					)
				)
				if err != nil {
					t.Fatal(err)
				}

				code, said := _RunStress(
					t,
					"run",
					"-memory",
					script,
				)
				t.Logf(
					"exit %d, stderr tail: %q",
					code,
					said[max(
						0,
						len(said)-300,
					):],
				)

				if !strings.Contains(
					said,
					"memory limit reached",
				) || !slices.Contains(
					item.accept,
					code,
				) {
					t.Fatalf(
						"exit %d, said %q; want %v and the limit text",
						code,
						said,
						item.accept,
					)
				}
			},
		)
	}
}

// TestAvailable_ReadsMemAvailable proves the reading the stress runs are cut by:
// on a system with /proc/meminfo it is a positive number below the machine's
// total memory.
//
// Revisions:
//   - 2026-11-02 23:00: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestAvailable_ReadsMemAvailable(t *testing.T) {
	if memwatch.Check() != nil {
		t.Skip("the system offers no /proc/meminfo")
	}

	got, ok := _Available()
	if !ok || got == 0 {
		t.Fatalf(
			"got %d, %v, want a positive reading",
			got,
			ok,
		)
	}
}

// _Limited runs the lark binary on script with flags, in a child shell that
// limits its address space to kilobytes (ulimit -v). A regression then ends that
// child with Go's "fatal error: out of memory", and never the machine. It
// returns the exit code and standard error.
//
// Revisions:
//   - 2026-11-03 21:55: initial creation (C46 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run verb (C119, Q305)
func _Limited(
	t *testing.T,
	kilobytes string,
	script string,
	flags ...string,
) (int, string) {
	t.Helper()

	var (
		stderr strings.Builder
		exit   *exec.ExitError
		code   int
		args   = append(
			append(
				[]string{
					"-c",
					"ulimit -v " + kilobytes +
						" && exec \"$0\" \"$@\"",
					BUILT,
					"run",
				},
				flags...,
			),
			script,
		)
		ctx, cancel = context.WithTimeout(
			t.Context(),
			RUN_LIMIT,
		)
		cmd = exec.CommandContext(
			ctx,
			"sh",
			args...,
		)
	)

	defer cancel()

	cmd.Stderr = &stderr
	cmd.WaitDelay = KILL_WAIT

	_Grouped(cmd)

	err := cmd.Run()
	if errors.As(
		err,
		&exit,
	) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return code, stderr.String()
}

// TestSizeGuard_B053RefusesInsteadOfCrashing runs the B053 script itself,
// hex.from_int(255, 1<<33), under ulimit -v 4000000 (about 4 GB of address
// space), with and without -memory. The guard counts the address-space limit
// (Q69), so 8 GiB does not fit however much memory the machine has free.
//
// Success: exit 1, standard error holds "hex.from_int: 8589934592 bytes: not
// enough free memory" and no "fatal error". Check: remove the memwatch.Room call
// in unpack.Counted and this test fails with the fatal error.
//
// Revisions:
//   - 2026-11-03 00:55: initial creation (C46)
//   - 2026-11-03 21:55: the literal 1<<33 under ulimit -v 4000000 (C46 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSizeGuard_B053RefusesInsteadOfCrashing(t *testing.T) {
	const WANT = "hex.from_int: 8589934592 bytes: not enough free memory"

	var (
		script = filepath.Join(
			t.TempDir(),
			"b053.star",
		)
		err = os.WriteFile(
			script,
			[]byte("def main():\n    print(hex.from_int(255, 1<<33))\n"),
			SHARED,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, flags := range map[string][]string{
		"watch off": {},
		"watch on":  {"-memory"},
	} {
		t.Run(
			name,
			func(t *testing.T) {
				code, said := _Limited(
					t,
					"4000000",
					script,
					flags...,
				)
				if code != 1 || !strings.Contains(
					said,
					WANT,
				) || strings.Contains(
					said,
					"fatal error",
				) {
					t.Fatalf(
						"exit %d, want 1 with %q, no fatal error\n%.400s",
						code,
						WANT,
						said,
					)
				}
			},
		)
	}
}

// TestSizeGuard_LinesRefusesAHugeLine runs file.lines over a sparse file of
// 6 GiB with no newline, under ulimit -v 2500000. The scanner's buffer would
// double until the allocation fails; the guard refuses first, and the run ends
// with the error and not with Go's "fatal error: out of memory".
//
// Success: exit 1, "not enough free memory" on standard error, no "fatal
// error". The sparse file takes no disk. The run holds at most about 1 GiB
// before the refusal. The test is skipped where the file system has no sparse
// files.
//
// Revisions:
//   - 2026-11-03 21:55: initial creation (C46 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSizeGuard_LinesRefusesAHugeLine(t *testing.T) {
	const SPARSE = 6 << 30

	var (
		dir = t.TempDir()
		big = filepath.Join(
			dir,
			"big.bin",
		)
		script = filepath.Join(
			dir,
			"lines.star",
		)
	)

	made, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}

	err = made.Truncate(SPARSE)
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

	err = os.WriteFile(
		script,
		[]byte("def main():\n    n = 0\n    for line in file.lines(\""+
			big+"\"):\n        n += len(line)\n    print(n)\n"),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	code, said := _Limited(
		t,
		"2500000",
		script,
	)
	if code != 1 || !strings.Contains(
		said,
		"not enough free memory",
	) || strings.Contains(
		said,
		"fatal error",
	) {
		t.Fatalf(
			"exit %d, want 1 with the refusal, no fatal error\n%.400s",
			code,
			said,
		)
	}
}

// TestFlags_Version proves the version verb prints one line, "lark <version>
// <go>", on standard output and exits 0, with standard error empty; and that it
// takes no argument, and that the old flag -version is no verb: both exit 2 with
// the TRY line, the second with a hint to the verb.
//
// Revisions:
//   - 2026-11-04 22:50: initial creation (C67)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: version is a verb that takes none (C119, Q305, Q307)
func TestFlags_Version(t *testing.T) {
	var stderr strings.Builder

	cmd := exec.Command(
		BUILT,
		"version",
	)
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf(
			"exit: %v, stderr %q",
			err,
			stderr.String(),
		)
	}

	if !regexp.MustCompile(`^lark \S+ go\S+\n$`).Match(out) {
		t.Fatalf(
			"standard output %q is not the version line",
			out,
		)
	}

	if stderr.Len() != 0 {
		t.Fatalf(
			"standard error %q, want empty",
			stderr.String(),
		)
	}

	for _, item := range []struct {
		name string
		args []string
		want string
	}{
		{
			"an argument",
			[]string{"version", "x"},
			"lark: wrong number of arguments (version takes none)\n",
		},
		{
			"the old flag",
			[]string{"-version"},
			"lark: unknown verb \"-version\" (did you mean version?)\n",
		},
		{
			"the old flag, double dash",
			[]string{"--version"},
			"lark: unknown verb \"--version\" (did you mean version?)\n",
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				code, said := _Run(
					t,
					item.args...,
				)

				if code != EXIT_FLAGS || said != item.want+"try lark help\n" {
					t.Fatalf(
						"exit %d, stderr %q, want exit %d and %q",
						code,
						said,
						EXIT_FLAGS,
						item.want+"try lark help\n",
					)
				}
			},
		)
	}
}

// TestFlags_TypoGetsOneSuggestion proves a misspelled flag of a verb is refused
// with exit 2 and, when a flag of that verb is close, the one suggestion after
// the flag package's own text. A flag nothing is close to gets the plain text. A
// misspelled verb gets the same (Q307).
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: flags of a verb, and the verbs themselves (C119, Q305, Q307)
func TestFlags_TypoGetsOneSuggestion(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // the first line of standard error
	}{
		{
			"one letter missing",
			[]string{"run", "-grph", "x"},
			"flag provided but not defined: -grph (did you mean -graph?)",
		},
		{
			"one letter changed",
			[]string{"run", "-lgs", "x"},
			"flag provided but not defined: -lgs (did you mean -logs?)",
		},
		{
			"a flag of another verb is not near",
			[]string{"compile", "-hold", "x"},
			"flag provided but not defined: -hold",
		},
		{
			"nothing close",
			[]string{"run", "-zebra", "x"},
			"flag provided but not defined: -zebra",
		},
		{
			"old one-letter name",
			[]string{"run", "-s", "x"},
			"flag provided but not defined: -s",
		},
		{
			"a verb, one letter missing",
			[]string{"compil", "x"},
			"lark: unknown verb \"compil\" (did you mean compile?)",
		},
		{
			"a verb, nothing close",
			[]string{"zebra", "x"},
			"lark: unknown verb \"zebra\"",
		},
		{
			"a file where a verb goes",
			[]string{"hello.star"},
			"lark: unknown verb \"hello.star\"",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				cmd := exec.Command(
					BUILT,
					tc.args...,
				)

				var stderr strings.Builder

				cmd.Stderr = &stderr

				err := cmd.Run()

				var exit *exec.ExitError
				if !errors.As(
					err,
					&exit,
				) || exit.ExitCode() != EXIT_FLAGS {
					t.Fatalf(
						"got %v, want exit %d",
						err,
						EXIT_FLAGS,
					)
				}

				first, _, _ := strings.Cut(
					stderr.String(),
					"\n",
				)
				if first != tc.want {
					t.Errorf(
						"got %q, want %q",
						first,
						tc.want,
					)
				}

				if strings.Count(
					stderr.String(),
					"did you mean",
				) > 1 {
					t.Errorf(
						"more than one suggestion: %q",
						stderr.String(),
					)
				}
			},
		)
	}
}

// TestFlags_StoppedJoinHasNoHint proves a run stopped by SIGINT while a join
// waits ends with the one line "script stopped" and exit 4, and that line holds
// no outcomes() hint. The command prints that line in place of the error, so
// this pins the CLI path; the library path is TestHints_NoHintForARunStop.
//
// Revisions:
//   - 2026-11-05 08:25: initial creation (C70 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run verb (C119, Q305)
func TestFlags_StoppedJoinHasNoHint(t *testing.T) {
	var (
		script = filepath.Join(
			t.TempDir(),
			"wait.star",
		)
		err error
	)

	err = os.WriteFile(
		script,
		[]byte("def w():\n    sleep(60000)\n\ndef main():\n"+
			"    print(\"up\")\n    join(spawn(w))\n"),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		BUILT,
		"run",
		script,
	)

	var stderr strings.Builder

	cmd.Stderr = &stderr

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// The line is printed before the join starts; the join then waits.
	buf := make(
		[]byte,
		256,
	)

	_, err = out.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	err = cmd.Process.Signal(os.Interrupt)
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Wait()

	var exit *exec.ExitError
	if !errors.As(
		err,
		&exit,
	) || exit.ExitCode() != 4 {
		t.Fatalf(
			"got %v, want exit 4",
			err,
		)
	}

	if strings.Contains(
		stderr.String(),
		"outcomes()",
	) || !strings.Contains(
		stderr.String(),
		"stopped",
	) {
		t.Errorf(
			"got %q",
			stderr.String(),
		)
	}
}

// TestBundle_SaysWhatItWrote proves compile ends with one line on standard
// error, naming the file in quotes, and that a bundle that cannot be written
// says no such line but one "lark:" line with the cause and exit 3, not "script
// failed" and exit 1 (B254): a path that is a directory, a directory that is not
// there, and /dev/full, which opens and takes no byte. The file name is the
// user's own text, so it is quoted (Q4).
//
// Revisions:
//   - 2026-11-05 16:44: initial creation (C82, Q166)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: compile -o; a failed write exits 3 with a lark: line (C119, B254, Q303)
func TestBundle_SaysWhatItWrote(t *testing.T) {
	var (
		dir    = t.TempDir()
		script = filepath.Join(
			dir,
			"hi.star",
		)
		bundle = filepath.Join(
			dir,
			"hi.bin",
		)
		missing = filepath.Join(
			dir,
			"no",
			"such",
			"hi.bin",
		)
	)

	err := os.WriteFile(
		script,
		[]byte("def main():\n    print(\"hi\")\n"),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		out  string
		want string
		code int
	}{
		{
			"written",
			bundle,
			"lark: wrote " + strconv.Quote(bundle) + "\n",
			0,
		},
		{
			"directory",
			dir,
			"lark: bundle " + strconv.Quote(dir) + ": is a directory\n",
			EXIT_NO_FILE,
		},
		{
			"missing directory",
			missing,
			"lark: bundle " + strconv.Quote(missing) + ": no such file or directory\n",
			EXIT_NO_FILE,
		},
		{
			"full device",
			"/dev/full",
			"lark: bundle \"/dev/full\": no space left on device\n",
			EXIT_NO_FILE,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if tc.out == "/dev/full" {
					_, err := os.Stat(tc.out)
					if err != nil {
						t.Skip("the system has no /dev/full")
					}
				}

				code, stderr := _Run(
					t,
					"compile",
					"-o",
					tc.out,
					script,
				)
				if code != tc.code {
					t.Fatalf(
						"exit %d, want %d (stderr %q)",
						code,
						tc.code,
						stderr,
					)
				}

				if stderr != tc.want {
					t.Fatalf(
						"stderr %q, want %q",
						stderr,
						tc.want,
					)
				}
			},
		)
	}
}

// TestShake_Demo is the demo of plan Q163 and Q164: a script declaring an
// argument nothing reads runs with no arguments, and convert lists only the
// argument the script uses.
//
// Revisions:
//   - 2026-11-05 17:15: initial creation (C44b, Q163, Q164)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:45: runs with the run and convert verbs (C119, Q305, Q306)
func TestShake_Demo(t *testing.T) {
	var (
		script = filepath.Join(
			t.TempDir(),
			"demo.star",
		)
		err = os.WriteFile(
			script,
			[]byte(
				"host = arg(\"host\", \"localhost\")\n"+
					"token = arg(\"token\")\n"+
					"def main():\n"+
					"    print(\"connect\", host)\n",
			),
			SHARED,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	code, stderr := _Run(
		t,
		"run",
		script,
	)
	if code != 0 {
		t.Fatalf(
			"exit %d, stderr %q, want 0 with no -args",
			code,
			stderr,
		)
	}

	out, err := exec.Command(
		BUILT,
		"convert",
		script,
	).Output()
	if err != nil {
		t.Fatalf(
			"convert: %v",
			err,
		)
	}

	if !strings.Contains(
		string(out),
		`"host"`,
	) || strings.Contains(
		string(out),
		`"token"`,
	) {
		t.Fatalf(
			"convert lists\n%s\nwant host and not token",
			out,
		)
	}
}
