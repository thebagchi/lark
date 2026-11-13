package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Output is the -graph file a run can be asked to leave: one JSON Patch a line,
// each change the run makes to its graph (Q304).
//
// The file is opened before the script runs, so a path that cannot be written is
// refused before the script has done anything, and it is emptied at once, being
// a stream. A reader who applies the lines in order to {} has the graph the run
// ended with; the status changes are lines of it, as every other change is.
//
// The lines are written as the run makes them, one write for each line. Under
// -hold they go to a spool in os.TempDir() instead, and _Settle writes them to the
// file when the run is over, as the held lines of the script are written.
//
// A graph path of GRAPH_STDOUT (-) is no file: the lines go to standard output
// (decision Q191).
//
// A zero Output asks for no file.
//
// Revisions:
//   - 2026-11-02 21:22: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Output to Output, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: one file of patches, streamed or held; the changes file, the temporary
//     file and the rename are gone (C119, Q304, B255)
type Output struct {
	mutex   sync.Mutex
	path    string
	file    *os.File
	spool   *Spool
	fault   error
	settled bool
	closed  bool
	stdout  bool
}

// Opened is the file asked for, opened, or an error naming the path that
// could not be.
//
// The directory of a path is never made: it is a file that is asked for, as
// with compile -o, and a directory that is not there is a path that is wrong.
// The file is opened at the path itself, so that the real error of a device or
// a full disk is the one reported: /dev/full opens, and its writes say no space
// (B255).
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-05 21:55: names the user's path, quoted and bounded, not the
//     temporary file's (B116, Q4); a path of - is standard output (C80, Q191)
//   - 2026-11-05 22:20: names the changes path the same way (B127)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Opened to Opened, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: opens the one file at its path, with no temporary file beside it; holds
//     the lines in a spool under -hold (C119, Q304, B255)
func Opened(asked *Asked) (*Output, error) {
	held := new(Output)

	if asked.Graph == "" {
		return held, nil
	}

	held.path = asked.Graph

	if asked.Graph == GRAPH_STDOUT {
		held.file = os.Stdout
		held.stdout = true
	} else {
		file, err := os.OpenFile(
			asked.Graph,
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			GRAPH_FILE,
		)
		if err != nil {
			return nil, _Failed(
				GRAPH_FLAG,
				asked.Graph,
				err,
			)
		}

		held.file = file
	}

	if !asked.Hold {
		return held, nil
	}

	temp, err := os.CreateTemp(
		os.TempDir(),
		HELD_PATTERN,
	)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf(
				"graph %s: %w",
				os.TempDir(),
				err,
			),
			held.Close(),
		)
	}

	held.spool = NewSpool(
		temp,
		true,
		true,
		held.file,
	)

	return held, nil
}

// _Failed is the error of a graph path that could not be written: the word,
// the path the user gave, quoted and cut at 100 bytes (Q4), and what went wrong.
//
// A file system error names the file it met, and the user gave the path, so the
// name is cut out. The underlying error is kept, so errors.Is still finds it.
//
// Revisions:
//   - 2026-11-05 21:55: initial creation (C80, B116)
//   - 2026-11-05 22:20: serves the changes path too (B127)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
//   - 2026-11-10 16:54: the file system error names the file the user gave, not a temporary
//     one (C119, Q304)
func _Failed(
	noun string,
	path string,
	err error,
) error {
	var (
		pathed *fs.PathError
		linked *os.LinkError
	)

	switch {
	case errors.As(
		err,
		&pathed,
	):
		err = pathed.Err

	case errors.As(
		err,
		&linked,
	):
		err = linked.Err
	}

	return fmt.Errorf(
		"%s %s: %w",
		noun,
		unpack.QuotePrefix(path),
		err,
	)
}

// Watch is the function a run tells its changes to, or nil when no -graph file
// was asked for.
//
// Each change is one line, the JSON Patch that makes it as an array of
// operations, compact. The first write that fails is kept and later changes are
// dropped: the run goes on, because a script is not stopped for a full disk,
// and Fault tells main afterwards. A change that comes after Close is dropped.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-05 22:20: a write error names the user's path, quoted (B127)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Watch to Watch, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: the lines of the -graph file; they go to the spool under -hold
//     (C119, Q304)
func (o *Output) Watch() func(change *workflowpb.Change) {
	if o.file == nil {
		return nil
	}

	var into io.Writer = o.file
	if o.spool != nil {
		into = o.spool
	}

	return func(change *workflowpb.Change) {
		o.mutex.Lock()
		defer o.mutex.Unlock()

		if o.fault != nil || o.closed {
			return
		}

		line, err := _Patch(change)
		if err == nil {
			_, err = into.Write(line)
		}

		if err != nil {
			o.fault = _Failed(
				GRAPH_FLAG,
				o.path,
				err,
			)
		}
	}
}

// _Patch is a change as one line of the -graph file.
//
// Each operation through protojson, whose spacing varies between builds, then
// compacted, so the line is the same bytes in every build.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
//   - 2026-11-10 16:54: a line of the -graph file (C119, Q304)
func _Patch(change *workflowpb.Change) ([]byte, error) {
	var line bytes.Buffer

	line.WriteByte('[')

	for idx, op := range change.GetOperations() {
		raw, err := protojson.Marshal(op)
		if err != nil {
			return nil, err
		}

		if idx > 0 {
			line.WriteByte(',')
		}

		err = json.Compact(
			&line,
			raw,
		)
		if err != nil {
			return nil, err
		}
	}

	line.WriteString("]\n")

	return line.Bytes(), nil
}

// Fault is the first thing that went wrong writing the file, or nil.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Fault to Fault, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: reads under the lock the lines are written under (C119, Q304)
func (o *Output) Fault() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.fault
}

// Close settles the file, closes it, and removes the spool of a run that held
// its lines. Closing a file already closed is not an error here. A run that did
// not begin, because the script did not compile, leaves the file empty.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Close to Close, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: closes the one file, standard output excepted (C119, Q304)
func (o *Output) Close() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.closed {
		return nil
	}

	o.closed = true

	o._Settled()

	if o.file == nil || o.stdout {
		return nil
	}

	return o.file.Close()
}

// _Settle writes the lines a run held to the -graph file. Called once the run
// is over; Close calls it too, so a run that is stopped writes what it held. A
// run that streams has nothing to write, and a second call does nothing.
//
// A failed write is kept for Fault, not returned: the run's own end wins, as
// it does for a file that fails while the run goes on.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-02 22:32: writes once, under a lock, for the memory watch's abandon (C27)
//   - 2026-11-05 21:55: writes to standard output for a path of - (C80, Q191)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
//   - 2026-11-10 16:54: writes the held patches, not a graph (C119, Q304, B255)
func (o *Output) _Settle() {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	o._Settled()
}

// _Settled is _Settle with the lock held.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q304, B255)
func (o *Output) _Settled() {
	if o.spool == nil || o.settled {
		return
	}

	o.settled = true

	err := o.spool.Close()
	if err != nil {
		o.fault = errors.Join(
			o.fault,
			_Failed(
				GRAPH_FLAG,
				o.path,
				err,
			),
		)
	}
}
