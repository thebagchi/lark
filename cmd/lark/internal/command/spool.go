package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"syscall"
)

// Spool is the file a run's printed lines go through, behind a buffer of one
// page.
//
// Two jobs, one type. A streaming spool (the default) is the buffered copy of
// the lines in the -logs file: standard output gets each line as it is printed,
// from the caller, and the spool only writes the file. A holding spool (-hold)
// is the only place the lines go while the run lasts: _Close writes them to out
// when the run is over, read back from the file, so the memory they take is the
// page and not the lines. The lines on disk stay in the order the log handler
// wrote them, which is the order of a stream.
//
// The mutex is the spool's own: the handler locks around each write, but _Close
// comes from another goroutine, and a bufio.Writer is not safe for two at once.
// After _Close a write is dropped without an error, because a thread can print
// after the run returned; the handler ignores a write error anyway.
//
// A nil *Spool is a run with no transcript: its methods do nothing.
//
// Revisions:
//   - 2026-11-03 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Spool to Spool, for the tests in
//     tests/ (C120b, Q315, Q314)
type Spool struct {
	mutex  sync.Mutex
	file   *os.File
	buf    *bufio.Writer
	size   int64
	out    io.Writer
	hold   bool
	remove bool
	closed bool
}

// NewSpool is a spool over file, which must be open for reading as well when
// hold is true. remove asks _Close to delete the file, as a temporary one is.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C42)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _NewSpool to NewSpool, for the tests in
//     tests/ (C120b, Q315, Q314)
func NewSpool(
	file *os.File,
	hold bool,
	remove bool,
	out io.Writer,
) *Spool {
	return &Spool{
		file: file,
		buf: bufio.NewWriterSize(
			file,
			os.Getpagesize(),
		),
		out:    out,
		hold:   hold,
		remove: remove,
	}
}

// Write puts p behind the page buffer. It reports every byte written once the
// spool is closed, and the real count and error before.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C42)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func (s *Spool) Write(p []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return len(p), nil
	}

	n, err := s.buf.Write(p)
	s.size += int64(n)

	return n, err
}

// Close ends the spool once: a holding spool writes its lines to out, then the
// buffer is flushed, the file closed and, for a temporary file, removed. It is
// the one place that writes the held lines, so the end of a run, a stop and the
// memory watch's abandon all use it, and what was printed is not lost to an
// exit. The faults are joined; a failed copy to out does not stop the removal.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C42)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Close to Close, for the tests in
//     tests/ (C120b, Q315, Q314)
func (s *Spool) Close() error {
	if s == nil {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	err := s.buf.Flush()
	if err == nil && s.hold {
		_, err = io.Copy(
			s.out,
			io.NewSectionReader(
				s.file,
				0,
				s.size,
			),
		)
	}

	err = errors.Join(
		err,
		s.file.Close(),
	)

	if s.remove {
		removed := os.Remove(s.file.Name())
		if removed != nil && !errors.Is(
			removed,
			fs.ErrNotExist,
		) {
			err = errors.Join(
				err,
				removed,
			)
		}
	}

	if err != nil {
		return fmt.Errorf(
			"log %s: %w",
			s.file.Name(),
			err,
		)
	}

	return nil
}

// _Pipe is standard output as the spool sees it: every write goes through, and
// the first that fails because the reader closed its end (EPIPE) calls broken,
// which stops the run, so the close that follows still runs, and the file is
// whole. Without it the Go runtime ends the process by SIGPIPE on a write to
// descriptor 1, which skips every close (os/signal, "SIGPIPE"). The error is
// returned as it came: the log handler drops it.
//
// Revisions:
//   - 2026-11-03 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
type _Pipe struct {
	w      io.Writer
	broken func()
	once   sync.Once
}

// Write writes p to the wrapped writer, and calls broken once on EPIPE.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C42)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func (p *_Pipe) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if errors.Is(
		err,
		syscall.EPIPE,
	) && p.broken != nil {
		p.once.Do(p.broken)
	}

	return n, err
}
