package observe

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// LINE is how a logged line is laid out: the lane that printed it, two
	// spaces, the line. One format in one place, because two hosts write
	// these files and a transcript that differed between them could not be
	// diffed against another run.
	LINE = "%s  %s\n"

	// SUFFIX is what a log file is called after the run or the script it
	// belongs to.
	SUFFIX = ".log"

	// DIRECTORY is the mode a log directory is made with, and LOG_FILE the
	// mode of the file itself.
	DIRECTORY = 0o750
	LOG_FILE  = 0o640
)

// Log is one run's printed output, in a file of its own.
//
// It is a scheduler.Reporter that hears only the printing, so a host hands one
// to a run the way it hands over any reporter. A host that wants the lines
// somewhere else as well composes: a reporter of its own holding one of these
// and doing both, which is what cmd/lark does to keep printing to standard
// output.
//
// Safe for any number of threads. A run prints from every goroutine it
// started, and a line that interleaved with another would be a transcript of
// neither.
type Log struct {
	guard sync.Mutex
	file  *os.File
}

// NewLog opens path, making its directory if it is not there.
//
// Truncating, because a file is one run's output and a run starts empty.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func NewLog(path string) (*Log, error) {
	err := os.MkdirAll(filepath.Dir(path), DIRECTORY)
	if err != nil {
		return nil, fmt.Errorf("log %s: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, LOG_FILE)
	if err != nil {
		return nil, fmt.Errorf("log %s: %w", path, err)
	}

	return &Log{file: file}, nil
}

// Started is empty: a log carries what a run printed, not what it did.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (l *Log) Started(thread string, name string, attempt int32) {
	// Empty
}

// Ended is empty, for the same reason as Started.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (l *Log) Ended(thread string, name string, err error) {
	// Empty
}

// Printed writes one line, with the lane that printed it in front.
//
// A failure to write is dropped rather than raised, and that is the one thing
// here worth arguing with. A reporter that raised would end the run, so a full
// disk would stop a workflow that was otherwise working; and there is nowhere
// to report it to, because the channel that failed is the one a report would
// travel on. Close is where a caller hears that the file was not what it
// should be.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (l *Log) Printed(thread string, msg string) {
	l.guard.Lock()
	defer l.guard.Unlock()

	_, _ = fmt.Fprintf(l.file, LINE, thread, msg)
}

// Close finishes the file. A log is closed once, by whoever opened it.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (l *Log) Close() error {
	l.guard.Lock()
	defer l.guard.Unlock()

	err := l.file.Close()
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}

	return nil
}
