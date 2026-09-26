// Command lark runs a Starlark script or a workflow graph, and translates
// either into the other.
//
//	lark -s samples/concurrent.star      run the script
//	lark -s samples/concurrent.star -t   print the graph it describes, as JSON
//	lark -g run.json                     run the graph
//	lark -g run.json -t                  print the Starlark it generates
//	lark -s build.star -l logs           run it, and keep a transcript
//	lark -s build.star -b build.bin      compile it into a bundle
//	lark -s build.star -a '{"n": 3}'     run it, supplying its arguments
//	lark -s build.star -m 64             run it, with 64MB of memory to use
//
// An interrupt stops a run: the script is told between instructions, the
// transcript is closed, and this exits 4.
//
// It is the smallest host this runtime supports. Given a script it compiles
// that script together with every module it loads and calls its entry point;
// modules resolve beside the file that loaded them, so a script in samples/
// loading "strings.star" reads samples/strings.star. Given a graph it checks
// it, generates a script and runs that - a graph names no modules, because
// deriving one inlines what its script loaded.
//
// The two translations are inverses, so a graph printed by the first line
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
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/args"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/file"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/random"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/time"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/utils"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	SCRIPT_FLAG     = "s"
	SCRIPT_USAGE    = "path to the Starlark script to run"
	GRAPH_FLAG      = "g"
	GRAPH_USAGE     = "path to the workflow graph, as JSON, to run"
	TRANSLATE_FLAG  = "t"
	TRANSLATE_USAGE = "print the other form instead of running it"
	LOGS_FLAG       = "l"
	LOGS_USAGE      = "directory to keep this run's transcript in"
	BUNDLE_FLAG     = "b"
	BUNDLE_USAGE    = "compile into a bundle at this path instead of running"
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

	// SCRIPT and GRAPH are what a failure calls the thing that failed, so a
	// reader knows which of the two inputs was wrong.
	SCRIPT = "script"
	GRAPH  = "graph"

	// INDENT is how a printed graph is laid out, one level per nesting.
	INDENT = "  "

	// BUNDLE_FILE is the mode a written bundle takes.
	BUNDLE_FILE = 0o640

	// GENERATED is appended to a graph's path to name the script it makes.
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
	// that cancels its own thread and joins it also meets ErrCancelled, and
	// that script failed in the ordinary way - samples/cancel.star is exactly
	// that, and exits 1.
	STOPPED = 4
)

var (
	// ErrOneInput is returned when neither input was named, or both were.
	ErrOneInput = errors.New("lark: give exactly one of -s and -g")

	// ErrOneOutput is returned when two things to produce were named.
	ErrOneOutput = errors.New("lark: -t and -b produce different things")

	// ErrMemory is returned for a ceiling that is not a quantity of memory.
	ErrMemory = errors.New("lark: -m is a number of megabytes, above zero")
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
func main() {
	var (
		script    = flag.String(SCRIPT_FLAG, "", SCRIPT_USAGE)
		described = flag.String(GRAPH_FLAG, "", GRAPH_USAGE)
		translate = flag.Bool(TRANSLATE_FLAG, false, TRANSLATE_USAGE)
		logs      = flag.String(LOGS_FLAG, "", LOGS_USAGE)
		bundle    = flag.String(BUNDLE_FLAG, "", BUNDLE_USAGE)
		supplied  = flag.String(ARGS_FLAG, "", ARGS_USAGE)
		memory    = flag.Int(MEMORY_FLAG, 0, MEMORY_USAGE)
		plugins   = flag.String(PLUGINS_FLAG, "", PLUGINS_USAGE)
	)

	flag.Parse()

	if (*script == "") == (*described == "") {
		fmt.Fprintln(os.Stderr, ErrOneInput)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	if *translate && *bundle != "" {
		fmt.Fprintln(os.Stderr, ErrOneOutput)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	path := *script
	noun := SCRIPT

	if *described != "" {
		path = *described
		noun = GRAPH
	}

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	// Stopping is the signal cancelling this context, which reaches the
	// interpreter between instructions, so a script with no sleep in it stops
	// as readily as one that blocks.
	stopping, released := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer released()

	ctx, kept, err := _Reporting(stopping, *logs, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	ctx, err = _Supplying(ctx, *supplied)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	ctx, err = _Allowing(ctx, *memory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(NO_INPUT)
	}

	opts, hosted, err := _Hosting(*plugins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lark: %v\n", err)
		os.Exit(NO_FILE)
	}

	switch {
	case noun == SCRIPT && *bundle != "":
		err = _Bundle(path, src, *bundle, opts...)
	case *bundle != "":
		err = _BundleOf(path, src, *bundle, opts...)
	case noun == SCRIPT && *translate:
		err = _Derive(path, src)
	case noun == SCRIPT:
		err = _Run(ctx, path, src, opts...)
	case *translate:
		err = _Emit(path, src)
	default:
		err = _Play(ctx, path, src, opts...)
	}

	// Closed here rather than deferred, because this function exits and a
	// deferred close would not run. A transcript that could not be finished
	// is reported, unless the run already had something worse to say. The
	// plugins go the same way, and for the same reason: nothing deferred here
	// runs, so a deferred kill would leave them behind on every failing run.
	closing := errors.Join(kept(), hosted())
	if err == nil {
		err = closing
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

// _Console is what this command tells a run: print to standard output, and
// keep the line in the run's own file when one was asked for.
//
// A host writing its own reporter is what the interface is for. This one hears
// the printing and ignores the rest, because a command shows a script's output
// and not its progress.
type _Console struct {
	log *runtime.Log
}

// Started is empty: this command reports what a script printed, not what it
// did.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (c *_Console) Started(thread string, name string, attempt int32) {
	// Empty
}

// Ended is empty, for the same reason as Started.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (c *_Console) Ended(thread string, name string, err error) {
	// Empty
}

// Printed puts the line on standard output, and behind its lane in the
// transcript.
//
// Standard output carries the line as the script wrote it, because that is
// what a pipe on the other end is reading. The transcript carries the lane as
// well, because a concurrent script interleaves and a file that did not say
// which thread spoke would be a transcript of neither.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (c *_Console) Printed(thread string, msg string) {
	_, err := fmt.Fprintln(os.Stdout, msg)
	if err != nil {
		// The script's line is lost and the script does not know. Standard
		// error is this command's own channel, so that is where it is said.
		fmt.Fprintf(os.Stderr, "lark: print: %v\n", err)
	}

	if c.log != nil {
		c.log.Printed(thread, msg)
	}
}

// _Close finishes the transcript, if there is one.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (c *_Console) _Close() error {
	if c.log == nil {
		return nil
	}

	return c.log.Close()
}

// _Reporting is ctx carrying what a run should tell this command, and the
// function that finishes the transcript.
//
// The file is named for the input with .log on the end, under the directory
// given, so a script run twice overwrites its own transcript. A run started
// through a store is named for its id instead, because two runs of one
// artifact are two transcripts and an id is what tells them apart.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func _Reporting(ctx context.Context, dir string, path string) (context.Context, func() error, error) {
	console := new(_Console)

	if dir == "" {
		return runtime.WithReporter(ctx, console), console._Close, nil
	}

	made, err := runtime.NewLog(filepath.Join(dir, filepath.Base(path)+runtime.LOG_SUFFIX))
	if err != nil {
		return nil, nil, err
	}

	console.log = made

	return runtime.WithReporter(ctx, console), console._Close, nil
}

// _Supplying is ctx carrying the arguments this run supplies, read from the
// JSON object a caller passed.
//
// Wrong JSON is the invocation being wrong rather than the script, so it exits
// with the flags rather than with the input. A caller keeping arguments in a
// file passes the file: -a "$(cat args.json)".
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Supplying(ctx context.Context, supplied string) (context.Context, error) {
	if supplied == "" {
		return ctx, nil
	}

	parsed, err := runtime.Parsed([]byte(supplied))
	if err != nil {
		return nil, err
	}

	return runtime.WithArgs(ctx, parsed), nil
}

// _Allowing is ctx carrying the memory ceiling this run was given, or ctx
// unchanged when it was given none.
//
// Megabytes rather than bytes, because a ceiling is written by a person and
// nobody writes 268435456. The library's own default stands when the flag is
// absent, so leaving it out is not the same as asking for nothing.
//
// Revisions:
//   - 2026-09-24 23:38: initial creation
func _Allowing(ctx context.Context, memory int) (context.Context, error) {
	if memory == 0 {
		return ctx, nil
	}

	if memory < 0 {
		return nil, ErrMemory
	}

	return scheduler.Allowing(ctx, int64(memory)*MEGABYTE), nil
}

// _Run compiles src as the script at path and calls its entry point, with
// what it prints going to standard output.
//
// The compiler is built without a loader, so a module resolves beside the file
// that loaded it. That is what lets a script name a neighbour by its bare name
// while the script itself is named by a path.
//
// Revisions:
//   - 2026-09-19 23:41: initial creation
//   - 2026-09-19 23:47: takes the source, so reading the file is the caller's
//     problem and can be reported as one
//   - 2026-09-21 00:26: reports only whether the run failed, since the value is
//     no longer printed
//   - 2026-09-21 08:09: sends what the script prints to standard output, which
//     the doc had claimed and the interpreter's default did not do
//   - 2026-09-21 16:42: takes the reporter already on ctx, since a transcript
//     wants the lane and a printer is handed only the line
func _Run(
	ctx context.Context,
	path string,
	src []byte,
	opts ...runtime.CompilerOption,
) error {
	built, err := runtime.NewCompiler(opts...).Compile(path, src)
	if err != nil {
		return err
	}

	_, err = built.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

// _Bundle compiles the script at path and writes the bundle to out.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func _Bundle(path string, src []byte, out string, opts ...runtime.CompilerOption) error {
	built, err := runtime.NewCompiler(opts...).Compile(path, src)
	if err != nil {
		return err
	}

	return _Kept(built, out)
}

// _BundleOf compiles the script the graph at path describes and writes the
// bundle to out, carrying the graph that came in rather than one derived from
// the script it generated.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func _BundleOf(path string, src []byte, out string, opts ...runtime.CompilerOption) error {
	built, err := _Compiled(path, src, opts...)
	if err != nil {
		return err
	}

	return _Kept(built, out)
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

// _Compiled is the artifact a graph describes, carrying that graph.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func _Compiled(
	path string,
	src []byte,
	opts ...runtime.CompilerOption,
) (*runtime.Artifact, error) {
	described, out, err := _Described(path, src)
	if err != nil {
		return nil, err
	}

	return runtime.NewCompiler(append(opts, runtime.WithAuthored(described))...).
		Compile(path+GENERATED, out)
}

// _Derive prints the graph of the script at path as JSON.
//
// What the graph could not carry is said on standard error, one line each, so
// the JSON on standard output is only the graph and a reader piping it still
// sees what was left behind. Modules resolve beside the file, as they do for a
// run.
//
// The JSON is protojson's, laid out by encoding/json rather than by protojson,
// whose layout deliberately varies between runs so nobody compares it byte for
// byte. A command's output is compared byte for byte, in a diff.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as _Graph
//   - 2026-09-21 16:25: named for the direction it goes, now that the other
//     one exists
func _Derive(path string, src []byte) error {
	report, err := graph.Of(src, path, nil)
	if err != nil {
		return err
	}

	for _, unmodelled := range report.Unknown {
		fmt.Fprintf(os.Stderr, "lark: drawn as text: %s\n", unmodelled)
	}

	compact, err := protojson.Marshal(report.Graph)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}

	var laid bytes.Buffer

	err = json.Indent(&laid, compact, "", INDENT)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}

	return _Written(laid.String())
}

// _Emit prints the Starlark the graph at path generates.
//
// The inverse of _Derive, and the whole of what a host needs to see before
// trusting a graph: what a user interface built, as the program it will run.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
func _Emit(path string, src []byte) error {
	_, out, err := _Described(path, src)
	if err != nil {
		return err
	}

	return _Written(string(out))
}

// _Play generates the script the graph at path describes and runs it.
//
// The script is named for the graph with .star on the end. No file of that
// name exists, and that is deliberate: a position in a failure belongs to the
// generated script rather than to the JSON, and -t is how to see the source it
// counts lines in.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
func _Play(
	ctx context.Context,
	path string,
	src []byte,
	opts ...runtime.CompilerOption,
) error {
	built, err := _Compiled(path, src, opts...)
	if err != nil {
		return err
	}

	_, err = built.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

// _Described is the graph at path and the Starlark it generates: decoded,
// checked, written.
//
// Checked before generating, because Emit does not check and a host that
// generates from a graph nobody validated gets a script that fails somewhere
// else for a reason nothing connects to the graph.
//
// The graph comes back beside the source because what compiles it should
// carry it: a bundle built this way shows what its author drew, not what
// deriving the generated script gives back.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation, as _Generated
//   - 2026-09-21 17:19: returns the graph as well
func _Described(path string, src []byte) (*workflowpb.Graph, []byte, error) {
	described := new(workflowpb.Graph)

	err := protojson.Unmarshal(src, described)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	err = graph.Check(described)
	if err != nil {
		return nil, nil, err
	}

	out, err := graph.Emit(described)
	if err != nil {
		return nil, nil, err
	}

	return described, out, nil
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

// _Hosting starts the plugins in dir and answers with what a compiler needs to
// see them, and with how to see them off.
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
// on without it. Returns an error only when dir itself could not be read.
//
// Revisions:
//   - 2026-09-26 00:34: initial creation
func _Hosting(dir string) ([]runtime.CompilerOption, func() error, error) {
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

	registry := plugin.New()

	// Everything a blank import installed, so plugins arrive beside the names
	// this binary already has rather than instead of them.
	for _, installed := range plugin.DEFAULT.Registered() {
		registry.Register(installed)
	}

	listener, err := remote.Listen(filepath.Join(held, PLUGIN_SOCKET), token, registry)
	if err != nil {
		return nil, nothing, err
	}

	closing := func() error {
		err := listener.Close()

		return errors.Join(err, os.RemoveAll(held))
	}

	loading, err := listener.Load(dir, "")
	if err != nil {
		return nil, closing, err
	}

	for _, skipped := range loading.Skipped {
		fmt.Fprintf(os.Stderr, "lark: %v\n", skipped)
	}

	return []runtime.CompilerOption{runtime.WithPlugins(registry)}, closing, nil
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
