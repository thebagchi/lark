// Command lark runs a Starlark script or a workflow's flow, and translates
// either into the other.
//
//	lark -s samples/concurrent.star      run the script
//	lark -s samples/concurrent.star -t   print the flow it describes, as JSON
//	lark -g run.json                     run the flow
//	lark -g run.json -t                  print the Starlark it generates
//	lark -s build.star -l logs           run it, and keep a transcript
//	lark -s build.star -b build.bin      compile it into a bundle
//	lark -r build.bin                    run a bundle
//	lark -s build.star -a '{"n": 3}'     run it, supplying its arguments
//	lark -s build.star -m 64             run it, with 64MB of memory to use
//
// An interrupt stops a run: the script is told between instructions, the
// transcript is closed, and this exits 4.
//
// It is the smallest host this runtime supports. Given a script it compiles
// that script together with every module it loads and calls its entry point;
// modules resolve beside the file that loaded them, so a script in samples/
// loading "strings.star" reads samples/strings.star. Given a flow it checks
// it, generates a script and runs that - a flow names no modules, because
// deriving one inlines what its script loaded.
//
// The two translations are inverses, so a flow printed by the second line
// above generates the script the third line runs. Running either form runs the
// same workflow, which is what the round trip in runtime/graph measures.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"google.golang.org/protobuf/encoding/protojson"

	larkfile "github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote"
)

const (
	SCRIPT_FLAG     = "s"
	SCRIPT_USAGE    = "path to the Starlark script to run"
	GRAPH_FLAG      = "g"
	GRAPH_USAGE     = "path to the workflow's flow, as JSON, to run"
	TRANSLATE_FLAG  = "t"
	TRANSLATE_USAGE = "print the other form instead of running it"
	LOGS_FLAG       = "l"
	LOGS_USAGE      = "directory to keep this run's transcript in"
	BUNDLE_FLAG     = "b"
	BUNDLE_USAGE    = "compile into a bundle at this path instead of running"
	RUN_FLAG        = "r"
	RUN_USAGE       = "path to a bundle, as -b writes one, to run"
	ARGS_FLAG       = "a"
	ARGS_USAGE      = "the arguments this run supplies, as a JSON object"
	PLUGINS_FLAG    = "p"
	PLUGINS_USAGE   = "directory of lark-*.bin plugins to start and keep for this run"

	// PLUGIN_DIR is what the directory holding this run's plugin socket is
	// called, and PLUGIN_SOCKET the socket in it. Short, because a unix socket
	// path has a low length limit.
	PLUGIN_DIR    = "lark"
	PLUGIN_SOCKET = "p"

	// TOKEN_BYTES is how much secret a run mints for its plugins.
	TOKEN_BYTES  = 32
	MEMORY_FLAG  = "m"
	MEMORY_USAGE = "the memory this run may use, in megabytes"

	// MEGABYTE is what -m counts in, because a ceiling is written by a person
	// and nobody writes 268435456.
	MEGABYTE = 1 << 20

	// SCRIPT, FLOW and BUNDLE are what a failure calls the thing that failed,
	// so a reader knows which input was wrong.
	SCRIPT = "script"
	FLOW   = "flow"
	BUNDLE = "bundle"

	// INDENT is how a printed flow is laid out, one level per nesting.
	INDENT = "  "

	// BUNDLE_FILE is the mode a written bundle takes.
	BUNDLE_FILE = 0o640

	// LOG_DIRECTORY is the mode a transcript's directory is made with,
	// LOG_FILE the mode of the transcript itself, and LOG_SUFFIX what it is
	// called after the script it belongs to.
	LOG_DIRECTORY = 0o750
	LOG_FILE      = 0o640
	LOG_SUFFIX    = ".log"

	// GENERATED is appended to a flow's path to name the script it makes.
	// No file of that name exists, which is the point: a position in a
	// failure belongs to the generated script, and -t is how to see it.
	GENERATED = ".star"

	// A script or a graph that is wrong is not the same event as a command
	// that cannot do its job, and a caller acts on them differently: the
	// first means the input is wrong, the second means the invocation is.
	// They are told apart by exit code and by the word the message opens
	// with.
	FAILED   = 1
	NO_INPUT = 2
	NO_FILE  = 3

	// STOPPED is a run this command was asked to stop, by an interrupt or a
	// termination signal. Its own code, because a run somebody stopped did
	// not fail and a caller acts on the two differently.
	//
	// Read from whether the signal arrived, never from the error. A script
	// that cancels its own thread and joins it also meets ERR_CANCELLED, and
	// that script failed in the ordinary way - samples/cancel.star is exactly
	// that, and exits 1.
	STOPPED = 4
)

var (
	// ERR_ONE_INPUT is returned when no input was named, or more than one was.
	ERR_ONE_INPUT = errors.New("lark: give exactly one of -s, -g and -r")

	// ERR_ONE_OUTPUT is returned when two things to produce were named.
	ERR_ONE_OUTPUT = errors.New("lark: -t and -b produce different things")

	// ERR_ONLY_RUN is returned when a bundle is to be translated or bundled. It
	// carries no source to do either from, so it is only run.
	ERR_ONLY_RUN = errors.New("lark: a bundle is only run; -t and -b need -s or -g")

	// ERR_MEMORY is returned for a ceiling that is not a quantity of memory.
	ERR_MEMORY = errors.New("lark: -m is a number of megabytes, above zero")
)

// main runs or translates whichever input was named.
//
// Exits 0 on success, 1 when the input was wrong, 2 when the flags were and 3
// when a file it was given could not be opened. The last two are this command's
// problem and open with "lark:"; the first is the input's and opens with what
// it was. A sample that demonstrates a failure therefore exits 1 and says so in
// its own words, without this command knowing anything about samples.
//
// A script's output is what it prints, which goes to standard output so it can
// be piped, as does a printed graph or a generated script. Everything this
// command says about itself goes to standard error. With -l the printed lines
// are kept as well, in <dir>/<input>.log, each line behind the lane that wrote
// it - a script's threads interleave, and a transcript has to say which said
// what.
//
// What the entry point returned is not printed. A script does its job and
// exits; its results are the lines it printed, which is what a host collects as
// the run's log. Printing the value too would put a trailing None under every
// script that correctly returns nothing.
//
// Revisions:
//   - 2026-09-19 23:40: initial creation
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
//
// _Prepared is what to close when a run is over, and what the compile and the
// run are handed.
//
// One function because these are one job - everything a run is told before it
// starts - and they read in the order they are layered: where a transcript
// goes, what arguments were supplied, how much memory may be used, and which
// plugins are hosted.
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
func _Prepared(asked *_Asked, path string) (func() error, *_Handed) {
	logging, kept, err := _Logging(asked.logs, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	run, err := _Supplying(asked.supplied)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	memory, err := _Allowing(asked.memory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	plugins, hosted, err := _Hosting(asked.plugins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	// Both closes, in one call, because main exits rather than returns and a
	// deferred close would not run: the transcript has to be finished and the
	// plugins have to be seen off on every path out.
	closing := func() error {
		return errors.Join(kept(), hosted())
	}

	// Every plugin in the one WithPlugins, since a second would replace the
	// first: file, and whatever attached.
	given := append([]runtime.Plugin{&larkfile.Plugin{}}, plugins...)

	handed := &_Handed{
		compile: []runtime.CompileOption{runtime.WithPlugins(given...)},
		run:     slices.Concat(run, memory, []runtime.RunOption{logging}),
	}

	return closing, handed
}

// _Handed is what this command hands the runtime besides the source: the
// options a compile takes, and the options a run does.
type _Handed struct {
	compile []runtime.CompileOption
	run     []runtime.RunOption
}

// _Asked is everything the command line said, after the pairs that cannot be
// asked for together have been refused.
//
// A struct rather than eight returns, per CLAUDE.md: the caller would otherwise
// take them in an order nothing checks.
type _Asked struct {
	path      string
	noun      string
	translate bool
	logs      string
	bundle    string
	supplied  string
	memory    int
	plugins   string
}

// _Flags is the command line, read and checked.
//
// Exits rather than returning an error, because there is no caller but main and
// a usage message is what a person wants here rather than a wrapped cause.
//
// Revisions:
//   - 2026-09-26 03:20: initial creation, lifted out of main
//   - 2026-10-03 00:28: reads a bundle to run under -r, which takes neither -t nor -b
func _Flags() *_Asked {
	var (
		script    = flag.String(SCRIPT_FLAG, "", SCRIPT_USAGE)
		described = flag.String(GRAPH_FLAG, "", GRAPH_USAGE)
		bundled   = flag.String(RUN_FLAG, "", RUN_USAGE)
		translate = flag.Bool(TRANSLATE_FLAG, false, TRANSLATE_USAGE)
		logs      = flag.String(LOGS_FLAG, "", LOGS_USAGE)
		bundle    = flag.String(BUNDLE_FLAG, "", BUNDLE_USAGE)
		supplied  = flag.String(ARGS_FLAG, "", ARGS_USAGE)
		memory    = flag.Int(MEMORY_FLAG, 0, MEMORY_USAGE)
		plugins   = flag.String(PLUGINS_FLAG, "", PLUGINS_USAGE)
	)

	flag.Parse()

	named := 0

	for _, input := range []string{*script, *described, *bundled} {
		if input != "" {
			named++
		}
	}

	if named != 1 {
		fmt.Fprintln(os.Stderr, ERR_ONE_INPUT)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	translated := *translate || *bundle != ""
	if *bundled != "" && translated {
		fmt.Fprintln(os.Stderr, ERR_ONLY_RUN)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	if *translate && *bundle != "" {
		fmt.Fprintln(os.Stderr, ERR_ONE_OUTPUT)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	held := &_Asked{
		path:      *script,
		noun:      SCRIPT,
		translate: *translate,
		logs:      *logs,
		bundle:    *bundle,
		supplied:  *supplied,
		memory:    *memory,
		plugins:   *plugins,
	}

	if *described != "" {
		held.path = *described
		held.noun = FLOW
	}

	if *bundled != "" {
		held.path = *bundled
		held.noun = BUNDLE
	}

	return held
}

// main runs, translates or bundles the script or flow the command line names,
// or runs the bundle it names, and exits with what happened to it.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
//   - 2026-10-03 00:28: runs a bundle under -r
//   - 2026-10-03 21:05: runs or keeps whatever it was handed through one function
//     each, the artifact built by _Built
//   - 2026-10-03 23:38: drops a first case that repeated the default
func main() {
	asked := _Flags()

	path := asked.path
	noun := asked.noun

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	// Stopping is the signal cancelling this context, which reaches the
	// interpreter between instructions, so a script with no sleep in it stops
	// as readily as one that blocks.
	stopping, released := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer released()

	closing, handed := _Prepared(asked, path)

	// A bundle under -r reaches the default: _Flags refuses -r beside -t or -b.
	switch {
	case asked.bundle != "":
		err = _Keeping(asked, src, handed)
	case noun == SCRIPT && asked.translate:
		err = _Derive(path, src)
	case asked.translate:
		err = _Emit(path, src)
	default:
		err = _Running(stopping, asked, src, handed)
	}

	// Closed here rather than deferred, because this function exits and a
	// deferred close would not run. A transcript that could not be finished
	// is reported, unless the run already had something worse to say. The
	// plugins go the same way, and for the same reason: nothing deferred here
	// runs, so a deferred kill would leave them behind on every failing run.
	ending := closing()
	if err == nil {
		err = ending
	}

	if err == nil {
		return
	}

	if stopping.Err() != nil {
		fmt.Fprintf(os.Stderr, "%s stopped\n", noun)
		os.Exit(STOPPED)
	}

	fmt.Fprintf(os.Stderr, "%s failed: %v\n", noun, err)
	os.Exit(FAILED)
}

// _Logging is the run option sending what this run prints to a text logger,
// with the function that finishes its file.
//
// Standard output always, because that is where a person running a script
// looks. The file as well when a directory was given, named for the input with
// .log on the end, so a script run twice overwrites its own log. WithLog writes
// each line, with its time, thread and function, and the message last.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, as _Reporting
//   - 2026-10-02 13:12: the runtime's transcript, to standard output and the
//     file, rather than a reporter of this command's own
//   - 2026-10-02 16:21: a logger handed to the run, whose handler writes the
//     time and the layout, rather than a transcript on the context
//   - 2026-10-02 16:33: writes each line's message last
//   - 2026-10-03 16:31: through the runtime's WithLog, which writes as this did
func _Logging(dir string, path string) (runtime.RunOption, func() error, error) {
	if dir == "" {
		nothing := func() error {
			return nil
		}

		return runtime.WithLog(os.Stdout), nothing, nil
	}

	err := os.MkdirAll(dir, LOG_DIRECTORY)
	if err != nil {
		return nil, nil, fmt.Errorf("log %s: %w", dir, err)
	}

	name := filepath.Join(dir, filepath.Base(path)+LOG_SUFFIX)

	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, LOG_FILE)
	if err != nil {
		return nil, nil, fmt.Errorf("log %s: %w", name, err)
	}

	return runtime.WithLog(io.MultiWriter(os.Stdout, file)), file.Close, nil
}

// _Supplying is the run options handing this run's script the arguments a
// caller passed as a JSON object, or none when it passed none.
//
// Wrong JSON is the invocation being wrong rather than the script, so it exits
// with the flags rather than with the input. A caller keeping arguments in a
// file passes the file: -a "$(cat args.json)".
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
func _Supplying(supplied string) ([]runtime.RunOption, error) {
	if supplied == "" {
		return nil, nil
	}

	var args map[string]any

	err := json.Unmarshal([]byte(supplied), &args)
	if err != nil {
		return nil, fmt.Errorf("arguments: %w", err)
	}

	return []runtime.RunOption{runtime.WithArgs(args)}, nil
}

// _Allowing is the option holding a run to the memory ceiling it was given, or
// none when it was given none.
//
// Megabytes rather than bytes, because a ceiling is written by a person and
// nobody writes 268435456. The library's own default stands when the flag is
// absent, so leaving it out is not the same as asking for nothing.
//
// Revisions:
//   - 2026-09-24 23:38: initial creation
//   - 2026-10-03 08:32: an option of the run, rather than a context carrying the
//     ceiling
func _Allowing(memory int) ([]runtime.RunOption, error) {
	if memory == 0 {
		return nil, nil
	}

	if memory < 0 {
		return nil, ERR_MEMORY
	}

	return []runtime.RunOption{runtime.WithMemory(int64(memory) * MEGABYTE)}, nil
}

// _Built is the artifact what was read from the file at asked.path describes:
// a script, compiled; a bundle -b wrote, loaded; or a flow, its script
// generated and compiled. Each takes the plugins a compile here is given -
// file, and whatever -p hosts - since compiled code names what it calls, and a
// bundle -b wrote was compiled with them.
//
// A script is compiled without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path.
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from the three ways a run or a bundle was
//     built, each beside what it was built for
func _Built(asked *_Asked, src []byte, opts []runtime.CompileOption) (*runtime.Artifact, error) {
	switch asked.noun {
	case SCRIPT:
		return runtime.Compile(&runtime.Source{Entry: asked.path, Text: src}, opts...)

	case BUNDLE:
		built, err := runtime.Load(src, opts...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", asked.path, err)
		}

		return built, nil

	default:
		// A flow, the one noun left.
		return _Compiled(asked.path, src, opts...)
	}
}

// _Running builds what was read and calls its entry point, with what it prints
// going where the run's options say.
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from _Run, _RunBundle and _Play, which
//     differed only in how the artifact was built
func _Running(ctx context.Context, asked *_Asked, src []byte, handed *_Handed) error {
	built, err := _Built(asked, src, handed.compile)
	if err != nil {
		return err
	}

	_, err = runtime.Start(ctx, built, handed.run...).Wait()

	return err
}

// _Keeping builds what was read and writes it as a bundle to asked.bundle.
//
// Revisions:
//   - 2026-10-03 21:05: initial creation, from _Bundle and _BundleOf, which differed
//     only in how the artifact was built
func _Keeping(asked *_Asked, src []byte, handed *_Handed) error {
	built, err := _Built(asked, src, handed.compile)
	if err != nil {
		return err
	}

	return _Kept(built, asked.bundle)
}

// _Kept writes a bundle.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
//   - 2026-09-21 23:47: says nothing about the graph, which a bundle either
//     has or has not
func _Kept(built *runtime.Artifact, out string) error {
	bundle, err := built.Save()
	if err != nil {
		return err
	}

	err = os.WriteFile(out, bundle, BUNDLE_FILE)
	if err != nil {
		return fmt.Errorf("bundle: %w", err)
	}

	return nil
}

// _Compiled is the artifact the flow at path describes.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
//   - 2026-10-02 01:45: reads a flow, and carries nothing beside the program
func _Compiled(
	path string,
	src []byte,
	opts ...runtime.CompileOption,
) (*runtime.Artifact, error) {
	out, err := _Generated(path, src)
	if err != nil {
		return nil, err
	}

	return runtime.Compile(&runtime.Source{Entry: path + GENERATED, Text: out}, opts...)
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
func _Derive(path string, src []byte) error {
	flow, err := runtime.Derive(&runtime.Source{Entry: path, Text: src})
	if err != nil {
		return err
	}

	compact, err := protojson.Marshal(flow)
	if err != nil {
		return fmt.Errorf("flow: %w", err)
	}

	var laid bytes.Buffer

	err = json.Indent(&laid, compact, "", INDENT)
	if err != nil {
		return fmt.Errorf("flow: %w", err)
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
func _Emit(path string, src []byte) error {
	out, err := _Generated(path, src)
	if err != nil {
		return err
	}

	return _Written(string(out))
}

// _Generated is the Starlark the flow at path generates: decoded, then
// written, Emit checking it first, so a flow nobody validated is refused with
// what is wrong with it rather than becoming a script that fails somewhere
// else.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation, as _Generated
//   - 2026-09-21 17:19: returns the graph as well, as _Described
//   - 2026-10-02 01:45: reads a flow, and returns only the script, since a
//     bundle no longer carries what it came from
//   - 2026-10-03 16:29: leaves the check to Emit, which now makes it
func _Generated(path string, src []byte) ([]byte, error) {
	described := new(runtime.Flow)

	err := protojson.Unmarshal(src, described)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return runtime.Emit(described)
}

// _Written puts one piece of output where a caller can pipe it.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
func _Written(out string) error {
	_, err := fmt.Fprintln(os.Stdout, out)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// _Hosting starts the plugins in dir and answers with the ones that attached,
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
func _Hosting(dir string) ([]runtime.Plugin, func() error, error) {
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

	// Its own directory, so the socket is short: a unix socket path has a low
	// length limit and a temporary name under it is what fits.
	held, err := os.MkdirTemp("", PLUGIN_DIR)
	if err != nil {
		return nil, nothing, fmt.Errorf("making somewhere for the plugin socket: %w", err)
	}

	// What is made is undone on every way out that fails, since a caller handed
	// an error exits on it without asking what else it was handed.
	listener, err := remote.Listen(filepath.Join(held, PLUGIN_SOCKET), token)
	if err != nil {
		return nil, nothing, errors.Join(err, os.RemoveAll(held))
	}

	closing := func() error {
		err := listener.Close()

		return errors.Join(err, os.RemoveAll(held))
	}

	loading, err := listener.Load(dir, "")
	if err != nil {
		return nil, nothing, errors.Join(err, closing())
	}

	for _, skipped := range loading.Skipped {
		fmt.Fprintf(os.Stderr, "lark: %v\n", skipped)
	}

	// Only what attached: a compiler adds these to the runtime's own.
	return listener.Plugins(), closing, nil
}

// _Token is a secret this run shares with the plugins it starts.
//
// Revisions:
//   - 2026-09-26 00:34: initial creation
func _Token() (string, error) {
	held := make([]byte, TOKEN_BYTES)

	_, err := rand.Read(held)
	if err != nil {
		return "", fmt.Errorf("minting a plugin token: %w", err)
	}

	return hex.EncodeToString(held), nil
}
