package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote"
)

const (
	// ROOT is the module root, seen from this tests directory.
	//
	// Revisions:
	//   - 2026-11-10 00:25: one more ".." after the move to tests/ (C120a, Q311)
	ROOT = "../../../../.."

	// DEADLINE bounds the wait for the token file, so a plugin that never
	// ran fails the test and does not hang it.
	//
	// Revisions:
	//   - 2026-11-08 13:08: initial creation (C110, B226)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DEADLINE = 30 * time.Second
)

// _Temp is a ListenTemp listener, and the one directory it made.
//
// TMPDIR points at an empty directory first, so the only entry in it is the
// directory ListenTemp made.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Temp(t *testing.T) (common.Listener, string) {
	t.Helper()

	t.Setenv(
		"TMPDIR",
		t.TempDir(),
	)

	listener, err := remote.ListenTemp()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = listener.Close()
	})

	entries, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Fatalf(
			"%d entries in TMPDIR, want 1",
			len(entries),
		)
	}

	return listener, filepath.Join(
		os.Getenv("TMPDIR"),
		entries[0].Name(),
	)
}

// TestListenTemp_ServesAClockPlugin is a round trip: the listener starts the
// clock plugin, hands it a socket and a token it made itself, and a script
// calls the plugin.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestListenTemp_ServesAClockPlugin(t *testing.T) {
	var (
		listener, _ = _Temp(t)
		plugins     = t.TempDir()
		build       = exec.Command(
			"go",
			"build",
			"-o",
			filepath.Join(
				plugins,
				"lark-clock.bin",
			),
			"./cmd/clock",
		)
	)

	build.Dir = ROOT

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"building the plugin: %v\n%s",
			err,
			out,
		)
	}

	loading, err := listener.Load(
		plugins,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(loading.Loaded) != 1 || len(loading.Skipped) != 0 {
		t.Fatalf(
			"loaded %v, skipped %v",
			loading.Loaded,
			loading.Skipped,
		)
	}

	got, err := _Ran(
		t,
		listener,
		"def main():\n    return [type(clock.now()), clock.add(2, 3)]\n",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != `["string", 5]` {
		t.Fatalf(
			"got %s",
			got,
		)
	}
}

// TestListenTemp_TheDirectoryIsPrivate proves the mode is 0700: no other user
// can reach the socket.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestListenTemp_TheDirectoryIsPrivate(t *testing.T) {
	var (
		_, dir    = _Temp(t)
		info, err = os.Stat(dir)
	)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o700 {
		t.Fatalf(
			"mode %v, want 0700",
			info.Mode().Perm(),
		)
	}
}

// TestListenTemp_CloseRemovesTheDirectory proves Close removes the directory
// and the socket, and that a second Close is safe.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestListenTemp_CloseRemovesTheDirectory(t *testing.T) {
	var (
		listener, dir = _Temp(t)
		_, err        = os.Stat(
			filepath.Join(
				dir,
				"plugins.sock",
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"no socket before Close: %v",
			err,
		)
	}

	for round := range 2 {
		err = listener.Close()
		if err != nil {
			t.Fatalf(
				"Close %d: %v",
				round+1,
				err,
			)
		}

		_, err = os.Stat(dir)
		if !os.IsNotExist(err) {
			t.Fatalf(
				"after Close %d the directory is there: %v",
				round+1,
				err,
			)
		}
	}
}

// TestListenTemp_AFailedListenLeavesNoDirectory forces Listen to fail with a
// socket path over the unix limit, and checks that the directory is removed.
//
// Revisions:
//   - 2026-11-08 03:08: initial creation (C84 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestListenTemp_AFailedListenLeavesNoDirectory(t *testing.T) {
	var (
		long = filepath.Join(
			t.TempDir(),
			strings.Repeat(
				"d",
				150,
			),
		)
		err = os.MkdirAll(
			long,
			0o700,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(
		"TMPDIR",
		long,
	)

	_, err = remote.ListenTemp()
	if err == nil {
		t.Fatal("got no error, want one for a socket path that is too long")
	}

	entries, readErr := os.ReadDir(long)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if len(entries) != 0 {
		t.Fatalf(
			"%d entries left in TMPDIR, want 0",
			len(entries),
		)
	}
}

// _Token starts one shell plugin on listener. The plugin writes the token it
// was given to a file beside itself, through a rename, so the file is never
// seen half written. _Token waits for the file, closes the listener, and gives
// the token.
//
// The plugin never announces itself, so Load would wait the whole of
// ANNOUNCED. _Token waits on the token file instead, then Close ends Load with
// io.EOF (C109). Load may return before the file is there (as soon as the
// process starts, or when it fails), so the wait does not stop on Load. It
// gives up after DEADLINE, so a plugin that never ran fails the test and does
// not hang it.
//
// Callers run one after the other. The script file is written and closed
// before Load runs, and no other Load forks while it is written: a fork that
// holds the write descriptor makes exec fail with "text file busy" (ETXTBSY).
// That was the cause of B226.
//
// Revisions:
//   - 2026-11-08 10:25: initial creation (C84 fix round 1)
//   - 2026-11-08 13:08: waits on the token file, closes the listener, and runs one
//     plugin at a time (C110, B226)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Token(
	t *testing.T,
	listener common.Listener,
) string {
	t.Helper()

	var (
		plugins = t.TempDir()
		named   = filepath.Join(
			plugins,
			"lark-token.bin",
		)
		script = "#!/bin/sh\n" +
			"echo \"$" + common.TOKEN_ENV + "\" > \"$0.part\"\n" +
			"mv \"$0.part\" \"$0.token\"\n"
		done = make(chan struct{})
	)

	err := os.WriteFile(
		named,
		[]byte(script),
		0o700,
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer close(done)

		_, _ = listener.Load(
			plugins,
			"",
		)
	}()

	var (
		tick  = time.NewTicker(10 * time.Millisecond)
		limit = time.After(DEADLINE)
	)
	defer tick.Stop()

	for {
		got, err := os.ReadFile(named + ".token")
		if err == nil {
			_ = listener.Close()

			<-done

			return strings.TrimSpace(string(got))
		}

		select {
		case <-limit:
			_ = listener.Close()

			<-done

			t.Fatal("the plugin wrote no token file")

		case <-tick.C:
		}
	}
}

// TestListenTemp_EachCallMakesANewToken proves two listeners give their
// plugins different tokens, 32 hex characters each (16 bytes).
//
// Revisions:
//   - 2026-11-08 10:25: initial creation (C84 fix round 1)
//   - 2026-11-08 13:08: the helper waits on the token file, not ANNOUNCED (C110, B226)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestListenTemp_EachCallMakesANewToken(t *testing.T) {
	var (
		first, _  = _Temp(t)
		second, _ = _Temp(t)
	)

	var (
		a = _Token(
			t,
			first,
		)
		b = _Token(
			t,
			second,
		)
	)

	if a == b {
		t.Fatal("two listeners gave the same token")
	}

	if len(a) != 32 || len(b) != 32 {
		t.Fatalf(
			"token lengths %d and %d, want 32 (16 bytes in hex)",
			len(a),
			len(b),
		)
	}
}
