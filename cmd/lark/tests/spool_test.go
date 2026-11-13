package tests

// This file tests the spool that holds the lines of a held run: it holds them,
// closes once, drops later writes, keeps every line from concurrent writers and
// bounds its memory. The binary reaches the spool only through the whole run,
// which cannot fix a write race, a closed spool or the heap bound (CLAUDE.md
// 1.8.1; C65a).
//
// Revisions:
//   - 2026-11-08 22:49: header states why the file is internal (C65a)
//   - 2026-11-10 01:37: moved to tests/ from spool_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

const (
	// BIG_LINES is how many lines the memory test writes, and BIG_GROWTH how
	// much the heap may grow while it does. A line is 56 bytes here, so the
	// lines are 56 MB, 56 times the allowance.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BIG_LINES  = 1_000_000
	BIG_GROWTH = 1 << 20
)

// _Opening is a spool over a new file in a new directory, holding or not, with
// out as its standard output, and the path of the file.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Opening(
	t *testing.T,
	hold bool,
	remove bool,
	out io.Writer,
) (*command.Spool, string) {
	t.Helper()

	var (
		name = filepath.Join(
			t.TempDir(),
			"spool.log",
		)
		file, err = os.OpenFile(
			name,
			os.O_CREATE|os.O_RDWR|os.O_TRUNC,
			command.LOG_FILE,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	return command.NewSpool(
		file,
		hold,
		remove,
		out,
	), name
}

// TestSpool_Holds proves a holding spool gives its lines to out only at the
// close, all of them and in the order written, and a streaming spool gives
// none: its caller writes the stream. Both leave the whole file on disk, unless
// asked to remove it.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_Holds(t *testing.T) {
	cases := []struct {
		name   string
		hold   bool
		remove bool
		out    string
		file   bool // the file is still there after the close
	}{
		{
			"held, kept",
			true,
			false,
			"one\ntwo\nthree\n",
			true,
		},
		{
			"held, removed",
			true,
			true,
			"one\ntwo\nthree\n",
			false,
		},
		{
			"streamed, kept",
			false,
			false,
			"",
			true,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					out         bytes.Buffer
					spool, name = _Opening(
						t,
						tc.hold,
						tc.remove,
						&out,
					)
				)

				for _, line := range []string{"one\n", "two\n", "three\n"} {
					_, err := spool.Write([]byte(line))
					if err != nil {
						t.Fatal(err)
					}

					if out.Len() != 0 {
						t.Fatalf(
							"out holds %q before the close",
							out.String(),
						)
					}
				}

				err := spool.Close()
				if err != nil {
					t.Fatal(err)
				}

				if out.String() != tc.out {
					t.Fatalf(
						"out %q, want %q",
						out.String(),
						tc.out,
					)
				}

				raw, err := os.ReadFile(name)
				if tc.file {
					if err != nil || string(raw) != "one\ntwo\nthree\n" {
						t.Fatalf(
							"file %q (%v), want the three lines",
							raw,
							err,
						)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf(
						"file still there: %v",
						err,
					)
				}
			},
		)
	}
}

// TestSpool_CloseIsOnceAndWritesAfterAreDropped proves a second close does
// nothing and a line written after the close is dropped without an error or a
// panic: a thread can print after the run returned.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_CloseIsOnceAndWritesAfterAreDropped(t *testing.T) {
	var (
		out         bytes.Buffer
		spool, name = _Opening(
			t,
			true,
			false,
			&out,
		)
	)

	_, err := spool.Write([]byte("a\n"))
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		err = spool.Close()
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := spool.Write([]byte("late\n"))
	if err != nil || n != 5 {
		t.Fatalf(
			"late write gave %d, %v",
			n,
			err,
		)
	}

	raw, err := os.ReadFile(name)
	if err != nil || string(raw) != "a\n" || out.String() != "a\n" {
		t.Fatalf(
			"file %q out %q (%v), want only a",
			raw,
			out.String(),
			err,
		)
	}
}

// TestSpool_NilIsNothing proves a nil spool, the run with no transcript, closes
// without a fault.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_NilIsNothing(t *testing.T) {
	var spool *command.Spool

	err := spool.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestSpool_ConcurrentWritersKeepEveryLine proves eight writers at once lose no
// line and keep each writer's order. Run with -race.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_ConcurrentWritersKeepEveryLine(t *testing.T) {
	const (
		WRITERS = 8
		LINES   = 2000
	)

	var (
		out      bytes.Buffer
		group    sync.WaitGroup
		spool, _ = _Opening(
			t,
			true,
			true,
			&out,
		)
	)

	for writer := range WRITERS {
		group.Add(1)

		go func() {
			defer group.Done()

			for line := range LINES {
				_, _ = fmt.Fprintf(
					spool,
					"%d %d\n",
					writer,
					line,
				)
			}
		}()
	}

	group.Wait()

	err := spool.Close()
	if err != nil {
		t.Fatal(err)
	}

	var (
		lines = strings.Split(
			strings.TrimSuffix(
				out.String(),
				"\n",
			),
			"\n",
		)
		next = make(
			[]int,
			WRITERS,
		)
	)

	if len(lines) != WRITERS*LINES {
		t.Fatalf(
			"got %d lines, want %d",
			len(lines),
			WRITERS*LINES,
		)
	}

	for _, line := range lines {
		var writer, number int

		_, err = fmt.Sscanf(
			line,
			"%d %d",
			&writer,
			&number,
		)
		if err != nil || number != next[writer] {
			t.Fatalf(
				"line %q out of order (want %d): %v",
				line,
				next[writer],
				err,
			)
		}

		next[writer]++
	}
}

// TestSpool_MemoryIsBounded proves a million lines held cost the heap one page,
// not the lines: the spool is a file. The heap is read after two collections
// before and after, the lines being 56 MB.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_MemoryIsBounded(t *testing.T) {
	spool, _ := _Opening(
		t,
		true,
		true,
		io.Discard,
	)

	var before, after runtime.MemStats

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)

	line := []byte("0123456789012345678901234567890123456789012345678901234\n")

	for range BIG_LINES {
		_, err := spool.Write(line)
		if err != nil {
			t.Fatal(err)
		}
	}

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)

	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > BIG_GROWTH {
		t.Fatalf(
			"heap grew %d bytes for %d lines, allowed %d",
			growth,
			BIG_LINES,
			BIG_GROWTH,
		)
	}

	err := spool.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestSpool_PageSizedBuffer proves nothing reaches the file until a page is
// full: after one byte the file is empty, after more than a page it holds a
// whole number of pages.
//
// Revisions:
//   - 2026-11-03 00:35: initial creation (C42)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpool_PageSizedBuffer(t *testing.T) {
	var (
		spool, name = _Opening(
			t,
			false,
			false,
			io.Discard,
		)
		_, err = spool.Write([]byte("x"))
	)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(name)
	if err != nil || info.Size() != 0 {
		t.Fatalf(
			"file holds %d bytes after one byte (%v), want 0",
			info.Size(),
			err,
		)
	}

	_, err = spool.Write(bytes.Repeat(
		[]byte("y"),
		os.Getpagesize(),
	))
	if err != nil {
		t.Fatal(err)
	}

	info, err = os.Stat(name)
	if err != nil || info.Size() != int64(os.Getpagesize()) {
		t.Fatalf(
			"file holds %d bytes, want one page (%v)",
			info.Size(),
			err,
		)
	}

	err = spool.Close()
	if err != nil {
		t.Fatal(err)
	}
}
