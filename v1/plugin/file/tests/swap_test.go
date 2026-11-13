package tests

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// SWAPS is how many times each writer is called while its path is swapped.
	// The window between a stat and an open is a few microseconds wide, so the
	// count is large; a call that is not caught costs about a millisecond.
	//
	// Revisions:
	//   - 2026-11-11 00:35: initial creation (C122, B266)
	SWAPS = 400
)

// TestWrite_APathSwappedForAPipeNeverBlocks proves write, append, writelines
// and appendlines check the file they opened, not the path they looked at: a
// path that is a regular file at the stat and a named pipe at the open ends the
// call with ErrRegular, as a path that was a pipe all along does, and does not
// wait for a reader that never comes (B266, B241).
//
// Revisions:
//   - 2026-11-11 00:35: initial creation (C122, B266)
func TestWrite_APathSwappedForAPipeNeverBlocks(t *testing.T) {
	_NeverBlocks(
		t,
		[]string{
			`file.write(swap, "a")`,
			`file.append(swap, "a")`,
			`file.writelines(swap, ["a"])`,
			`file.appendlines(swap, ["a"])`,
		},
	)
}

// TestRead_APathSwappedForAPipeNeverBlocks proves read, bytes and the walk of
// lines open the file once and check what they opened: a path that is a
// regular file at the stat and a named pipe at the open ends the call with
// ErrRegular, and does not wait for a writer that never comes (B272, B266).
//
// Revisions:
//   - 2026-11-11 06:42: initial creation (C122, B272)
func TestRead_APathSwappedForAPipeNeverBlocks(t *testing.T) {
	_NeverBlocks(
		t,
		[]string{
			`file.read(swap)`,
			`file.bytes(swap)`,
			`[line for line in file.lines(swap)]`,
		},
	)
}

// _NeverBlocks runs each expression SWAPS times while the path swap is
// repointed between a named pipe and a regular file as fast as one goroutine
// can. A call may succeed or be refused with ErrRegular, and either is right.
// With the check before the open, a call that meets the swap in the window
// blocks in open(2), so the deadline turns the defect into a failure. The test
// then opens the pipe, so the blocked goroutine ends.
//
// Revisions:
//   - 2026-11-11 00:35: initial creation, from the body of the write test
//     (C122, B266, B272)
//   - 2026-11-12 17:20: makes and releases the pipe with _Pipe and _Release, which skip
//     where there are no pipes (C128, B277)
func _NeverBlocks(
	t *testing.T,
	expressions []string,
) {
	t.Helper()

	var (
		root = t.TempDir()
		fifo = filepath.Join(
			root,
			"pipe",
		)
		plain = filepath.Join(
			root,
			"plain",
		)
		swap = filepath.Join(
			root,
			"swap",
		)
	)

	_Pipe(
		t,
		fifo,
	)

	err := os.WriteFile(
		plain,
		nil,
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(
		plain,
		swap,
	)
	if err != nil {
		t.Fatal(err)
	}

	env := _Environment(t)
	env["swap"] = starlark.String(swap)

	var (
		stop    = make(chan struct{})
		stopped sync.WaitGroup
	)

	stopped.Go(func() {
		_Swapping(
			root,
			swap,
			[]string{fifo, plain},
			stop,
		)
	})

	t.Cleanup(func() {
		close(stop)
		stopped.Wait()
	})

	for _, expression := range expressions {
		for range SWAPS {
			done := make(
				chan error,
				1,
			)

			go func() {
				_, err := starlark.EvalOptions(
					common.OPTIONS,
					&starlark.Thread{
						Name: SCRIPT,
					},
					SCRIPT,
					expression,
					env,
				)
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil && !errors.Is(
					err,
					larkfile.ErrRegular,
				) {
					t.Fatalf(
						"%s: got %v, want nothing or ErrRegular",
						expression,
						err,
					)
				}

			case <-time.After(DEADLINE):
				_Release(fifo)

				t.Fatalf(
					"%s on a path swapped for a pipe did not return",
					expression,
				)
			}
		}
	}
}

// _Swapping points the symbolic link at each of targets in turn, until stop is
// closed. The link is made beside the path and renamed over it, so the path
// names one of the targets at every moment.
//
// Revisions:
//   - 2026-11-11 00:35: initial creation (C122, B266)
func _Swapping(
	dir string,
	path string,
	targets []string,
	stop chan struct{},
) {
	spare := filepath.Join(
		dir,
		"spare",
	)

	for turn := 0; ; turn++ {
		select {
		case <-stop:
			return

		default:
		}

		_ = os.Remove(spare)

		err := os.Symlink(
			targets[turn%len(targets)],
			spare,
		)
		if err != nil {
			continue
		}

		_ = os.Rename(
			spare,
			path,
		)
	}
}
