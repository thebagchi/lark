// Package command is the host side of the lark command: the questions it was
// asked, the run it hosts, the output, the spool and the log it keeps, how it
// abandons a run and how it finishes. Package main keeps the entry point, the
// flags, the help and the unused-flag report.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package command

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// PLUGIN_DIR is what the directory holding this run's plugin socket is
	// called, and PLUGIN_SOCKET the socket in it. Short, because a unix socket
	// path has a low length limit.
	//
	// Revisions:
	//   - 2026-09-26 00:46: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	PLUGIN_DIR = "lark"

	// PLUGIN_SOCKET is the socket in the directory named PLUGIN_DIR.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	PLUGIN_SOCKET = "p"

	// TOKEN_BYTES is how much secret a run mints for its plugins.
	//
	// Revisions:
	//   - 2026-09-26 00:46: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	TOKEN_BYTES = 32

	// SCRIPT is what a failure calls the thing that failed, as FLOW and BUNDLE
	// are, so a reader knows which input was wrong.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	SCRIPT = "script"

	// BUNDLE is what a failure calls a bundle that failed.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	BUNDLE = "bundle"

	// STDIN_LOG is the name a transcript takes when the script came from standard input.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	STDIN_LOG = "stdin"

	// GRAPH_STDOUT is the -graph path that means standard output. A file of
	// that name is written as ./-.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	GRAPH_STDOUT = "-"

	// CAUSE_MARK opens the line that names the function a run failed in, and
	// ENTRY is the function every run starts in, which that line leaves out.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	CAUSE_MARK = "cause: "

	// ENTRY is the function every run starts in.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	ENTRY = "main"

	// GRAPH_FILE is the mode of the -graph file.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	//   - 2026-11-10 11:45: the one mode of the one file; CHANGES_FILE, TEMP_SUFFIX and
	//     TEMP_BYTES are gone with the temporary file and the changes file (C119, Q304)
	GRAPH_FILE = 0o640

	// HELD_PATTERN names the file the patches go to under -hold, until the run is
	// over; os.CreateTemp puts a random string in place of the star.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, Q304)
	HELD_PATTERN = "lark-*.patch"

	// LOG_DIRECTORY is the mode a transcript's directory is made with,
	// LOG_FILE the mode of the transcript itself, and LOG_SUFFIX what it is
	// called after the script it belongs to.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	LOG_DIRECTORY = 0o750

	// LOG_FILE is the mode of a transcript.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	LOG_FILE = 0o640

	// LOG_SUFFIX is what a transcript is called after the script it belongs to.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	LOG_SUFFIX = ".log"

	// SPOOL_PATTERN names the file held lines go to when no directory was
	// given; os.CreateTemp puts a random string in place of the star.
	//
	// Revisions:
	//   - 2026-11-03 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	SPOOL_PATTERN = "lark-*.log"

	// GENERATED is appended to a flow's path to name the script it makes.
	// No file of that name exists, which is the point: a position in a
	// failure belongs to the generated script, and convert is how to see it.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	//   - 2026-11-10 11:45: names the convert verb in place of -translate (C119, Q306)
	GENERATED = ".star"

	// FAILED is the exit code of a script or a graph that is wrong. That is
	// not the same event as a command that cannot do its job, and a caller
	// acts on them differently: the first means the input is wrong, the
	// second means the invocation is. They are told apart by exit code and
	// by the word the message opens with.
	//
	// Revisions:
	//   - 2026-09-19 23:41: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	FAILED = 1

	// NO_INPUT is the exit code of a command that cannot do its job: the
	// invocation is wrong, not the input.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation as a documented constant (C120b, Q315)
	NO_INPUT = 2

	// NO_FILE is the exit code of a file this command was given and could not
	// read, or was to make and could not.
	//
	// Revisions:
	//   - 2026-11-10 16:54: initial creation, moved from cmd/lark/main.go into
	//     internal/command (C119, B254)
	NO_FILE = 3

	// GROWTH is how many times the input buffer grows at each step of Read.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, B253)
	GROWTH = 2

	// FIRST is the size of the first buffer Read gives an input of unknown size.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation (C119, B253)
	FIRST = 512

	// BUNDLE_FILE is the mode a written bundle takes.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 16:54: moved from cmd/lark/main.go into internal/command (C119, B254)
	BUNDLE_FILE = 0o640

	// MEMORY is a run the memory watch ended: it tripped, and the run either
	// stopped within the grace or did not and was abandoned. One code for both
	// (decision Q44), its own, so it is not mistaken for NO_FILE (3) or for a
	// script that failed (1). The codes are 1 to 5: FAILED, NO_INPUT, NO_FILE,
	// STOPPED and MEMORY.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	MEMORY = 5

	// STOPPED is a run this command was asked to stop, by an interrupt or a
	// termination signal. Its own code, because a run somebody stopped did
	// not fail and a caller acts on the two differently.
	//
	// Read from whether the signal arrived, never from the error. A script
	// that cancels its own thread and joins it also meets runtime.ErrCancelled, and
	// that script failed in the ordinary way - samples/cancel.star is exactly
	// that, and exits 1.
	//
	// Revisions:
	//   - 2026-09-23 22:46: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	STOPPED = 4
)

var (
	// ErrArgs is returned when a verb gets the wrong number of arguments:
	// run, compile and convert take exactly one FILE, version takes none, and
	// help takes one verb at most. A hint after the text says which.
	//
	// Revisions:
	//   - 2026-11-05 18:10: names the script given with no flag (C68, Q174)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	//   - 2026-11-10 11:45: the number of arguments of a verb; the text is proposed
	//     (C119, Q305)
	//   - 2026-11-10 17:15: renamed from ErrOneInput (C121, Q349)
	ErrArgs = errors.New("lark: wrong number of arguments")

	// ErrVerb is returned when the command line has no words at all. A first
	// word that is no verb is a refusal of its own, with the word in it.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation; the text is proposed (C119, Q307)
	//   - 2026-11-10 17:15: renamed from ErrNoVerb (C121, Q349)
	ErrVerb = errors.New("lark: give a verb (run, compile, convert, version or help)")

	// ErrOutput is returned when compile has no -o (Q309).
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation; the text is proposed (C119, Q309)
	//   - 2026-11-10 17:15: renamed from ErrNoOutput (C121, Q349)
	ErrOutput = errors.New("lark: compile needs -o FILE")

	// ErrAs is returned for a -as that is not script, flow or bundle, and for
	// a -as beside a FILE that is not standard input: a file is a script, a
	// flow or a bundle by its extension (Q308). A hint after the text says which.
	//
	// Revisions:
	//   - 2026-11-10 11:45: initial creation; the text is proposed (C119, Q308)
	ErrAs = errors.New("lark: wrong -as")

	// ErrBundle is returned when a bundle is to be compiled or converted. It
	// carries no source to do either from, so it is only run.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	//   - 2026-11-10 11:45: names the verbs, not the flags; the text is proposed (C119, Q303)
	//   - 2026-11-10 17:15: renamed from ErrOnlyRun (C121, Q349)
	ErrBundle = errors.New(
		"lark: a bundle is only run (compile and convert need a script or a flow)",
	)
)

// Run is what one run of this command is made of: the options the compile
// and the run are given, the files it leaves, the spool its lines go through,
// the close of its plugins, and the run once it has started.
//
// The memory watch's abandon function is made before the run starts. It reads
// started to write the graph, then closes spool to write the lines.
//
// supplied is the run option that hands the script the -args arguments, or
// none. It is kept apart from options because Running gives Start one
// WithArgs: this one, or the one _Unused makes without the keys the script does
// not declare.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: holds the started run in a _Started, Execution being an interface (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 11:45: holds the -args option apart from options, so a run gives
//     WithArgs once (C75, Q256)
//   - 2026-11-08 12:50: says what supplied holds (C75, Q256)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Run to Run; renamed the fields compile
//     to Compile, supplied to Supplied, options to Options, output to Output, spool to Spool,
//     hosted to Hosted, started to Started, for the tests in tests/ (C120b, Q315, Q314)
type Run struct {
	Compile  []common.CompileOption
	Supplied []common.RunOption
	Options  []common.RunOption
	Output   *Output
	Spool    *Spool
	Hosted   func() error
	Started  atomic.Pointer[_Started]
}

// _Started holds a started run, so an atomic pointer can hold one: the
// Execution a run starts is an interface, and a pointer to an interface has
// no methods. It has the run's methods through the embedded Execution.
//
// Revisions:
//   - 2026-11-07 23:23: initial creation (C91w7, Q101)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
type _Started struct {
	common.Execution
}

// Close closes the spool, the plugins and the output files, in that order,
// and returns what they said together.
//
// One call, because main exits rather than returns.
//
// Revisions:
//   - 2026-11-03 23:57: initial creation, out of _HandedOf (C57)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Close to Close, for the tests in
//     tests/ (C120b, Q315, Q314)
func (run *Run) Close() error {
	return errors.Join(
		run.Spool.Close(),
		run.Hosted(),
		run.Output.Close(),
	)
}

// Report says one line about the command on standard error, in the form
// "lark: <err>". A failed write has nothing left to try, so it is dropped.
//
// Revisions:
//   - 2026-11-03 23:57: initial creation, out of nine copies of the line (C57)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Report to Report, for the tests in
//     tests/ (C120b, Q315, Q314)
func Report(err error) {
	_, _ = fmt.Fprintf(
		os.Stderr,
		"lark: %v\n",
		err,
	)
}

// Die reports err and exits with code. It is the end of every _Must function.
//
// Revisions:
//   - 2026-11-03 23:57: initial creation, out of seven copies of the report and the exit (C57)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Die to Die, for the tests in tests/
//     (C120b, Q315, Q314)
func Die(
	err error,
	code int,
) {
	Report(err)
	os.Exit(code)
}

// MustWatch is the run options for the memory watch, or none when it is off.
//
// When on, it first asks whether the system offers what the watch reads, and
// exits 2 with "lark:" and what it could not read when it does not, so the
// watch never silently does nothing. Then it hands the run the watch, with a
// function (Abandon) that settles the -graph file, prints one line and exits MEMORY
// for a run that will not end. No Go memory limit is set: the watch alone ends
// the run. The live graph is not turned on by the watch; a -graph file asks for
// the changes by its own option.
//
// Exits rather than returning an error, as the other _Must functions do.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: the abandon function writes the -graph file first (C27)
//   - 2026-11-02 22:32: no brake and no live graph; the abandon function takes a snapshot (C27)
//   - 2026-11-03 23:57: takes the _Run the abandon function reads, and exits through _Die (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _MustWatch to MustWatch, for the tests
//     in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: the abandon function takes no snapshot; the -graph file holds
//     patches (C119, Q304)
func MustWatch(
	asked *Asked,
	run *Run,
) []common.RunOption {
	if !asked.Memory {
		return nil
	}

	err := memwatch.Check()
	if err != nil {
		Die(
			err,
			NO_INPUT,
		)
	}

	return []common.RunOption{
		runtime.WithMemoryWatch(Abandon(
			run,
			os.Stderr,
			os.Exit,
		)),
	}
}

// Abandon is the function the watch calls for a run that did not end within
// its grace. It closes the output, which writes the patches a -hold run still
// holds to the -graph file, then closes the spool to write the held lines, then
// prints one line and exits MEMORY.
//
// A run that streams has nothing left to write: each patch went to the file as
// the run made it.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation, out of _MustWatch (C27)
//   - 2026-11-03 00:50: closes the spool after the graph, so the held lines are
//     written before the exit (C42)
//   - 2026-11-03 23:57: reads the output, the started run and the spool from one _Run (C57)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Abandon to Abandon, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: closes the output, which holds patches and no graph, so it takes no
//     snapshot of the run (C119, Q304)
func Abandon(
	run *Run,
	w io.Writer,
	exit func(int),
) func(error) {
	return Abandoning(
		w,
		exit,
		func() {
			// The patches first, then the lines, as at the end of any run. The
			// exit that follows skips the close in main, so a held transcript
			// would be lost, and the page behind a -logs file with it. A write
			// that fails has nothing left to be reported to.
			_ = run.Output.Close()
			_ = run.Spool.Close()
		},
	)
}

// Abandoning returns the function the watch calls for a run that did not end
// within its grace: it calls settle, which writes the -graph patches and the
// held lines, prints one line to w and calls exit with MEMORY. exit is a
// parameter so a test can see the code without the test process ending.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 22:32: settles the graph file before it exits (C27)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Abandoning to Abandoning, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 16:54: the settle function writes the -graph patches and the held lines
//     (C119, Q304)
func Abandoning(
	w io.Writer,
	exit func(int),
	settle func(),
) func(error) {
	return func(cause error) {
		settle()

		// A failed write to standard error leaves nothing else to try before
		// the exit, so the error is dropped on purpose.
		_, _ = fmt.Fprintf(
			w,
			"lark: %v\n",
			cause,
		)

		exit(MEMORY)
	}
}

// Asked is everything the command line said, after the pairs that cannot be
// asked for together have been refused.
//
// A struct rather than a dozen returns, per CLAUDE.md: the caller would
// otherwise take them in an order nothing checks. Parse fills it. Verb is the
// first word. File is the argument that names the input, as the user gave it,
// Path is what a failure calls it (<stdin> for standard input), and Noun is
// what kind of input it is: script, flow or bundle. Files are the arguments
// that are not flags, in order; Topic is the verb help is asked about.
//
// Revisions:
//   - 2026-09-26 23:55: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Asked to Asked; renamed the fields
//     script to Script, described to Described, bundled to Bundled, path to Path, noun to Noun,
//     translate to Translate, logs to Logs, bundle to Bundle, supplied to Supplied, plugins to
//     Plugins, graph to Graph, changes to Changes, hold to Hold, memory to Memory, version to
//     Version, stdin to Stdin, files to Files, for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: the verbs: adds Verb, Topic, File, As and Out; drops Script, Described,
//     Bundled, Translate, Bundle, Changes and Version, which the verbs and -o replace
//     (C119, Q304, Q305, Q309)
type Asked struct {
	Verb     string
	Topic    string
	Noun     string
	File     string
	Path     string
	As       string
	Out      string
	Logs     string
	Supplied string
	Plugins  string
	Graph    string
	Hold     bool
	Memory   bool
	Stdin    bool
	Files    []string
}

// Refusal is why a command line cannot be run, or nil. The first that applies
// is returned:
//  1. run, compile and convert do not get exactly one FILE, version gets an
//     argument, or help gets more than one (ErrArgs).
//  2. help is asked about a word that is no verb.
//  3. -as is not script, flow or bundle, or goes beside a FILE that is not
//     standard input (ErrAs).
//  4. compile has no -o (ErrOutput).
//  5. compile or convert is given a bundle (ErrBundle).
//
// Revisions:
//   - 2026-10-24 18:29: initial creation, as _OneInput
//   - 2026-11-02 21:04: refuses -graph and -changes beside -translate or -bundle
//   - 2026-11-03 00:50: refuses -hold there as well (C42)
//   - 2026-11-03 23:57: returns the refusal, and does not print or exit (C57)
//   - 2026-11-05 00:23: counts a script file given without -script, and refuses two (C81)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Refusal to Refusal, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: refuses by verb: flags belong to one verb, so the pairs of flags are
//     gone; adds -as and -o (C119, Q305, Q308, Q309)
//   - 2026-11-10 17:15: uses ErrOutput and ErrArgs and ErrBundle, formerly ErrNoOutput and
//     ErrOneInput and ErrOnlyRun (C121, Q349)
func Refusal(asked *Asked) error {
	switch asked.Verb {
	case VERSION:
		if len(asked.Files) > 0 {
			return fmt.Errorf(
				"%w (version takes none)",
				ErrArgs,
			)
		}

		return nil

	case HELP:
		return _Topic(asked)
	}

	if len(asked.Files) != 1 {
		return fmt.Errorf(
			"%w (give exactly one FILE)",
			ErrArgs,
		)
	}

	switch {
	case asked.As != "" && !asked.Stdin:
		return fmt.Errorf(
			"%w (it goes with FILE of -; a file's kind is its extension)",
			ErrAs,
		)

	case asked.As != "" && !slices.Contains(
		[]string{AS_SCRIPT, AS_FLOW, AS_BUNDLE},
		asked.As,
	):
		return fmt.Errorf(
			"%w (it is script, flow or bundle)",
			ErrAs,
		)

	case asked.Verb == COMPILE && asked.Out == "":
		return ErrOutput

	case asked.Verb != RUN && asked.Noun == BUNDLE:
		return ErrBundle

	default:
		return nil
	}
}

// _Topic is the refusal of a help that is asked about more than one word, or
// about a word that is no verb.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
//   - 2026-11-10 17:15: uses ErrArgs, formerly ErrOneInput (C121, Q349)
func _Topic(asked *Asked) error {
	switch {
	case len(asked.Files) > 1:
		return fmt.Errorf(
			"%w (help takes one verb at most)",
			ErrArgs,
		)

	case asked.Topic != "" && !slices.Contains(
		_Verbs(),
		asked.Topic,
	):
		return _Unknown(asked.Topic)

	default:
		return nil
	}
}

// Finish closes what the command opened and exits when the work failed.
//
// A transcript that could not be finished is reported, unless the run
// already had something worse to say. Nothing deferred in main runs, so
// this is the close.
//
// A file the command could not make, the bundle of compile, is the command's
// own failure and not the input's: one "lark:" line and exit NO_FILE.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-24 21:41: takes the context first
//   - 2026-11-02 22:32: a run the memory watch ended exits MEMORY with its one line (C27)
//   - 2026-11-03 23:57: says the one line and exits MEMORY itself, with no empty function (C57)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Finish to Finish, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: a bundle that could not be written exits NO_FILE with a lark: line
//     (C119, B254)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func Finish(
	stopping context.Context,
	closing func() error,
	err error,
	noun string,
) {
	ending := closing()
	if err == nil {
		err = ending
	}

	if err == nil {
		return
	}

	if stopping.Err() != nil {
		fmt.Fprintf(
			os.Stderr,
			"%s stopped\n",
			noun,
		)
		os.Exit(STOPPED)
	}

	if errors.Is(
		err,
		memwatch.ErrLimit,
	) {
		// The graph file was written when Wait returned.
		Die(
			memwatch.ErrLimit,
			MEMORY,
		)
	}

	var unwritten *_UnwrittenError
	if errors.As(
		err,
		&unwritten,
	) {
		Die(
			unwritten.err,
			NO_FILE,
		)
	}

	fmt.Fprintf(
		os.Stderr,
		"%s failed: %v\n",
		noun,
		err,
	)
	os.Exit(FAILED)
}

// Caused is err, with the line "cause: F" under its text, or err itself when
// the line adds nothing (decision Q190).
//
// F is the function the graph names as the cause. The line is left out when
// the run has no cause, when F is main (the position in the text is main's
// already), and when the text of err names F as a segment of its own: the
// runtime writes "F: file:line: ..." for a thread that failed, and a position
// then follows it. Everything is read from the run's graph, which exists once
// Wait has returned. The result unwraps to err, so errors.Is still sees it.
//
// Revisions:
//   - 2026-11-05 21:58: initial creation (C80)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Caused to Caused, for the tests in
//     tests/ (C120b, Q315, Q314)
func (run *Run) Caused(err error) error {
	started := run.Started.Load()
	if started == nil || err == nil {
		return err
	}

	function := started.Status().GetCause().GetFunction()
	if function == "" || function == ENTRY {
		return err
	}

	text := err.Error()
	if strings.HasPrefix(
		text,
		function+": ",
	) || strings.Contains(
		text,
		": "+function+": ",
	) {
		return err
	}

	// A name with a control byte or a quote is written quoted, so a name a
	// script chose cannot start a line of its own (Q4).
	shown := unpack.QuotePrefix(function)
	if shown == `"`+function+`"` {
		shown = function
	}

	return &_CausedError{
		err:  err,
		line: CAUSE_MARK + shown,
	}
}

// _CausedError is a failure with the line that names its cause under its text.
//
// Revisions:
//   - 2026-11-05 21:58: initial creation (C80)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
type _CausedError struct {
	err  error
	line string
}

// Error is the text of the failure, a line break and the cause line.
//
// Revisions:
//   - 2026-11-05 21:58: initial creation (C80)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func (c *_CausedError) Error() string {
	return c.err.Error() + "\n" + c.line
}

// Unwrap is the failure without the line.
//
// Revisions:
//   - 2026-11-05 21:58: initial creation (C80)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func (c *_CausedError) Unwrap() error {
	return c.err
}

// Logging is the run option sending what this run prints to a text logger,
// with the spool its lines go through, or nil when they go nowhere but out.
//
// By default the lines stream: standard output always, because that is where a
// person running a script looks, and the file as well when a directory was
// given, named for the input with .log on the end, so a script run twice
// overwrites its own log. The file is written through a page-sized buffer
// (decision Q7); the spool's close flushes it. WithLog writes each line, with
// its time, thread and function, and the message last.
//
// The lines are the same on a terminal, a pipe and a file: the time, the
// level, the thread, the function and the message (Q342). The command has no
// plain form, so the file and the screen agree.
//
// With hold, the lines go to the spool alone, and its close writes them to out
// once the run is over (decision Q1, made opt-in by Q43). The spool is the
// -logs file, opened for reading as well; with no directory it is a file in
// os.TempDir() that the close removes, and its name goes to notice at once, so
// the lines can be found should the process be killed.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, as _Reporting
//   - 2026-10-02 13:12: the runtime's transcript, to standard output and the
//     file, rather than a reporter of this command's own
//   - 2026-10-02 16:21: a logger handed to the run, whose handler writes the
//     time and the layout, rather than a transcript on the context
//   - 2026-10-02 16:33: writes each line's message last
//   - 2026-10-03 16:31: through the runtime's WithLog, which writes as this did
//   - 2026-11-03 00:50: the file behind a page-sized buffer, and -hold (C42)
//   - 2026-11-03 01:20: takes a _Where, having five things to be told (C42)
//   - 2026-11-03 21:45: a closed standard output stops the run, and the file
//     is written before standard output, so no line is lost to it (C42)
//   - 2026-11-03 23:57: reads the directory, the hold and the input path from asked, and
//     prints to standard output and standard error, being told nothing else (C57)
//   - 2026-11-04 19:35: the message alone on a terminal, with no -logs or -hold; takes the
//     terminal check (C72, Q81)
//   - 2026-11-05 16:44: the -hold line says the lines are held until the run
//     ends, which no longer reads as a fault (C82)
//   - 2026-11-05 21:58: with -graph -, the lines go to standard error, which is then the
//     output the terminal check reads (C80, Q191)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Logging to Logging, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: always full lines, on a terminal too, so it takes no terminal check (C121,
//     Q342)
func Logging(
	asked *Asked,
	broken func(),
) (common.RunOption, *Spool, error) {
	printed := os.Stdout
	if asked.Graph == GRAPH_STDOUT {
		printed = os.Stderr
	}

	if asked.Logs == "" && !asked.Hold {
		return runtime.WithLog(printed), nil, nil
	}

	file, kept, err := _Spooled(asked)
	if err != nil {
		return nil, nil, err
	}

	var (
		out = &_Pipe{
			w:      printed,
			broken: broken,
		}
		spool = NewSpool(
			file,
			asked.Hold,
			asked.Logs == "",
			out,
		)
	)

	if asked.Logs == "" {
		_, _ = fmt.Fprintf(
			os.Stderr,
			"lark: lines are held until the run ends: %s\n",
			kept,
		)
	}

	if asked.Hold {
		return runtime.WithLog(spool), spool, nil
	}

	// The spool first, so that a line standard output refuses is in the file.
	return runtime.WithLog(io.MultiWriter(
		spool,
		out,
	)), spool, nil
}

// _Spooled is the file the lines go to: the transcript in dir, or a temporary
// file when dir is empty. It is opened for reading as well when the lines are
// held, being read back at the end. The second result is its name.
//
// Revisions:
//   - 2026-11-03 00:50: initial creation, out of _Logging (C42)
//   - 2026-11-03 23:57: reads the directory, the hold and the input path from asked (C57)
//   - 2026-11-05 00:23: names the transcript of a script from standard input stdin.log (C81, Q153)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Spooled(asked *Asked) (*os.File, string, error) {
	dir := asked.Logs

	if dir == "" {
		file, err := os.CreateTemp(
			os.TempDir(),
			SPOOL_PATTERN,
		)
		if err != nil {
			return nil, "", fmt.Errorf(
				"log %s: %w",
				os.TempDir(),
				err,
			)
		}

		return file, file.Name(), nil
	}

	err := os.MkdirAll(
		dir,
		LOG_DIRECTORY,
	)
	if err != nil {
		return nil, "", fmt.Errorf(
			"log %s: %w",
			dir,
			err,
		)
	}

	base := filepath.Base(asked.Path)
	if asked.Stdin {
		base = STDIN_LOG
	}

	var (
		name = filepath.Join(
			dir,
			base+LOG_SUFFIX,
		)
		mode = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	)

	if asked.Hold {
		mode = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	}

	file, err := os.OpenFile(
		name,
		mode,
		LOG_FILE,
	)
	if err != nil {
		return nil, "", fmt.Errorf(
			"log %s: %w",
			name,
			err,
		)
	}

	return file, name, nil
}

// Built is the artifact what was read from the file at asked.path describes:
// a script, compiled; a bundle compile wrote, loaded; or a flow, its script
// generated and compiled. Each takes the plugins a compile here is given -
// file, and whatever -plugins hosts - since compiled code names what it calls, and a
// bundle compile wrote was compiled with them.
//
// A script is compiled without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path. The compile merges the modules
// into the script.
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from the three ways a run or a bundle was
//     built, each beside what it was built for
//   - 2026-11-01 12:30: flag names in the comment are the full names
//   - 2026-11-02 22:58: the merge is the library's, so this holds none (C44a)
//   - 2026-11-07 23:04: returns common.Artifact, an interface (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Built to Built, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: names the compile verb in place of -bundle (C119, Q303)
func Built(
	asked *Asked,
	src []byte,
	opts []common.CompileOption,
) (common.Artifact, error) {
	switch asked.Noun {
	case SCRIPT:
		return runtime.Compile(
			&common.Source{
				Entry: asked.Path,
				Text:  src,
			},
			opts...,
		)

	case BUNDLE:
		built, err := runtime.Load(
			src,
			opts...,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: %w",
				asked.Path,
				err,
			)
		}

		return built, nil

	default:
		// A flow, the one noun left.
		return _Compiled(
			asked.Path,
			src,
			opts...,
		)
	}
}

// Running builds what was read and calls its entry point, with what it prints
// going where the run's options say.
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from _Run, _RunBundle and _Play, which
//     differed only in how the artifact was built
//   - 2026-11-02 21:04: settles the graph file once the run is over
//   - 2026-11-02 22:32: tells the abandon function which run this is (C27)
//   - 2026-11-03 23:57: takes the _Run, which holds the started run (C57)
//   - 2026-11-05 19:20: warns of each -args key the script does not declare
//     and drops it from the run (C71)
//   - 2026-11-07 17:40: returns the error Start returns (C91 wave 0)
//   - 2026-11-07 23:23: stores the run in a _Started (C91w7)
//   - 2026-11-08 11:45: hands Start one WithArgs, the filtered one or the read one (C75, Q256)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Running to Running, for the tests in
//     tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: settles the held patches, not a graph (C119, Q304)
func Running(
	ctx context.Context,
	asked *Asked,
	src []byte,
	run *Run,
) error {
	built, err := Built(
		asked,
		src,
		run.Compile,
	)
	if err != nil {
		return err
	}

	// One WithArgs: the filtered arguments when -args named a key the script
	// does not declare, else the ones read from -args.
	args := _Unused(
		asked,
		src,
		run.Compile,
	)
	if args == nil {
		args = run.Supplied
	}

	started, err := runtime.Start(
		ctx,
		built,
		slices.Concat(
			args,
			run.Options,
		)...,
	)
	if err != nil {
		return err
	}

	run.Started.Store(&_Started{
		Execution: started,
	})

	_, err = started.Wait()

	// Whatever the end was, the patches a -hold run held are written now.
	run.Output._Settle()

	return err
}

// _Compiled is the artifact the flow at path describes.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
//   - 2026-10-02 01:45: reads a flow, and carries nothing beside the program
//   - 2026-11-07 23:04: returns common.Artifact, an interface (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Compiled(
	path string,
	src []byte,
	opts ...common.CompileOption,
) (common.Artifact, error) {
	out, err := Generated(
		path,
		src,
	)
	if err != nil {
		return nil, err
	}

	return runtime.Compile(
		&common.Source{
			Entry: path + GENERATED,
			Text:  out,
		},
		opts...,
	)
}

// Generated is the Starlark the flow at path generates: read by runtime.Read, which
// decodes it and checks it, then written by Emit, so a flow nobody validated is
// refused with what is wrong with it rather than becoming a script that fails
// somewhere else.
//
// A flow written before its values were tagged holds plain JSON, "west" where
// {"string": "west"} stands now, and no longer reads (Q422). Its refusal says
// what a value is now. A JSON null where a value stands is refused too (Q439). The
// words of a refusal that quote the file are cut to 100 bytes (Q4). The error wraps
// the error of Read, so errors.Is names what Read's error names. The words of every
// refusal begin with path, a flow that fails Check included (Q448).
//
// Revisions:
//   - 2026-09-21 16:25: initial creation, as _Generated
//   - 2026-09-21 17:19: returns the graph as well, as _Described
//   - 2026-10-02 01:45: reads a flow, and returns only the script, since a
//     bundle no longer carries what it came from
//   - 2026-10-03 16:29: leaves the check to Emit, which now makes it
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Generated to Generated, for the tests
//     in tests/ (C120b, Q315, Q314)
//   - 2026-11-13 10:15: bounds the decoder's words and says what a value is, for a flow of plain
//     values; refuses a null where a value stands; the error matches runtime.ErrForm and the
//     decoder's error (C132, Q4, Q422, Q439)
//   - 2026-11-13 17:39: reads the flow with runtime.Read, which holds the decoding, the null
//     refusal and the hints; the words of every refusal begin with path (C133, Q441, Q444,
//     Q448)
func Generated(
	path string,
	src []byte,
) ([]byte, error) {
	described, err := runtime.Read(src)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			path,
			err,
		)
	}

	return runtime.Emit(described)
}

// Hosting starts the plugins in dir and answers with the ones that attached,
// and with how to see them off.
//
// Empty dir is no plugins, which is not a failure: every plugin here is
// optional, and a host that wants none says nothing.
//
// The token is minted here and never leaves this process except in a child's
// environment. It is the only thing between a local process and putting names
// into every script this run compiles, so it is not an argument - an argument
// is in the process table, readable by anyone on the machine.
//
// A plugin that will not start is reported on standard error and the run goes
// on without it. Returns an error when the socket could not be set up or dir
// could not be searched, having first undone whatever it had made.
//
// Revisions:
//   - 2026-09-26 00:34: initial creation
//   - 2026-10-02 13:12: hands the compiler the listener's plugins, there being
//     no registry to hand it
//   - 2026-10-02 15:56: hands over only the plugins that attached, since a
//     compiler adds them to the runtime's own
//   - 2026-10-02 17:12: hands the listener no registry, since it keeps what
//     attaches itself
//   - 2026-10-02 17:47: answers with the plugins rather than an option, for the
//     one WithPlugins a compile is handed
//   - 2026-10-03 20:29: removes the socket's directory, and closes the listener, when
//     it fails after making them, rather than leaving both to a caller that
//     exits on the error
//   - 2026-11-03 23:57: reports a skipped plugin through _Report (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Hosting to Hosting, for the tests in
//     tests/ (C120b, Q315, Q314)
func Hosting(dir string) ([]common.Plugin, func() error, error) {
	nothing := func() error {
		return nil
	}

	if dir == "" {
		return nil, nothing, nil
	}

	token, err := _Token()
	if err != nil {
		return nil, nothing, err
	}

	held, listener, err := _Socket(token)
	if err != nil {
		return nil, nothing, err
	}

	var closing = func() error {
		err := listener.Close()

		return errors.Join(
			err,
			os.RemoveAll(held),
		)
	}

	loading, err := listener.Load(
		dir,
		"",
	)
	if err != nil {
		return nil, nothing, errors.Join(
			err,
			closing(),
		)
	}

	for _, skipped := range loading.Skipped {
		Report(skipped)
	}

	// Only what attached: a compiler adds these to the runtime's own.
	return listener.Plugins(), closing, nil
}

// _Socket makes a short directory and listens there for plugins.
//
// The directory is its own, so the socket path stays under the length a
// unix socket allows. A failed listen removes the directory.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-07 19:54: holds the listener as common.Listener (C91w8c)
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Socket(token string) (string, common.Listener, error) {
	held, err := os.MkdirTemp(
		"",
		PLUGIN_DIR,
	)
	if err != nil {
		return "", nil, fmt.Errorf(
			"making somewhere for the plugin socket: %w",
			err,
		)
	}

	listener, err := remote.Listen(
		filepath.Join(
			held,
			PLUGIN_SOCKET,
		),
		token,
	)
	if err != nil {
		return "", nil, errors.Join(
			err,
			os.RemoveAll(held),
		)
	}

	return held, listener, nil
}

// _Token is a secret this run shares with the plugins it starts.
//
// Revisions:
//   - 2026-09-26 00:34: initial creation
//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
func _Token() (string, error) {
	var (
		held = make(
			[]byte,
			TOKEN_BYTES,
		)
		err error
	)

	_, err = rand.Read(held)
	if err != nil {
		return "", fmt.Errorf(
			"minting a plugin token: %w",
			err,
		)
	}

	return hex.EncodeToString(held), nil
}

// Read is the text of the input asked names: standard input from stdin for a
// FILE of -, else the file at its path. It refuses an input that does not fit
// the free memory, with an error that wraps memwatch.ErrRoom (Q23, B253).
//
// room is the check, as memwatch.Room is: who, then the size in bytes. A
// regular file is checked once, by the size the file system gives, before
// anything is read. An input of unknown size, standard input or a device such
// as /dev/zero, is checked each time its buffer must grow, for the old buffer
// and the new one, which exist together while the old one is copied. A short
// input costs checks below common.ROOM_SKIP, which read no memory.
//
// An open or read that fails returns the error of the os package, which names
// the path.
//
// Revisions:
//   - 2026-11-05 00:23: initial creation (C81)
//   - 2026-11-10 11:32: reads Asked from command, which moved to internal/command (C120b, Q315)
//   - 2026-11-10 16:54: renamed from _Read, moved from cmd/lark/main.go into
//     internal/command, and bounded by the free memory (C119, B253, Q23)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Read(
	asked *Asked,
	stdin io.Reader,
	room func(
		string,
		uint64,
	) error,
) ([]byte, error) {
	who := asked.Noun + " " + unpack.QuotePrefix(asked.Path)

	if asked.Stdin {
		return _Streamed(
			who,
			stdin,
			FIRST,
			room,
		)
	}

	file, err := os.Open(asked.File)
	if err != nil {
		return nil, err
	}

	var (
		first         = FIRST
		info, statErr = file.Stat()
	)

	if statErr == nil && info.Mode().IsRegular() {
		err = room(
			who,
			uint64(info.Size()),
		)
		if err != nil {
			return nil, errors.Join(
				err,
				file.Close(),
			)
		}

		first = int(info.Size()) + 1
	}

	text, err := _Streamed(
		who,
		file,
		first,
		room,
	)

	return text, errors.Join(
		err,
		file.Close(),
	)
}

// _Streamed reads in to its end into a buffer that starts at first bytes and
// grows by GROWTH. Before each growth it asks room for the old buffer and the
// new one, which exist together while the old one is copied into the new one.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B253)
func _Streamed(
	who string,
	in io.Reader,
	first int,
	room func(
		string,
		uint64,
	) error,
) ([]byte, error) {
	held := make(
		[]byte,
		0,
		first,
	)

	for {
		if len(held) == cap(held) {
			err := room(
				who,
				uint64(cap(held))*(1+GROWTH),
			)
			if err != nil {
				return nil, err
			}

			held = slices.Grow(
				held,
				cap(held),
			)
		}

		count, err := in.Read(held[len(held):cap(held)])
		held = held[:len(held)+count]

		if errors.Is(
			err,
			io.EOF,
		) {
			return held, nil
		}

		if err != nil {
			return nil, err
		}
	}
}

// _UnwrittenError is a file this command was to make and could not. Finish ends a
// run that failed with it by the exit NO_FILE and one "lark:" line, as it ends
// a graph file it could not write.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B254)
type _UnwrittenError struct {
	err error
}

// Error is the text of the failure.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B254)
func (u *_UnwrittenError) Error() string {
	return u.err.Error()
}

// Unwrap is the failure the file system gave.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B254)
func (u *_UnwrittenError) Unwrap() error {
	return u.err
}

// Keeping builds what was read and writes it as a bundle to asked.Out.
//
// A bundle that cannot be written is not a failed script: the error is an
// _UnwrittenError, so that Finish exits NO_FILE with a "lark:" line (B254).
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from _Bundle and _BundleOf, which differed
//     only in how the artifact was built
//   - 2026-11-03 23:57: takes the _Run (C57)
//   - 2026-11-05 16:44: says "lark: wrote <file>" on standard error when the
//     bundle is written (C82, Q166)
//   - 2026-11-10 11:32: reads Asked and Run from command, calls Built of command, which moved
//     to internal/command (C120b, Q315)
//   - 2026-11-10 16:54: renamed from _Keeping, moved from cmd/lark/main.go into
//     internal/command with the write of _Kept; a failed write exits 3 (C119, B254, Q305)
func Keeping(
	asked *Asked,
	src []byte,
	run *Run,
) error {
	built, err := Built(
		asked,
		src,
		run.Compile,
	)
	if err != nil {
		return err
	}

	bundle, err := built.Save()
	if err != nil {
		return err
	}

	err = os.WriteFile(
		asked.Out,
		bundle,
		BUNDLE_FILE,
	)
	if err != nil {
		return &_UnwrittenError{
			err: _Failed(
				BUNDLE,
				asked.Out,
				err,
			),
		}
	}

	// The bundle is the command's own output file, so the line goes to
	// standard error with the other lines the command says about itself.
	_, _ = fmt.Fprintf(
		os.Stderr,
		"lark: wrote %s\n",
		unpack.QuotePrefix(asked.Out),
	)

	return nil
}
