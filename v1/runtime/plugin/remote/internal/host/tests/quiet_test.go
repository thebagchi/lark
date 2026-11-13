package tests

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
)

const (
	// QUIET_BOUND is how long after Close a Load on a quiet plugin may take to
	// return. ANNOUNCED is 10 s, so the bound is well under it.
	QUIET_BOUND = 2 * time.Second
)

// _Result is what a Load returned, handed back from its goroutine.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Result struct {
	err error
}

// TestLoad_CloseEndsAQuietLoad proves Close ends a Load that waits for a plugin
// that never announces itself. Load returns io.EOF well before ANNOUNCED, and
// the quiet process is gone when Close returns.
//
// The plugin is a shell script. It writes its pid, then replaces itself with a
// sleep, so the pid is the process Close must kill.
//
// Before the fix Load polled until ANNOUNCED, so the test failed with a
// timeout of QUIET_BOUND (bug B221).
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:31: checks the process with _Reaped, which skips without signal 0
//     (C128, B284)
func TestLoad_CloseEndsAQuietLoad(t *testing.T) {
	var (
		plugins = t.TempDir()
		named   = filepath.Join(
			plugins,
			"lark-quiet.bin",
		)
		script = "#!/bin/sh\necho $$ > \"$0.pid\"\nexec sleep 60\n"
	)

	err := os.WriteFile(
		named,
		[]byte(script),
		0o700,
	)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := host.Listen(
		filepath.Join(
			t.TempDir(),
			"s",
		),
		"token",
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = listener.Close()
	})

	var (
		loaded = make(
			chan _Result,
			1,
		)
		closed = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := listener.Load(
			plugins,
			"",
		)
		loaded <- _Result{
			err,
		}
	}()

	pid := _WaitPid(
		t,
		named+".pid",
	)

	go func() {
		closed <- listener.Close()
	}()

	select {
	case got := <-loaded:
		if !errors.Is(
			got.err,
			io.EOF,
		) {
			t.Fatalf(
				"Load error %v, want io.EOF",
				got.err,
			)
		}

	case <-time.After(QUIET_BOUND):
		t.Fatalf(
			"Load still waiting %v after Close",
			QUIET_BOUND,
		)
	}

	err = <-closed
	if err != nil {
		t.Fatal(err)
	}

	// Close has waited for the process, so signal 0 finds no process.
	_Reaped(
		t,
		pid,
	)

	// A second Close stays safe.
	err = listener.Close()
	if err != nil {
		t.Fatalf(
			"second Close: %v",
			err,
		)
	}
}

// TestLoad_AfterCloseIsRefused proves a Load on a closed listener returns
// io.EOF at once and starts nothing.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_AfterCloseIsRefused(t *testing.T) {
	var (
		plugins = t.TempDir()
		named   = filepath.Join(
			plugins,
			"lark-late.bin",
		)
	)

	err := os.WriteFile(
		named,
		[]byte("#!/bin/sh\ntouch \"$0.ran\"\nexec sleep 60\n"),
		0o700,
	)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := host.Listen(
		filepath.Join(
			t.TempDir(),
			"s",
		),
		"token",
	)
	if err != nil {
		t.Fatal(err)
	}

	err = listener.Close()
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()

	_, err = listener.Load(
		plugins,
		"",
	)
	if !errors.Is(
		err,
		io.EOF,
	) {
		t.Fatalf(
			"Load error %v, want io.EOF",
			err,
		)
	}

	if time.Since(start) > QUIET_BOUND {
		t.Fatalf(
			"Load took %v",
			time.Since(start),
		)
	}

	_, err = os.Stat(named + ".ran")
	if err == nil {
		t.Fatal("a plugin ran after Close")
	}
}

// _WaitPid reads the pid the plugin wrote to file, waiting for it.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _WaitPid(
	t *testing.T,
	file string,
) int {
	t.Helper()

	deadline := time.Now().Add(host.ANNOUNCED)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(file)
		if err == nil && strings.HasSuffix(
			string(raw),
			"\n",
		) {
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(err)
			}

			return pid
		}

		time.Sleep(host.POLL)
	}

	t.Fatal("the plugin never started")

	return 0
}
