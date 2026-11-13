// Command lark runs a Starlark script or a workflow's flow, compiles either
// into a bundle, and converts either into the other.
//
// The first word is the verb. A verb is required; each flag belongs to one verb
// (Q305), and lark help prints the list of them.
//
//	lark run samples/concurrent.star                   run the script
//	lark run - < build.star                            run the script on standard input
//	lark run -as flow - < run.json                     run the flow on standard input
//	lark run run.json                                  run the flow
//	lark run build.bin                                 run a bundle
//	lark convert samples/concurrent.star               print the flow it describes, as JSON
//	lark convert run.json                              print the Starlark it generates
//	lark run -logs logs build.star                     run it, and keep a transcript
//	lark compile -o build.bin build.star               compile it into a bundle
//	lark run -args '{"n": 3}' build.star               run it, supplying its arguments
//	lark run -plugins plugins build.star               run it, with the plugins in plugins
//	lark run -graph changes.jsonl build.star           run it, and keep its changes
//	lark run -memory build.star                        run it, ending it at 95 percent memory
//	lark run -hold build.star                          run it, printing its lines at the end
//	lark version                                       print "lark <version> <go version>"
//	lark help run                                      print the help of run
//
// The kind of FILE is by its extension (Q308): .bin is a bundle, .json a flow,
// and anything else a script. A FILE of - reads standard input as a script, a
// position in it is called <stdin>, and its -logs file is stdin.log; -as flow
// or -as bundle reads it as that kind. -as goes with FILE of - only.
//
// -hold is off by default, and the lines stream as they are printed. With it
// they go to a file instead - the -logs file, else a temporary file whose name
// is the first line on standard error - and are printed when the run is over,
// after the -graph file is written. The temporary file is removed then. A run
// ended by -memory, by a signal or by a failure prints what it held.
//
// -memory is off by default. It reads /proc/meminfo and the cgroup v2 of
// this process, takes the tighter, and at 95 percent ends the run with "memory
// limit reached". A run that does not end within 500 ms of that, because a Go
// builtin is busy allocating, is abandoned. Either way the command prints one
// line on standard error and exits 5. A system without /proc/meminfo or cgroup
// v2 refuses the flag with exit 2.
//
// version prints one line, "lark <version> <go version>", and exits 0. The
// version is the main module's, from runtime/debug.ReadBuildInfo: the tag for
// go install github.com/thebagchi/lark/cmd/lark@v1.2.3, a pseudo-version for a
// go build inside a git checkout, and (devel) otherwise.
//
// Every flag has its full name only. Go's flag package takes -name and --name
// alike and exits 2 on a name it does not know.
//
// A signal stops a run: SIGINT, SIGTERM and SIGHUP alike. The script is told
// between instructions, the transcript is closed, a held spool is removed, and
// this exits 4. A SIGHUP that the process was started with ignored, as nohup
// does, stays ignored.
//
// -graph writes each change the run makes to its graph, one JSON Patch a line,
// the moment the run makes it; patching the lines in order onto {} gives the
// graph the run ended with, and the status changes are lines of it. A path of -
// writes them to standard output instead, and what the script prints, -hold lines
// included, goes to standard error, so standard output holds the lines alone.
// Under -hold the lines are kept until the run is over and written then. A
// failed run whose failing function is not main, and is not already named in the
// failure text, ends with a line "cause: <function>" on standard error. A path in
// a directory that is not there is refused before the script runs, with exit 3.
//
// A bundle that compile cannot write, like any file the command is given and
// cannot read or make, exits 3 with one "lark:" line.
//
// It is the smallest host this runtime supports. Given a script it compiles
// that script together with every module it loads and calls its entry point;
// modules resolve beside the file that loaded them, so a script in samples/
// loading "strings.star" reads samples/strings.star. Given a flow it checks
// it, generates a script and runs that - a flow names no modules, because
// deriving one inlines what its script loaded.
//
// The two conversions are inverses, so a flow printed by the convert of a script
// generates the script the convert of the flow prints. Running either form runs
// the same workflow, which is what the round trip in runtime/graph measures.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 11:45: the verbs run, compile, convert, version and help in place of the
//     flags; -graph is the patches and -changes is gone; SIGHUP stops a run (C119, Q299,
//     Q303, Q304, Q305, Q306, Q307, Q308, Q309)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

const (
	// INDENT is how a printed flow is laid out, one level per nesting.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	INDENT = "  "
)

// _Prepared is the run a command line asks for, with everything it needs open
// and a close for all of it.
//
// One function because these are one job - everything a run is told before it
// starts - and they read in the order they are layered: where the output files
// go, what arguments were supplied, where a transcript goes, and which plugins
// are hosted.
//
// Every compile is handed file. A script this command runs is the user's own,
// run with the user's own permissions, so it gets the disk the user has; file
// is a plugin a host hands over, and this command is that host.
//
// Exits rather than returning an error, as _Flags does, because there is no
// caller but main.
//
// Revisions:
//   - 2026-09-26 03:24: initial creation, lifted out of main
//   - 2026-10-02 16:08: hands the run the arguments, which no longer travel on
//     the context
//   - 2026-10-02 16:18: reads the arguments into a Struct, refusing anything
//     that is not a JSON object before the run
//   - 2026-10-02 16:21: hands the run its logger, which no longer travels on
//     the context
//   - 2026-10-02 17:12: hands every compile file, which the runtime no longer
//     gives a script by itself
//   - 2026-10-02 17:47: hands file and the hosted plugins in one WithPlugins,
//     a second replacing the first
//   - 2026-10-03 08:32: hands the run its memory ceiling as an option, and so
//     returns no context, having none to derive
//   - 2026-11-01 12:00: hands the run no memory ceiling, the -m flag being gone
//   - 2026-11-02 21:04: opens the graph and changes files before the run
//   - 2026-11-02 22:32: hands the run the memory watch under -memory (C27)
//   - 2026-11-03 00:50: holds the transcript's spool beside the run, so the
//     abandon function can write it (C42)
//   - 2026-11-03 21:45: makes the spool after the checks that can exit, and
//     takes the function that stops the run when standard output closes (C42)
//   - 2026-11-03 23:57: builds the one _Run itself, which the watch reads, in place of
//     the three carriers it was joined from, and reads the input path from
//     asked (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 12:50: keeps the -args option in run.supplied, apart from run.options (C75, Q256)
//   - 2026-11-10 11:32: reads Asked and Run from command, calls MustWatch of command, which
//     moved to internal/command (C120b, Q315)
//   - 2026-11-10 11:45: the -graph file is the one output file (C119, Q304)
func _Prepared(
	asked *command.Asked,
	broken func(),
) *command.Run {
	// The spool is made after everything that can exit before the run, except
	// the plugins: a temporary file made earlier would be left behind by those
	// exits. The plugins come last, and their failure closes the spool.
	var (
		run = &command.Run{
			Output: _MustOpen(asked),
		}
		supplied = _MustSupply(asked.Supplied)
		watching = command.MustWatch(
			asked,
			run,
		)
		logging, spool = _MustLog(
			asked,
			broken,
		)
		plugins, hosted = _MustHost(
			asked.Plugins,
			spool,
			run.Output,
		)
	)

	run.Spool = spool
	run.Hosted = hosted
	run.Compile = []common.CompileOption{
		runtime.WithPlugins(append(
			[]common.Plugin{larkfile.New()},
			plugins...,
		)...),
	}
	run.Supplied = supplied
	run.Options = slices.Concat(
		watching,
		[]common.RunOption{logging},
	)

	if watch := run.Output.Watch(); watch != nil {
		run.Options = append(
			run.Options,
			runtime.WithChanges(watch),
		)
	}

	return run
}

// _Usage says err on standard error as it is, then TRY, and exits NO_INPUT:
// the invocation is wrong, and the person is told where the help is, not shown
// the whole flag list again (C68).
//
// Revisions:
//   - 2026-11-03 23:57: initial creation, out of five copies (C57)
//   - 2026-11-05 18:10: prints TRY, not the flag list (C68, Q172)
func _Usage(err error) {
	_, _ = fmt.Fprintln(
		os.Stderr,
		err,
	)

	_Try()
}

// _Try prints TRY on standard error and exits NO_INPUT.
//
// Revisions:
//   - 2026-11-05 18:10: initial creation (C68)
//   - 2026-11-10 11:32: calls NO_INPUT of command, which moved to internal/command (C120b,
//     Q315)
//   - 2026-11-10 16:54: the flag package no longer writes its own error line (C119, Q305)
func _Try() {
	_, _ = fmt.Fprintln(
		os.Stderr,
		TRY,
	)

	os.Exit(command.NO_INPUT)
}

// _MustLog opens the transcript, or exits when it cannot.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-03 00:50: returns the spool, and takes -hold (C42)
//   - 2026-11-03 21:45: takes the function a closed standard output calls (C42)
//   - 2026-11-03 23:57: reads the input path from asked, and exits through _Die (C57)
//   - 2026-11-04 19:35: gives _Logging the terminal check (C72)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 11:32: reads Asked from command, calls Spool, Logging, CharDevice and Die of
//     command, which moved to internal/command (C120b, Q315)
//   - 2026-11-10 11:45: calls NO_FILE of command, which moved there (C119, B254)
//   - 2026-11-10 17:15: gives Logging no terminal check, which is dropped (C121, Q342)
func _MustLog(
	asked *command.Asked,
	broken func(),
) (common.RunOption, *command.Spool) {
	logging, spool, err := command.Logging(
		asked,
		broken,
	)
	if err != nil {
		command.Die(
			err,
			command.NO_FILE,
		)
	}

	if spool != nil {
		// A write to a closed standard output then fails with EPIPE, which
		// Logging turns into a stop, rather than ending the process by
		// SIGPIPE before the spool is closed (os/signal, "SIGPIPE"). Nothing
		// reads the channel: the signal is only to be caught.
		signal.Notify(
			make(
				chan os.Signal,
				1,
			),
			syscall.SIGPIPE,
		)
	}

	return logging, spool
}

// _MustOpen opens the -graph file, or exits when it cannot.
//
// Revisions:
//   - 2026-11-02 21:04: initial creation
//   - 2026-11-03 23:57: exits through _Die (C57)
//   - 2026-11-10 11:32: reads Asked from command, calls Output, Opened and Die of command,
//     which moved to internal/command (C120b, Q315)
//   - 2026-11-10 11:45: calls NO_FILE of command, which moved there (C119, B254)
func _MustOpen(asked *command.Asked) *command.Output {
	output, err := command.Opened(asked)
	if err != nil {
		command.Die(
			err,
			command.NO_FILE,
		)
	}

	return output
}

// _MustSupply reads the arguments file, or exits when it cannot.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-03 23:57: exits through _Usage (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
func _MustSupply(supplied string) []common.RunOption {
	run, err := _Supplying(supplied)
	if err != nil {
		_Usage(fmt.Errorf(
			"lark: %w",
			err,
		))
	}

	return run
}

// _MustHost starts the plugins, or closes spool and exits when it cannot.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-03 21:45: closes the spool before the exit, so a temporary file
//     is not left (C42)
//   - 2026-11-03 23:57: exits through _Die (C57)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 11:32: calls Spool, Hosting and Die of command, which moved to
//     internal/command (C120b, Q315)
//   - 2026-11-10 11:45: closes the output too, so a held patch spool is not left, and calls
//     NO_FILE of command (C119, B254)
func _MustHost(
	dir string,
	spool *command.Spool,
	output *command.Output,
) ([]common.Plugin, func() error) {
	plugins, hosted, err := command.Hosting(dir)
	if err != nil {
		_ = spool.Close()
		_ = output.Close()

		command.Die(
			err,
			command.NO_FILE,
		)
	}

	return plugins, hosted
}

// _Flags is the command line, read and checked.
//
// Exits rather than returning an error, because there is no caller but main and
// a usage message is what a person wants here rather than a wrapped cause. The
// verbs help and version are answered here, before any input is read: they need
// none.
//
// Revisions:
//   - 2026-09-26 03:20: initial creation, lifted out of main
//   - 2026-10-03 00:28: reads a bundle to run under -r, which takes neither -t nor -b
//   - 2026-11-03 23:57: names the input the check allowed, with no copy of the flags (C57)
//   - 2026-11-04 22:52: prints the version and exits 0 under -version, before any check (C67)
//   - 2026-11-05 00:23: takes a script without -script, and - for standard input (C81)
//   - 2026-11-05 18:10: reads the flags through _Parse, so -h prints the help (C68)
//   - 2026-11-10 11:32: reads Asked from command, calls Refusal, BUNDLE and SCRIPT of command,
//     which moved to internal/command (C120b, Q315)
//   - 2026-11-10 11:45: the command line is read by Parse of command, one verb at a time; help
//     and version are verbs (C119, Q305, Q307)
func _Flags() *command.Asked {
	asked, err := command.Parse(os.Args[1:])
	if err != nil {
		_Usage(err)
	}

	switch asked.Verb {
	case command.HELP:
		_PrintHelp(asked.Topic)

	case command.VERSION:
		_PrintVersion()
	}

	return asked
}

// main runs, compiles or converts the script or flow the command line names, or
// runs the bundle it names, and exits with what happened to it.
//
// Exits 0 on success, 1 when the input was wrong, 2 when the command line was, 3
// when a file it was given could not be read or made, 4 when the run was stopped
// and 5 when the memory watch ended it (-memory). A standard output that is
// closed by its reader counts as a stop, when -logs or -hold is given: the -logs
// file is written whole, a held spool is removed, and the exit is 4 (decision
// Q72). Without either flag the process ends by SIGPIPE, as before.
//
// The command line, the file and the memory are this command's problem and open
// with "lark:"; the first is the input's and opens with what it was. A sample
// that demonstrates a failure therefore exits 1 and says so in its own words,
// without this command knowing anything about samples.
//
// A script's output is what it prints, which goes to standard output so it can
// be piped, as does a converted flow or script. Everything this command says
// about itself goes to standard error. With -logs the printed lines are kept as
// well, in <dir>/<input>.log, each line behind the lane that wrote it - a
// script's threads interleave, and a transcript has to say which said what.
//
// What the entry point returned is not printed. A script does its job and
// exits; its results are the lines it printed, which is what a host collects as
// the run's log. Printing the value too would put a trailing None under every
// script that correctly returns nothing.
//
// Revisions:
//   - 2026-09-19 23:40: initial creation
//   - 2026-09-19 23:41: initial creation
//   - 2026-09-19 23:47: tells a failed script apart from a command that could
//     not run one, by exit code and by the word the message opens with
//   - 2026-09-21 00:26: no longer prints what the entry point returned
//   - 2026-09-21 08:09: names the third exit code in full
//   - 2026-09-21 16:25: takes a graph as well as a script, and -t translates
//     either into the other, replacing -g as a flag that only printed
//   - 2026-09-21 16:42: keeps a transcript under -l
//   - 2026-09-21 17:19: compiles into a bundle under -b
//   - 2026-09-22 22:24: supplies a run's arguments under -a
//   - 2026-09-23 07:06: an interrupt stops the run and exits 4
//   - 2026-10-03 00:28: runs a bundle under -r
//   - 2026-10-03 21:05: runs or keeps whatever it was handed through one function
//     each, the artifact built by _Built
//   - 2026-10-03 23:38: drops a first case that repeated the default
//   - 2026-11-01 12:00: the flags carry full names only; -m is gone
//   - 2026-11-02 21:04: -graph leaves the end graph, -changes the changes; the
//     run is stopped, failed or done, and the graph is written all the same
//   - 2026-11-02 21:04: says when the graph or changes file could not be
//     written, exiting 3 unless the run had already failed or been stopped
//   - 2026-11-02 22:32: -memory turns on the memory watch (C27)
//   - 2026-11-03 23:57: takes the one _Run from _Prepared and exits through _Die (C57)
//   - 2026-11-05 00:23: reads the input through _Read, which takes - as standard input (C81)
//   - 2026-11-05 21:58: adds the cause line to a failure (C80, Q190)
//   - 2026-11-08 21:12: rejoins the doc that dcf3a69 left above _Prepared (C113, B232)
//   - 2026-11-10 11:32: calls Die, Report and Finish of command, which moved to
//     internal/command (C120b, Q315)
//   - 2026-11-10 11:45: reads the input through Read of command, which bounds it by the free
//     memory; SIGHUP stops a run as SIGTERM does (C119, B253, B256, Q23, Q299)
func main() {
	var (
		asked    = _Flags()
		src, err = command.Read(
			asked,
			os.Stdin,
			memwatch.Room,
		)
	)
	if err != nil {
		command.Die(
			err,
			command.NO_FILE,
		)
	}

	// Stopping is the signal cancelling this context, which reaches the
	// interpreter between instructions, so a script with no sleep in it stops
	// as readily as one that blocks.
	stopping, released := signal.NotifyContext(
		context.Background(),
		_Stops()...,
	)
	defer released()

	run := _Prepared(
		asked,
		released,
	)

	err = _Chose(
		stopping,
		asked,
		src,
		run,
	)

	// A graph file that could not be written is this command's problem, as a
	// file that could not be opened is. The run's own end wins when it was not a
	// success: that exit code says more.
	fault := run.Output.Fault()
	if fault != nil {
		command.Report(fault)

		if err == nil {
			_ = run.Close()

			os.Exit(command.NO_FILE)
		}
	}

	// The cause line is part of the failure text, so it prints under it.
	err = run.Caused(err)

	// Closed here rather than deferred, because this function exits and a
	// deferred close would not run.
	command.Finish(
		stopping,
		run.Close,
		err,
		asked.Noun,
	)
}

// _Stops is the signals that stop a run: an interrupt, a termination and a hang
// up, the three alike (Q299). A hang up the process was started with ignored, as
// nohup starts it, stays ignored: os/signal un-ignores a signal it is asked to
// catch, which would end a run that was meant to outlive its terminal.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, B256, Q299)
func _Stops() []os.Signal {
	stops := []os.Signal{
		os.Interrupt,
		syscall.SIGTERM,
	}

	if !signal.Ignored(syscall.SIGHUP) {
		stops = append(
			stops,
			syscall.SIGHUP,
		)
	}

	return stops
}

// _Chose runs, compiles, converts, or keeps what the command line asked for.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-01 12:30: flag names in the comment are the full names
//   - 2026-11-03 23:57: takes the _Run (C57)
//   - 2026-11-10 11:32: reads Asked and Run from command, calls SCRIPT and Running of command,
//     which moved to internal/command (C120b, Q315)
//   - 2026-11-10 11:45: chooses by verb (C119, Q305)
func _Chose(
	stopping context.Context,
	asked *command.Asked,
	src []byte,
	run *command.Run,
) error {
	switch {
	case asked.Verb == command.COMPILE:
		return command.Keeping(
			asked,
			src,
			run,
		)

	case asked.Verb == command.CONVERT && asked.Noun == command.SCRIPT:
		return _Derive(
			asked.Path,
			src,
		)

	case asked.Verb == command.CONVERT:
		return _Emit(
			asked.Path,
			src,
		)

	default:
		return command.Running(
			stopping,
			asked,
			src,
			run,
		)
	}
}

// _Supplying is the run options handing this run's script the arguments a
// caller passed as a JSON object, or none when it passed none.
//
// Wrong JSON is the invocation being wrong rather than the script, so it exits
// with the flags rather than with the input. A caller keeping arguments in a
// file passes the file: -args "$(cat args.json)".
//
// Returns a wrapped error for anything that is not a JSON object.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-02 13:12: one call, the runtime reading the JSON itself
//   - 2026-10-02 16:08: options of the run, the arguments being the script's
//     rather than the context's
//   - 2026-10-02 16:18: reads the JSON object into the Struct a run is handed
//   - 2026-10-03 16:30: reads it into the map a run is handed
//   - 2026-11-01 12:30: flag name in the comment is -args
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-13 12:50: reads each number as the text it was written in, so an int stays exact
//     and a number with a fraction or an exponent is a float, with command.Arguments (C132,
//     Q4, Q437)
func _Supplying(supplied string) ([]common.RunOption, error) {
	if supplied == "" {
		return nil, nil
	}

	args, err := command.Arguments(supplied)
	if err != nil {
		return nil, fmt.Errorf(
			"arguments: %w",
			err,
		)
	}

	return []common.RunOption{runtime.WithArgs(args)}, nil
}

// _Derive prints the flow of the script at path as JSON. Modules resolve beside
// the file, as they do for a run.
//
// The JSON is protojson's, laid out by encoding/json rather than by protojson,
// whose layout deliberately varies between runs so nobody compares it byte for
// byte. A command's output is compared byte for byte, in a diff.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as _Graph
//   - 2026-09-21 16:25: named for the direction it goes, now that the other
//     one exists
//   - 2026-10-02 01:45: prints a flow
//   - 2026-10-03 00:17: says nothing on standard error, a flow being the whole script
//     or refused
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
func _Derive(
	path string,
	src []byte,
) error {
	flow, err := runtime.Derive(&common.Source{
		Entry: path,
		Text:  src,
	})
	if err != nil {
		return err
	}

	compact, err := protojson.Marshal(flow)
	if err != nil {
		return fmt.Errorf(
			"flow: %w",
			err,
		)
	}

	var laid bytes.Buffer

	err = json.Indent(
		&laid,
		compact,
		"",
		INDENT,
	)
	if err != nil {
		return fmt.Errorf(
			"flow: %w",
			err,
		)
	}

	return _Written(laid.String())
}

// _Emit prints the Starlark the flow at path generates.
//
// The inverse of _Derive, and the whole of what a host needs to see before
// trusting a flow: what a user interface built, as the program it will run.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
//   - 2026-10-02 01:45: reads a flow
//   - 2026-11-10 11:32: calls Generated of command, which moved to internal/command (C120b,
//     Q315)
func _Emit(
	path string,
	src []byte,
) error {
	out, err := command.Generated(
		path,
		src,
	)
	if err != nil {
		return err
	}

	return _Written(string(out))
}

// _Written puts one piece of output where a caller can pipe it.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
func _Written(out string) error {
	_, err := fmt.Fprintln(
		os.Stdout,
		out,
	)
	if err != nil {
		return fmt.Errorf(
			"write: %w",
			err,
		)
	}

	return nil
}
