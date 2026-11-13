// Package runtime compiles Starlark scripts and runs them concurrently.
//
// This comment is the guide for a host: a Go program that embeds the runtime.
// The guide for a person who writes scripts is README.md, in the root of the
// repository.
//
// A host imports this package for the calls. The types that the calls take and
// return are in the package common beside it: Source, Artifact, Execution and the
// rest. Common holds the data types the API returns. A host may read them, but
// should not build on its other types. The graph types Flow, Graph and Change come
// from proto/gen/workflow. This package holds functions and errors only.
//
// To add the runtime to a module, run go get github.com/thebagchi/lark.
//
// # Which call runs a script
//
//   - Run, when the host wants the value main returned and nothing more. It
//     compiles with the runtime's own plugins, so it cannot hand a script a
//     plugin of the host's. See the example for Run.
//   - Eval, when the host wants the value, the graph and the prints at the
//     end. It runs on the caller's goroutine, so the call returns when the run
//     is over. Compile the script first. See the example for Eval.
//   - Start, when the host must do something while the run goes on: watch its
//     graph with Status, stop it by cancelling the context, or wait on it from
//     elsewhere. It starts a goroutine for the run. See the example for Start,
//     and its cancel example.
//
// Run compiles and runs in one call. Compile and Start are its two halves. Use
// them when a host adds its own plugins, reads the graph of a run, or stops a
// run. A host gives the options of a run to Start or Eval, for one run at a
// time. Each option has the same meaning on each run. The context carries only
// the cancellation.
//
// # The toolchain
//
// The module's go.mod names go 1.26.10. A Go older than that fetches this
// toolchain on first use, so a host on an older Go downloads it. The Go command
// prints "switching to go1.26.10" when it does.
//
// # Nil values
//
// A nil or zero value handed to this package does not crash the host:
//
//   - a nil source: Compile, Merge and Run return ErrSource;
//   - a nil artifact: Start and Eval return ErrArtifact, as they do for
//     an artifact lark did not compile;
//   - a nil option, a nil plugin, or an interface holding a nil pointer as a
//     plugin: skipped;
//   - an Execution is nil when Start returns an error, and a nil Execution must
//     not be used: check the error first;
//   - a nil Handle: finished, with no value, no error and nothing to stop.
//
// One nil stays the host's to avoid: a nil *Result, since Go itself refuses to
// read a field of it. Eval never returns one.
//
// # What a run tells its host
//
// A run tells its host two things, and nothing else is reported, so nothing else
// is paid for:
//
//   - Its graph. Status returns all of the graph at the end of the run. During
//     the run, Status returns an empty graph whose status is STATUS_RUNNING,
//     unless the host asks for the live graph with WithTracking. WithChanges
//     sends each change as a JSON Patch, from {}, and asks for the live graph
//     too.
//   - What its script printed. This goes to the logger that the run gets,
//     which WithLog names. Eval hands a host the graph and
//     the prints at the end instead, beside the value main returned.
//
// # The scripts in this guide
//
// The snippets below are written as a host writes them, to the conventions of
// this repository, with these changes to save space. A snippet is code from the
// body of a function that has a ctx and returns an error. A type or a plugin is
// shown complete. A snippet shows no imports: common is
// github.com/thebagchi/lark/v1/runtime/common, and workflowpb is
// workflowpb "github.com/thebagchi/lark/proto/gen/workflow". Doc comments in a
// snippet have no revision history. The snippets share built, the artifact that
// lark compiled from build.star, and these constants:
//
//	const (
//		// SCRIPT is the script the examples compile, and STEPS the module it loads.
//		SCRIPT = "build.star"
//		STEPS  = "steps.star"
//
//		// LOG_FILE is the file a run's printed lines are logged to, BUNDLE the file
//		// a saved artifact is written to, and BUNDLE_MODE that file's permissions.
//		LOG_FILE    = "build.log"
//		BUNDLE      = "build.bundle"
//		BUNDLE_MODE = 0o640
//
//		// LIMIT is how long a started run may take before the host stops it, and
//		// EVERY how often the host reports on a run that is still going.
//		LIMIT = 30 * time.Second
//		EVERY = time.Second
//	)
//
// build.star loads one function from steps.star:
//
//	load("steps.star", "fetch")
//
//	def main():
//	    fetch("lark")
//	    print("built")
//	    return "ok"
//
// steps.star holds fetch:
//
//	def fetch(name):
//	    print("fetching " + name)
//	    return name
//
// The output beside a snippet is what it printed when it ran.
//
// # Compile and Load
//
// A script names its modules itself. If the Source has no Loader, a module is a
// file beside the script that loads it. Thus lark reads steps.star from beside
// build.star:
//
//	src, err := os.ReadFile(SCRIPT)
//	if err != nil {
//		return err
//	}
//
//	built, err := runtime.Compile(&common.Source{Entry: SCRIPT, Text: src})
//	if err != nil {
//		return err
//	}
//
// With a loader, the modules come from where the loader keeps them. A loader
// can get modules from a directory, an archive, a database or a map in memory.
// The runtime works the same with each of them. See the section Loader below.
// _Memory, the loader in that section, keeps them in a map. The variables build
// and steps hold the source of the two scripts:
//
//	modules := &_Memory{
//		files: map[string][]byte{
//			SCRIPT: build,
//			STEPS:  steps,
//		},
//	}
//
//	built, err := runtime.Compile(&common.Source{
//		Entry:  SCRIPT,
//		Text:   build,
//		Loader: modules,
//	})
//
//	// A ring of loads, or a call with the wrong arguments, is the script's to fix,
//	// and the error says where.
//	wrong := errors.Is(err, runtime.ErrCycle) || errors.Is(err, runtime.ErrArgs)
//	if wrong {
//		return fmt.Errorf("fix %s: %w", SCRIPT, err)
//	}
//
//	if err != nil {
//		return err
//	}
//
// Compile refuses these, and each error is a value of this package:
//
//   - steps.star loads build.star again: "cycle in the load graph: build.star ->
//     steps.star -> build.star", which matches ErrCycle.
//   - fetch("lark", "now"): "build.star:4:5: call of fetch: a call passes
//     arguments the function does not take", which matches ErrArgs.
//   - a steps.star that the loader cannot find: "cannot load "steps.star" from
//     build.star: module steps.star: file does not exist".
//
// Compile builds the whole load graph before anything runs. It refuses a ring of
// loads before one top-level statement runs. It gets each module one time for
// each compile, however many scripts load it.
//
// A compile does not refuse a script without an entry point, because a library
// has none, and a library compiles. A run refuses it: Wait returns ErrMain.
// Save and Derive also refuse it, each before anything runs.
//
// Save writes an artifact as one bundle, and Load reads it back with no loader
// and no source. Thus a host can compile one time and run in a different place.
// Load takes the options that the compile took. Here there are none, so it uses
// the plugins of the runtime itself:
//
//	bundle, err := built.Save()
//	if err != nil {
//		return err
//	}
//
//	err = os.WriteFile(BUNDLE, bundle, BUNDLE_MODE)
//	if err != nil {
//		return err
//	}
//
//	// Elsewhere, later.
//	saved, err := os.ReadFile(BUNDLE)
//	if err != nil {
//		return err
//	}
//
//	loaded, err := runtime.Load(saved)
//	if err != nil {
//		return err
//	}
//
//	run, err := runtime.Start(ctx, loaded)
//	if err != nil {
//		return err
//	}
//
//	value, err := run.Wait()
//
// Here value is "ok", as it is for built. Save refuses an artifact that a run
// cannot start, because a host reads a bundle back to run it. steps.star is a
// module, and Save gives "steps.star: no entry point (define def main() with no
// required parameters; a file without it is a module, for load)", which matches
// ErrMain. Load refuses bytes that are not a bundle, with "unmarshal bundle:
// proto: cannot parse invalid wire-format data". It refuses a bundle that does
// not hold a module that one of its units loads, with ErrUnit. It refuses a
// bundle whose script uses a plugin of the host that Load was not given, with
// ErrPlugin: the text names the plugin. Give Load the same WithPlugins
// that Compile had. A bundle written before the names were saved is loaded with
// no such check.
//
// # Loader
//
// A Loader is the interface a host implements to give a script its modules. It
// has two methods:
//
//   - Resolve(from string, target string) (string, error) gives the identity of
//     the module that a script in from spells target. Two spellings of one
//     module resolve to one name, so lark builds the module one time.
//   - Load(name string) ([]byte, error) gives the source of the module that
//     Resolve named.
//
// A host gives a loader as the Loader of a Source. This loader serves modules
// from a map, keyed by the name a script loads each one by, as a test or a host
// that keeps its scripts in a database would:
//
//	// _Memory serves modules from a map, keyed by the name a script loads each
//	// one by, as a test or a host keeping its scripts in a database would.
//	type _Memory struct {
//		files map[string][]byte
//	}
//
//	// Resolve names a module by its spelling alone. There are no directories, so
//	// "steps.star" is one module whichever script loads it, and it is built once.
//	func (m *_Memory) Resolve(from string, path string) (string, error) {
//		return path, nil
//	}
//
//	// Load is the source of the module Resolve named.
//	func (m *_Memory) Load(name string) ([]byte, error) {
//		src, found := m.files[name]
//		if !found {
//			return nil, fmt.Errorf("module %s: %w", name, fs.ErrNotExist)
//		}
//
//		return src, nil
//	}
//
// A loader that has directories resolves target relative to from, as the
// default loader does. Thus lib/steps.star, loaded from ci/build.star, is
// ci/lib/steps.star. This loader reads a directory:
//
//	type Disk struct{ root string }
//
//	func (d *Disk) Resolve(from string, target string) (string, error) {
//		return path.Join(path.Dir(from), target), nil
//	}
//
//	func (d *Disk) Load(name string) ([]byte, error) {
//		return os.ReadFile(filepath.Join(d.root, name))
//	}
//
// # Plugins
//
// Importing this package gives a script every plugin of the runtime's own:
// spawn, join, cancel, assert and sleep from core, and args, flow, state, event,
// queue, json, math, time and the rest. This package imports each of them, and each
// registers itself. They are always there.
//
// A host adds its own beside them with WithPlugins, for one compile. That is
// also how a script gets the file system: file is a plugin a host hands over,
// never one of the runtime's own. A plugin is a separate package. A host imports
// it to give it to a script, or to name its errors. See the example for
// WithPlugins.
//
// WithPlugins takes all the plugins of the host at one time, and a second
// WithPlugins is refused: the compile returns an error that names it. The file
// plugin, from github.com/thebagchi/lark/v1/plugin/file, can get to all that the
// host process can get to. Thus a script has it only when its host gives it. A
// host that runs scripts from other persons does not give it. Without it, a
// compile refuses a script that names it, before anything runs. For example, a
// script that calls file.read:
//
//	def main():
//	    return file.read("notes.txt")
//
// gives this error:
//
//	reads.star:2:12: undefined: file
//
// A plugin cannot take a name that another plugin supplies, and this includes
// the plugins of the runtime itself. A hidden builtin is a defect, and it would
// show much later, as incorrect behavior in a different place. The compile that
// builds the environment refuses such a plugin, and names both plugins. The
// error matches ErrConflict. For example, a plugin with the name mine that
// supplies its own sleep:
//
//	compile build.star: mine supplies "sleep", which core already supplies:
//	two plugins supply one name
//
// # Plugin
//
// A Plugin is the interface a host implements to give a script names of its own:
//
//   - Name() string is the name of the plugin, so that an error about two
//     plugins that supply one name can name it.
//   - Values() starlark.StringDict holds the names that the plugin gives a
//     script.
//
// This plugin gives a script setting(name):
//
//	const (
//		// SETTINGS is what this plugin is called, and SETTING the builtin it
//		// gives a script.
//		SETTINGS = "settings"
//		SETTING  = "setting"
//	)
//
//	// _Settings gives a script setting(name): a value from the host's own
//	// configuration, which a script has no other way to reach.
//	type _Settings struct {
//		values map[string]string
//	}
//
//	// Name is what a clash calls this plugin.
//	func (s *_Settings) Name() string {
//		return SETTINGS
//	}
//
//	// Values is the one name this plugin gives a script.
//	func (s *_Settings) Values() starlark.StringDict {
//		return starlark.StringDict{
//			SETTING: starlark.NewBuiltin(SETTING, s._Setting),
//		}
//	}
//
//	// _Setting is setting(name): what the host holds under name, or None when it
//	// holds nothing there.
//	func (s *_Settings) _Setting(
//		thread *starlark.Thread,
//		fn *starlark.Builtin,
//		args starlark.Tuple,
//		kwargs []starlark.Tuple,
//	) (starlark.Value, error) {
//		var name string
//
//		err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &name)
//		if err != nil {
//			return nil, err
//		}
//
//		value, found := s.values[name]
//		if !found {
//			return starlark.None, nil
//		}
//
//		return starlark.String(value), nil
//	}
//
// In a script that is compiled with this plugin, region = setting("region") is
// "eu-west-1".
//
// # Derive, Check and Emit
//
// Derive reads a script into the flow that a user interface shows. It gets the
// loaded modules through the loader of the host, here modules:
//
//	flow, err := runtime.Derive(&common.Source{
//		Entry:  SCRIPT,
//		Text:   build,
//		Loader: modules,
//	})
//	if err != nil {
//		return err
//	}
//
// For build.star, with fetch from steps.star, the flow is:
//
//	{
//	  "functions": [
//	    {
//	      "name": "fetch",
//	      "params": ["name"],
//	      "body": "print(\"fetching \" + name)\nreturn name"
//	    }
//	  ],
//	  "text": "fetch(\"lark\")\nprint(\"built\")\nreturn \"ok\""
//	}
//
// If the walk cannot model a function as statements, the function goes into the
// flow as the text of its body. Both functions do so here. Thus a flow always
// holds all that the script does. lark refuses what it cannot carry: modules that
// load each other give ErrCycle, as they do for a compile.
//
// A user interface draws a flow before a run. What a run did is its graph, in
// the section Execution below. A graph shows a function when the run first calls
// it, so a function that nothing called is not in the graph. Draw the flow
// beside the graph to show what did not occur yet. A node has the name of its
// function, so the function that a statement calls matches the statement to its
// node.
//
// When a run fails or is stopped, the graph adds a node with the status
// STATUS_NOT_STARTED for each function that a called function spawns and the run
// never started. Such a node may include a spawn site in a branch that the
// script did not take: the graph cannot tell a spawn the stop prevented from one
// in a branch that would not run. Read the node as "the spawn site is in a
// function that ran before the stop".
//
// A user interface that edits a workflow sends it back as a Flow, the message of
// workflow.proto, written as JSON. The host reads it with Read, which refuses a flow
// that lark cannot run, and writes its script with Emit. Then the host compiles the
// script as it compiles all other scripts:
//
//	flow, err := runtime.Read(sent)
//	if err != nil {
//		return err
//	}
//
//	src, err := runtime.Emit(flow)
//	if err != nil {
//		return err
//	}
//
//	authored, err := runtime.Compile(&common.Source{Entry: "authored.star", Text: src})
//	if err != nil {
//		return err
//	}
//
// For this sent:
//
//	{
//	  "functions": [
//	    {
//	      "name": "fetch",
//	      "params": ["name"],
//	      "body": "print(\"fetching \" + name)\nreturn name"
//	    }
//	  ],
//	  "main": {"statement": [{"call": {"function": "fetch", "args": [{"string": "lark"}]}}]}
//	}
//
// Every value of a flow is an object named for its kind, as {"string": "lark"} is.
// protojson reads a JSON null where a value stands as no value at all, so an
// argument default that is null would read as no default. Read refuses such a null
// (Q439), and the lark command reads a flow with Read. A host that reads a flow with
// protojson instead gets no such refusal and no Check.
//
// Emit writes:
//
//	def fetch(name):
//	    print("fetching " + name)
//	    return name
//
//	def main():
//	    fetch("lark")
//	    pass
//
// Check refuses a flow whose main passes two arguments to fetch, and thus Read and
// Emit also refuse it:
//
//	fetch takes 1 and is passed 2: a call passes arguments the function does not take
//
// Compile refuses the same call in build.star, at the line and the column of the
// call:
//
//	build.star:4:5: call of fetch: a call passes arguments the function does not take
//
// Both errors match ErrArgs. One mistake has one value, in a flow or in a
// script.
//
// # Start
//
// Start returns while the script still runs, whatever the script does next. The
// host goes on with its own work, and collects the run when it wants it. The
// example for Start shows Compile, Start and Wait. If a host needs only the result, it
// calls Eval, which blocks until the run ends and returns the value, the
// graph and the prints. The section Execution below does more with a started
// run: its Status is the graph that a user interface draws.
//
// A cancel of the context that the host gave is the one way for a host to stop
// the run. It stops a script that is busy as easily as a script that waits,
// because the interpreter gets the cancel between instructions. Wait then
// returns ErrCancelled. See the cancel example for Start. The section Cancellation
// below lists the ways that a run, or a part of a run, stops.
//
// Nothing keeps the run for you. There is no store, no id to find a run, and
// nothing that removes runs on a schedule. You decide what to keep of a finished
// run, and for how long. A run gives Status while you hold it, and the garbage
// collector removes it when you release it.
//
// If a host stops its wait for a run, the run continues. Wait blocks until the
// run is over, and takes no context of its own. A caller that must not block
// selects on Done. Neither stops the run. To stop a run, cancel the context that
// you gave to Start.
//
// # Arguments
//
// arg() comes from the args plugin, which is a plugin of the runtime itself. A
// script declares its arguments at module level. Each has a default for a run
// that supplies nothing:
//
//	host = arg("host", "localhost")
//	port = arg("port", 8080)
//
// WithArgs takes a map with Go values. Each entry is the value of the argument
// with that name. A host that has them gives the map:
//
//	args := map[string]any{"host": "build.internal"}
//
//	run, err := runtime.Start(ctx, built, runtime.WithArgs(args))
//	if err != nil {
//		return err
//	}
//
// A host reads JSON from a flag or a request into the map with encoding/json. If
// the JSON is not an object, the read fails. The host asks the decoder to keep
// each number as the text it was written in (UseNumber), because the run reads a
// number by its text, as the rules of protobuf JSON do: 9090 is an int, and 9090.0 and
// 1e3 are floats. A number the decoder reads to a float64 is a float, even when it is
// whole:
//
//	// body is a JSON object from a flag or a request: {"host": "build.internal"}.
//	var args map[string]any
//
//	reading := json.NewDecoder(bytes.NewReader(body))
//
//	reading.UseNumber()
//
//	err := reading.Decode(&args)
//	if err != nil {
//		return fmt.Errorf("bad arguments: %w", err)
//	}
//
//	run, err := runtime.Start(ctx, built, runtime.WithArgs(args))
//	if err != nil {
//		return err
//	}
//
//	value, err := run.Wait()
//	if err != nil {
//		return err
//	}
//
//	log.Printf("main returned %s", value)
//
// The JSON [1, 2] fails at the decoder, with "json: cannot unmarshal array
// into Go value of type map[string]interface {}", before the host makes a run. A
// value that JSON cannot hold, for example a function or a channel, fails the run
// when it starts. The error is "an argument holds a value JSON cannot: proto:
// invalid type: func()", which matches ErrJSON, as does an int outside int64 and a
// string that is not valid UTF-8. A key that no arg()
// declares fails the run before main, with ErrArgs naming the key and the
// nearest declared name. A second WithArgs is refused, and a nil map is no call.
//
// Each run initialises the script again, so each run of one artifact can get
// different values. With {"host": "build.internal", "port": 9090}, the script
// sees build.internal and the int 9090. With nothing, it sees localhost and 8080.
//
// # Logs
//
// WithLog writes each printed line to a writer as a log line: first where the
// line came from, and then what it said. The lark command writes the same log
// line to a terminal, to a pipe, to a file, and with -logs or -hold.
//
//	value, err := runtime.Run(ctx, source, runtime.WithLog(os.Stdout))
//
// The lines for build.star are below. The time is left out as "...".
//
//	time=... level=INFO thread=thread_0 function=fetch msg="fetching lark"
//	time=... level=INFO thread=thread_0 function=main msg=built
//
// WithLog is the one log option. Its writer is any io.Writer, so the host
// chooses where the lines go. Here, a run writes each line when the script
// prints it, to standard output and to a file at the same time:
//
//	file, err := os.Create(LOG_FILE)
//	if err != nil {
//		return err
//	}
//
//	run, err := runtime.Start(ctx, built, runtime.WithLog(io.MultiWriter(os.Stdout, file)))
//	if err != nil {
//		return errors.Join(err, file.Close())
//	}
//
//	_, err = run.Wait()
//
//	// Closed however the run ended.
//	return errors.Join(err, file.Close())
//
// Or the host keeps the lines in a buffer, and reads them when the run is over:
//
//	var out bytes.Buffer
//
//	run, err := runtime.Start(ctx, built, runtime.WithLog(&out))
//	if err != nil {
//		return err
//	}
//
//	_, err = run.Wait()
//	if err != nil {
//		return err
//	}
//
//	log.Print(out.String())
//
// The threads of a run print at the same time, and WithLog writes one line at
// a time, so lines never mix. The function is the one that called print:
// <toplevel> for the top level of a module, and lambda for a lambda. A second
// WithLog is refused, and the error names it.
//
// The host names the file, because the host knows what this run is, and the
// runtime does not. When two runs of one artifact get two writers, they write
// two files. If the writer cannot take a line, lark discards the line and does
// not stop the run. A full disk must not end a workflow that otherwise works.
//
// # Changes
//
// A user interface keeps its own copy of the graph, from {}. It patches the copy
// with each change when the run makes it, so it shows which function each thread
// runs at that time. Here, each change goes to the interface as JSON, through a
// channel of the host, updates:
//
//	watch := func(change *workflowpb.Change) {
//		raw, err := protojson.Marshal(change)
//		if err != nil {
//			log.Printf("change: %v", err)
//
//			return
//		}
//
//		updates <- raw
//	}
//
//	run, err := runtime.Start(ctx, built, runtime.WithChanges(watch))
//	if err != nil {
//		return err
//	}
//
// These are the changes that a run of build.star sends, one line for each
// operation, and a blank line between changes:
//
//	add /status "STATUS_RUNNING"
//
//	add /functions [{"name":"main","status":"STATUS_RUNNING"}]
//	add /functions/0/threads ["thread_0"]
//
//	add /functions/0 {"name":"fetch","status":"STATUS_RUNNING"}
//	remove /functions/1/threads
//	add /functions/0/threads ["thread_0"]
//	add /calls [{"callee":"fetch","caller":"main"}]
//
//	remove /functions/0/threads
//	add /functions/1/threads ["thread_0"]
//	add /functions/0/status "STATUS_SUCCEEDED"
//
//	remove /functions/1/threads
//	add /functions/1/status "STATUS_SUCCEEDED"
//
//	add /status "STATUS_SUCCEEDED"
//
// In order:
//
//  1. The run starts.
//  2. main starts on thread_0.
//  3. fetch starts, and goes before main in the list. thread_0 moves to it, and
//     the call from main is added.
//  4. fetch ends, and thread_0 returns to main.
//  5. main ends.
//  6. The run succeeds.
//
// Applied in this order, the changes give exactly the graph that Status returns.
//
// The function watch runs on the thread whose step made the change, and no other
// step occurs while it runs. Thus a watch that blocks, for example on a full
// channel, stops the run. Changes arrive one at a time, in order.
//
// In watch, Status is exactly the graph after the current change. Thus a host
// can give the whole graph to an interface that joins late, and then patch it
// from the next change. This is also true for the last change: from then, Status
// tells that the run ended, a short time before Done closes.
//
// The changes carry only the graph, and the log goes beside them. A run that
// nobody watches builds no changes, so it pays only for its graph.
//
// # Execution
//
// A started run is a common.Execution. It has three methods:
//
//   - Wait() (starlark.Value, error) blocks until the run is over, and returns
//     what main returned, or why the run failed.
//   - Done() <-chan struct{} closes when the run is over, for a host that
//     selects on it beside its own work.
//   - Status() *workflowpb.Graph is the graph of the run, as it is now (with
//     WithTracking or WithChanges) or as it ended. Before the end of a run with
//     neither option, it is an empty graph whose status is STATUS_RUNNING. It
//     shows each function that the run called, with its status and its threads.
//     It also shows the calls between the functions, and what ended the run if it
//     failed.
//
// Status is what a user interface draws: a call graph, as a profiler draws one.
// It has one node for each function that the run called. A node shows the status
// of its function and the threads that run it. The graph also has an edge for
// each call between two functions. The interface reads it when it asks, during
// the run or after it, when the run has the live graph (WithTracking). Or it
// keeps it current with WithChanges.
//
// A function is one node, however many times the run called it. The node shows
// the status of its newest call. Thus a function that a retry calls until it
// succeeds shows success, and a function whose last call still runs shows that
// it still runs.
//
// A call is one edge from caller to callee, however many times the run made it.
// A spawn is an edge from the function that spawned. A line that calls no
// function is not a node, for example a join, a sleep or a cancel. A library call
// is also not a node.
//
// A common.Artifact has one method, Save() ([]byte, error). It encodes the
// artifact as one bundle that Load can read back. The bundle holds the name of
// the entry unit, and all compiled units, with dependencies first. It refuses an
// artifact that has no main that a run can call, with ErrMain.
//
// This host holds a started run to LIMIT, and reports on it while it goes. The
// cancel example for Start shows the cancel alone:
//
//	ctx, stop := context.WithCancel(ctx)
//	defer stop()
//
//	run, err := runtime.Start(ctx, built, runtime.WithTracking())
//	if err != nil {
//		return err
//	}
//
//	_Supervise(run, stop)
//
//	value, err := run.Wait()
//	if errors.Is(err, runtime.ErrCancelled) {
//		return fmt.Errorf("stopped after %v: %w", LIMIT, err)
//	}
//
//	if err != nil {
//		return err
//	}
//
//	log.Printf("main returned %s", value)
//
// and _Supervise is:
//
//	// _Supervise holds a started run to LIMIT, stopping it with stop, and reports
//	// on it every EVERY until it is over.
//	func _Supervise(run common.Execution, stop context.CancelFunc) {
//		ticker := time.NewTicker(EVERY)
//		defer ticker.Stop()
//
//		expired := time.After(LIMIT)
//
//		for {
//			select {
//			case <-run.Done():
//				return
//			case <-expired:
//				// Cancelling does not wait: Done closes at the unwind.
//				stop()
//			case <-ticker.C:
//				graph := run.Status()
//				log.Printf("%d functions so far", len(graph.GetFunctions()))
//			}
//		}
//	}
//
// These are the statuses of a function and of a run:
//
//   - STATUS_RUNNING: still runs.
//   - STATUS_SUCCEEDED: finished.
//   - STATUS_FAILED: something went wrong, and cause tells what.
//   - STATUS_CANCELLED: stopped. This is not a failure.
//
// Cancelled is not failed, and the difference is important. This runtime stops
// at the first failure, so a failed run stops threads that did nothing wrong.
// Without a separate value, one broken script would show many failures.
//
// When main returns, lark cancels a function on a thread that nothing joined, if
// it still runs. Its status is succeeded or cancelled, and this depends on how
// fast it was. The status of a stopped run is STATUS_CANCELLED, and the node of
// main is also cancelled. A stopped run has no cause, because nothing went wrong
// in it.
//
// The cause of a failed run names the function whose failure ended it, and what
// that failure said. Thus a host goes to that node, and does not search for a
// failed node:
//
//	graph := run.Status()
//
//	log.Printf("%s with %d functions", graph.GetStatus(), len(graph.GetFunctions()))
//
//	cause := graph.GetCause()
//	if cause != nil {
//		log.Printf("ended by %s: %s", cause.GetFunction(), cause.GetFailure())
//	}
//
// For this script:
//
//	def work():
//	    print("working")
//
//	def boom():
//	    fail("boom")
//
//	def main():
//	    first = spawn(work)
//	    second = spawn(boom)
//	    join(first, second)
//
// the host logs:
//
//	STATUS_FAILED with 3 functions
//	ended by boom: build.star:5: fail: boom
//
// main also failed in that run, and that is not a second count of one failure.
// The failure got to main through join, so the evaluation of main failed. The
// cause tells which call caused the failure.
//
// A question costs nothing and changes nothing. Each caller that asks gets an
// answer, as often as it asks. Two interfaces that watch one run both get
// answers. Status does not consume a run, and Wait does not either. A host that
// must keep a record of a run after it releases the run keeps the last Status and
// the logs of that run.
//
// # The graph as JSON
//
// A user interface takes the graph as JSON. Status returns the message of
// workflow.proto, so protojson, from google.golang.org/protobuf/encoding/protojson,
// writes it:
//
//	graph, err := protojson.Marshal(run.Status())
//	if err != nil {
//		return err
//	}
//
// For a run of build.star:
//
//	{
//	  "status": "STATUS_SUCCEEDED",
//	  "functions": [
//	    {"name": "fetch", "status": "STATUS_SUCCEEDED"},
//	    {"name": "main", "status": "STATUS_SUCCEEDED"}
//	  ],
//	  "calls": [
//	    {"caller": "main", "callee": "fetch"}
//	  ]
//	}
//
// Functions are in the order of their names, and calls are in the order of caller
// and then callee. Thus two reads of one run differ only where the run changed.
// While the run continues, a node also lists the threads that run it, as in
// {"name": "fetch", "status": "STATUS_RUNNING", "threads": ["thread_0"]}. When
// the run ends, a node lists no threads.
//
// A change to a graph is a JSON Patch, as RFC 6902 sets it. Thus a user interface
// that holds the graph as JSON can apply a change with any JSON Patch library.
// Change, in workflow.proto, holds the operations. Operation, in patch.proto, is
// one operation:
//
//   - op is spelled as the RFC spells it.
//   - path and from are JSON Pointers.
//   - value is any JSON.
//
// Each field is absent for an op that does not use it. Paths point into the JSON
// that protojson writes. The protojson package does not write a field that has
// its default value. Thus a change sets a field with add, and removes a list that
// becomes empty. The section Changes above shows the changes of a run.
//
// # Handle
//
// A handle is a spawned thread, as a script holds it and a host sees it. A
// common.Handle has four methods:
//
//   - Name() string is the function that the thread runs.
//   - Thread() string is the id of the thread: thread_1, or thread_1_2 for the
//     second thread that thread_1 started.
//   - Done() <-chan struct{} closes when the evaluation of the thread is over,
//     however it ended.
//   - Stop() cancels the thread, and does not wait for it.
//
// A handle is also a Starlark value, so it has the five methods that each value
// has. The interpreter calls them, not a host: String, Type, Freeze, Truth and
// Hash.
//
// The entry point is thread_0, and it is not a handle. The children of a thread
// get numbers from 1 after its own id, in the order that it made them. For
// example, thread_1_1 is the first child of thread_1. Thus a script numbers its
// threads the same on each run.
//
// A script gets a handle from spawn. A host gets a handle as an argument to its
// own builtin. The builtin deadline(worker, ms) stops a thread that still runs ms
// milliseconds later. The plugin _Deadlines gives it to a script that is compiled
// with runtime.Compile(source, runtime.WithPlugins(&_Deadlines{})):
//
//	const (
//		// DEADLINES is what this plugin is called, and DEADLINE the builtin it
//		// gives a script.
//		DEADLINES = "deadlines"
//		DEADLINE  = "deadline"
//	)
//
//	// _Deadlines gives a script deadline(worker, ms). Empty: a deadline belongs to
//	// the call that set it, so the plugin has nothing to hold.
//	type _Deadlines struct{}
//
//	// Name is what a clash calls this plugin.
//	func (d *_Deadlines) Name() string {
//		return DEADLINES
//	}
//
//	// Values is the one name this plugin gives a script.
//	func (d *_Deadlines) Values() starlark.StringDict {
//		return starlark.StringDict{
//			DEADLINE: starlark.NewBuiltin(DEADLINE, _Deadline),
//		}
//	}
//
//	// _Deadline is deadline(worker, ms): it stops worker if it is still running ms
//	// milliseconds from now. It returns at once, so the script goes on meanwhile.
//	func _Deadline(
//		thread *starlark.Thread,
//		fn *starlark.Builtin,
//		args starlark.Tuple,
//		kwargs []starlark.Tuple,
//	) (starlark.Value, error) {
//		var (
//			worker common.Handle
//			limit  int
//		)
//
//		err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &worker, &limit)
//		if err != nil {
//			return nil, err
//		}
//
//		go func() {
//			select {
//			case <-worker.Done():
//				// Finished in time: nothing to stop.
//			case <-time.After(time.Duration(limit) * time.Millisecond):
//				log.Printf("%s on %s is overdue", worker.Name(), worker.Thread())
//
//				worker.Stop()
//			}
//		}()
//
//		return starlark.None, nil
//	}
//
// The interpreter calls those five methods, and a script sees their results:
//
//	def work():
//	    sleep(5000)
//
//	def main():
//	    worker = spawn(work)
//
//	    # The host's builtin: stops worker if it is still running in 100 ms.
//	    deadline(worker, 100)
//
//	    # String: <handle work #thread_1>.
//	    print(worker)
//
//	    # Type: handle
//	    print(type(worker))
//
//	    # Truth: a handle is always true.
//	    if worker:
//	        print("spawned")
//
//	    # Hash: refused, so this fails the run with "handle is unhashable".
//	    index = {worker: 1}
//
// A script that joins the handle sees the effect of the deadline:
//
//	def work():
//	    sleep(5000)
//
//	def main():
//	    worker = spawn(work)
//	    deadline(worker, 100)
//	    join(worker)
//
// After 100 ms, the host logs "work on thread_1 is overdue", and the join fails
// with an error that starts with the position of the join and ends with "work:
// cancelled: context canceled (read a cancelled thread with outcomes())". The
// error matches ErrCancelled.
//
// # A plugin in another process
//
// A plugin does not have to be a package that this binary imports. The host can
// listen, and a plugin in another process can connect and tell what it supplies.
//
// Build the plugin into a directory, and give that directory to lark with
// -plugins:
//
//	go build -o plugins/lark-clock.bin ./cmd/clock
//	lark run -plugins plugins build.star
//
// lark searches the directory for lark-*.bin. The extension tells that a file is
// compiled, and the prefix tells that the file is for lark. Thus the directory
// can hold other binaries, and lark does not start them. lark starts each plugin,
// waits until the plugin tells what it supplies, and keeps the plugin until the
// run ends. A host that embeds the runtime does these steps in its own code. The
// host calls ListenTemp from the package remote. It makes a secret token with
// crypto/rand and a socket in a private temporary directory, and Close removes
// the directory. A host that must choose its own socket path and token calls
// Listen. This program is complete:
//
//	package main
//
//	import (
//		"context"
//		"errors"
//		"log"
//		"os"
//
//		"github.com/thebagchi/lark/v1/runtime"
//		"github.com/thebagchi/lark/v1/runtime/common"
//		"github.com/thebagchi/lark/v1/runtime/plugin/remote"
//	)
//
//	const (
//		// PLUGINS is the directory that holds the plugin binaries, as lark-*.bin.
//		// SCRIPT is the script to run.
//		PLUGINS = "plugins"
//		SCRIPT  = "build.star"
//	)
//
//	func main() {
//		err := _Main(context.Background())
//		if err != nil {
//			log.Fatal(err)
//		}
//	}
//
//	// _Main runs SCRIPT with the plugins in PLUGINS, each in a process of its own.
//	// The plugins answer calls until the run ends, so the listener closes after it.
//	func _Main(ctx context.Context) error {
//		src, err := os.ReadFile(SCRIPT)
//		if err != nil {
//			return err
//		}
//
//		listener, err := remote.ListenTemp()
//		if err != nil {
//			return err
//		}
//
//		err = _Run(ctx, &common.Source{Entry: SCRIPT, Text: src}, listener)
//
//		return errors.Join(err, listener.Close())
//	}
//
//	// _Run runs source with the plugins that listener starts. lark names a plugin
//	// that does not start, and runs without it. This host refuses the run.
//	func _Run(ctx context.Context, source *common.Source, listener common.Listener) error {
//		loading, err := listener.Load(PLUGINS, "")
//		if err != nil {
//			return err
//		}
//
//		if len(loading.Skipped) > 0 {
//			return errors.Join(loading.Skipped...)
//		}
//
//		built, err := runtime.Compile(source, runtime.WithPlugins(listener.Plugins()...))
//		if err != nil {
//			return err
//		}
//
//		run, err := runtime.Start(ctx, built)
//		if err != nil {
//			return err
//		}
//
//		_, err = run.Wait()
//
//		return err
//	}
//
// If a plugin does not start, lark skips it and names it on standard error. The
// run continues without that plugin. The search reads the name of a file, not
// its execute bit. Thus a file whose name matches and that is not a program is a
// usual result. A host that must refuse such a run examines Loading, as _Run
// does. Loaded holds the path of each plugin that started. Skipped holds an error
// for each plugin that did not start.
//
// lark starts the plugins, and the plugins connect back to lark. This is not
// unnecessary work. The host listens, so nothing must connect to a plugin. Thus a
// plugin needs no port, no address that other processes must know, and no way for
// other processes to enter. Also, lark does not parse a handshake line from the
// standard output of a child process. lark decides which code runs, and that is
// why the host must be the parent.
//
// lark makes a new token for each run, and sends it in the environment. lark
// never sends the token in an argument, because the process table shows the
// arguments to all users of the machine. A plugin reads LARK_PLUGIN_SOCKET and
// LARK_PLUGIN_TOKEN. lark sets both when it starts a plugin.
//
// The runtime sees a usual plugin, with Name and Values. Neither method mentions
// a socket. Thus the code that compiles and runs a script did not change for a
// plugin in another process. A script also cannot see the difference: it writes
// clock.now(), as it does for all modules.
//
// cmd/clock is the example. The important part is what it does not do. It
// imports the schema that is generated from proto/plugin.proto, and nothing else:
// no starlark.StringDict, and no part of the runtime. Thus you can write a plugin
// in a language that this repository does not use.
//
// Only data goes between the processes. A function, a thread handle or a module
// has no meaning in another process, so lark refuses to send one. It refuses at
// compile time when the source shows the value clearly, and at run time if not. A
// plugin never gets such a value, so it never must decide what to do with one.
// Numbers go as protobuf numbers, which are float64. An integer comes back as a
// float. lark refuses an integer larger than 2^53, and does not round it
// silently.
//
// Listen returns an error, and that is intentional. A blank import cannot report
// a failed connection. It cannot wait for a registration, and it cannot close a
// socket when the host stops. The plugins of the runtime still register by
// import. All other plugins go to a compile through WithPlugins, and a plugin in
// another process does too.
//
// lark uses only Unix sockets. The names of an attached plugin go into each
// script that is compiled with the plugins of the listener. Such a script can
// also have file, if the host gives it. The file plugin can get to all files that
// the host process can get to. Thus an open port would give the filesystem to
// each process that can connect to the port. There is no TCP option. lark checks
// the token before it accepts a name. Thus a process that only found the socket
// cannot install a name.
//
// When a plugin stops, its names fail, and they stay. A call to one of its names
// fails, and does not wait on a stream that nothing reads. The host continues to
// run. The names also stay, because the run already built its environment. If
// lark removed the names during the run, one failure would become a failure that
// is more difficult to understand.
//
// # Cancellation
//
// A host stops a run in two ways. It cancels the context that it gave to Start,
// Eval or Run: Wait returns ErrCancelled, and the run, and each function that
// still runs, shows STATUS_CANCELLED, with no cause. Or a builtin of the host
// calls Stop on a Handle that a script gave it: a join of that thread raises
// ErrCancelled, and its function shows STATUS_CANCELLED.
//
// A cancelled evaluation stops at its next instruction. It does not interrupt a
// Go function that already runs. Thus a builtin that blocks must take a context
// of its own. Wait does not return until each thread that the run started stops.
// lark cancels a handle that nothing joined, and does not wait for it. Thus a
// spawn that a script did not join cannot keep a call open.
//
// A script, the runtime and the lark command stop a run in other ways. README.md
// lists them all in its section Cancellation.
//
// Nothing limits a run, in memory, in steps or in time. A host that runs scripts
// that it did not write gives a context that it can cancel, runs them in a
// process that it can lose, and may turn on WithMemoryWatch. README.md says what
// the watch does and does not do, in its section What a run can use.
//
// # Errors
//
// This package declares the failures a host can act on. Each is the value of
// the package that raises it, so errors.Is matches it through this package or
// through that one, however many times something wrapped it. The error of a run
// comes from Wait, or from Run. The examples for Run and Start test errors in
// this way. This is the complete list.
//
// Compile, Load and Merge:
//
//   - ErrArgs: a run gets an argument that no arg() declares. Also: a call
//     gives a function arguments that it does not take, too many, too few, one
//     that has no parameter, or one two times. This applies to a script and to
//     a flow.
//   - ErrCycle: modules load each other in a ring. The error names the ring.
//   - ErrMain: the script defines no main, or a main that a run cannot call
//     with no arguments.
//   - ErrConflict: two plugins supply one name. The plugins of the runtime are
//     included.
//   - ErrSource: a call got a nil Source, where a script was wanted.
//   - ErrUnit: Load got a bundle that does not hold its entry, or a module
//     that one of its units loads.
//   - ErrPlugin: Load got a bundle whose script uses a plugin that Load
//     was not given. The text names the plugin.
//   - ErrExport: Merge found a load of a name that a module does not
//     define at its top level.
//   - ErrCapture: Merge found a rename that would put a loaded name in the
//     scope of a local of the same spelling.
//   - ErrUTF8: Save, a method of an Artifact, got a script whose merged
//     source is not valid UTF-8. The script runs, but a bundle cannot carry it.
//
// Start, Eval and Run:
//
//   - ErrArtifact: the artifact is nil, or its type is not one that Compile
//     or Load returns. Nothing ran.
//   - ErrJSON: an argument that WithArgs got holds a value that JSON cannot
//     hold: a function, a channel, a struct. Wait returns it.
//
// Wait, for a script that failed while it ran:
//
//   - ErrAssert: the script asserted a false condition, or asserted with only
//     msg =.
//   - ErrCondition: assert got only a message. That looks like a failure,
//     but it would pass.
//   - ErrFunction: spawn got nothing, a value that is not a function, or a
//     function that takes an argument that spawn cannot supply.
//   - ErrClosure: a value that goes to a new thread captures a local variable.
//   - ErrInit: spawn or timeout, which run a function on a thread of their own,
//     was called while a module's top level runs: on the top level, in a function
//     it calls, or in a module it loads. Call it from main, or from a function
//     main calls. A flow whose init holds either matches ErrInit too, beside
//     ErrForm (Check, Emit) or ErrCarry (Derive).
//   - ErrHandle: join or cancel got a value that is not a handle, or a
//     keyword argument.
//   - ErrSelf: a thread joined its own handle. Such a join would wait for
//     itself for ever, so it fails at once. A join across two threads is not
//     refused.
//   - ErrCancelled: something cancelled a joined handle, or the context of the
//     run stopped the run. The end of the run also stops a sleep before its
//     time, and the error of that sleep matches this one.
//   - ErrDuration: a duration is not a whole number of milliseconds that an
//     int32 can hold.
//   - ErrNested: a script called update, set or join while it held a name. This
//     also applies on each thread that the update started.
//
// Derive, Check and Emit:
//
//   - ErrDuplicate: a flow declares one name two times, or two modules that the
//     script loads declare one name.
//   - ErrFork: a flow joins or cancels a thread where it did not spawn the
//     thread, or spawns two threads under one binding.
//   - ErrUnresolved: a name in a flow resolves to nothing, or a load names no
//     module.
//   - ErrForm: a statement in a flow has no form that lark can write in a
//     script. A function or main with nothing to write, a call with both
//     args and operands, and a name that is not a Starlark identifier or is a
//     keyword, are also ErrForm. So is a value that is none: a Value with no
//     kind set, a null kind that holds anything but null, and a float that is NaN
//     or infinite. So is a text or a body that does not parse as Starlark, and a
//     constant, an argument, an operand or a statement that states nothing. A value
//     nested deeper than common.DEPTH is trail.ErrDeep.
//   - ErrCarry: a flow cannot carry what the script holds: an arg()
//     declaration that is malformed, a module-level value, constants that name
//     each other, a def whose signature has a default, a *args or a **kwargs, a
//     top-level statement of a loaded module, or a load that gives a different
//     name to what it binds.
//   - ErrShadow: a declaration hides a builtin that another file reads.
//
// A plugin keeps its own errors, for the plugins of the runtime and for a
// plugin that a host gives. The error of a run holds them, wrapped as it holds
// all errors. A host names a plugin error through the package of the plugin. The
// errors of core are in the list above, with their names in this package.
//
// # Also exported
//
// This package exports functions and errors only. Its types are in
// v1/runtime/common: Artifact, CompileOption, RunOption, Loader, Handle, Plugin,
// Execution, Result, Print and Source. Flow, Graph and Change are in
// proto/gen/workflow, as are the other messages of the schema, for example
// Operation, and proto/gen/patch.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: a nil or foreign artifact gives ErrNotCompiled (C91w6)
//   - 2026-11-07 23:35: a nil Execution is not safe to use, as for any Go value returned
//     with an error (C91w7, Q233)
//   - 2026-11-07 23:42: the types are in common and workflowpb, not here (C91w9, Q102, Q103)
//   - 2026-11-08 10:46: Export (C73, Q234)
//   - 2026-11-08 13:22: the host guide moved here from README.md (C78, C83b, Q171)
//   - 2026-11-10 11:45: the lark command example uses the run verb (C119, Q305)
//   - 2026-11-10 16:00: ErrSelfJoin in the list of errors (C116, B237, Q327)
//   - 2026-11-10 16:40: ErrForm is also a name that is no identifier (C118, B259, Q300)
//   - 2026-11-10 17:15: one-word names for the sentinels, Eval, Merge and WithTracking in the
//     text; the Logs section has WithLog alone, WithLogger being dropped (C121, Q338-Q342, Q348)
//   - 2026-11-11 22:56: ErrInit in the list of errors; the Handle example spawns in main only
//     (C127, B274, Q383, Q388, Q389)
//   - 2026-11-13 01:27: queue in the list of plugins (C131, Q408)
//   - 2026-11-13 10:21: ErrForm is also a value that is none, and a deep value is ErrDeep
//     (C132, Q2, Q407, Q421, Q437, Q439); the argument example reads numbers by their text
//     (Q437); the argument example reads numbers by their text
//     (Q437)
//   - 2026-11-13 17:48: Read; the guide reads a flow with Read, and says protojson gives no null
//     refusal and no Check; a text or a body that does not parse, and what states nothing, is
//     ErrForm (C133, B295, B296, Q441, Q444, Q445, Q449, Q453)
//   - 2026-11-13 23:55: ErrArgs, formerly ErrArity, in the example and the lists of errors
//     (C136, Q475)
package runtime

import (
	"context"
	"io"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/internal/merge"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
	"github.com/thebagchi/lark/v1/runtime/internal/reader"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/internal/wire"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/args"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/event"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/queue"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/random"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/time"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/utils"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// The failures a host can act on. Each is the subpackage's own value, so
// errors.Is matches whether a host names it through this package or through
// the one that raised it.
//
// The rule for what is here: every sentinel a call through this package can
// return. A flow's ErrArgs and ErrMain are the compiler's own values, so
// each is here once. A plugin a host chooses to import - flow, state, jsonpath
// - keeps its own, because the host already names that package.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: ErrNotCompiled (C91w6, Q118)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-08 22:36: ErrPluginMissing (C78, Q275)
//   - 2026-11-10 16:00: ErrSelfJoin (C116, B237, Q327)
//   - 2026-11-10 17:15: one-word names for the 15 sentinels it names (C121, Q348)
//   - 2026-11-11 22:50: ErrInit (C127, B274, Q383)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
//   - 2026-11-13 23:55: ErrArgs, formerly ErrArity (C136, Q475)
var (
	ErrArgs       = spelling.ErrArgs
	ErrCycle      = spelling.ErrCycle
	ErrMain       = spelling.ErrMain
	ErrUnit       = artifact.ErrUnit
	ErrJSON       = artifact.ErrJSON
	ErrAssert     = core.ErrAssert
	ErrCondition  = core.ErrCondition
	ErrHandle     = core.ErrHandle
	ErrSelf       = core.ErrSelf
	ErrFunction   = core.ErrFunction
	ErrClosure    = core.ErrClosure
	ErrInit       = core.ErrInit
	ErrCancelled  = scheduler.ErrCancelled
	ErrNested     = scheduler.ErrNested
	ErrDuration   = scheduler.ErrDuration
	ErrConflict   = plugin.ErrConflict
	ErrDuplicate  = graph.ErrDuplicate
	ErrFork       = graph.ErrFork
	ErrUnresolved = graph.ErrUnresolved
	ErrForm       = graph.ErrForm
	ErrCarry      = graph.ErrCarry
	ErrShadow     = graph.ErrShadow
	ErrExport     = merge.ErrExport
	ErrCapture    = merge.ErrCapture
	ErrUTF8       = artifact.ErrUTF8
	ErrPlugin     = artifact.ErrPlugin
	ErrArtifact   = artifact.ErrArtifact
	ErrSource     = script.ErrSource
)

// Compile builds source, and every module it loads, into an artifact, without
// running any of it.
//
// Compile merges every load() into the script first, and the artifact is the
// compile of that one merged script: a module that is loaded twice runs once,
// a name two files declare takes a prefix on every side, a load that renames
// gives the global the local name, and globals freeze once the merged top
// level has run and before main is called. See package merge for the rules.
// The merged source travels with the artifact and Save writes it into the
// bundle.
//
// Without options a script is given every plugin of the runtime's own. Its
// modules come from the source's own Modules, or are read as files beside the
// one that loaded them when it names none.
//
// Returns the refusals of the merge (ErrCycle, ErrExport, ErrCapture) and the
// interpreter's own parse and resolve errors, and ErrSource for a nil source.
// A nil option is skipped.
//
// Revisions:
//   - 2026-10-03 00:12: initial creation, replacing NewCompiler, which held a
//     loader the source now names itself
//   - 2026-11-02 23:48: merges every load into the script first (C44a)
//   - 2026-11-04 23:50: returns ErrNoSource for a nil source, and skips a nil
//     option (C86)
//   - 2026-11-07 23:04: returns the interface Artifact (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: uses ErrExport and ErrSource, formerly ErrNoExport and ErrNoSource (C121,
//     Q348)
//   - 2026-11-13 17:40: names package merge, formerly amalgam (C134, Q446)
func Compile(
	source *common.Source,
	opts ...common.CompileOption,
) (common.Artifact, error) {
	built, err := artifact.Compile(
		source,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return built, nil
}

// Merge is source and every module it loads as one script that loads
// nothing, with a loader that refuses every load.
//
// What a bundle carries: Compile of the result is an artifact of one unit. A
// name two files declare takes its module's prefix on every side, a load that
// renames gives the global the local name, and every module runs once, before
// the script's own statements. Globals freeze once the merged top level has run
// and before main is called, so a module may write a value another module owns
// while the top level runs, which the unmerged run refuses.
//
// It takes the options Compile takes, since the names a plugin gives are names
// no module may take.
//
// A collision is renamed, not refused. Returns ErrCycle for modules that load
// each other, ErrExport for a load of a name a module does not define,
// ErrCapture for a rename a local of the loading file would capture, and
// ErrSource for a nil source. A nil option is skipped.
//
// Revisions:
//   - 2026-11-02 23:48: initial creation (C44a)
//   - 2026-11-04 23:50: names ErrNoSource and the skipped nil option (C86 fix round 2)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: renamed from Amalgamate; uses ErrExport and ErrSource, formerly
//     ErrNoExport and ErrNoSource; calls artifact.Merge (C121, Q339, Q348, Q350)
func Merge(
	source *common.Source,
	opts ...common.CompileOption,
) (*common.Source, error) {
	return artifact.Merge(
		source,
		opts...,
	)
}

// Load reads back a bundle Artifact.Save wrote, as an artifact Start runs, so
// a host can compile once and run elsewhere.
//
// It takes the options the compile took, since compiled code names what it
// calls: a bundle compiled with plugins of the host's is loaded with them too.
// The runtime's own are always there, as they are for Compile.
//
// Returns ErrUnit for a bundle that does not hold every module it loads,
// and a wrapped error for bytes that are not a bundle. A nil option is skipped.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
//   - 2026-11-04 23:50: says that a nil option is skipped (C86 fix round 2)
//   - 2026-11-07 23:04: returns the interface Artifact (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func Load(
	bundle []byte,
	opts ...common.CompileOption,
) (common.Artifact, error) {
	loaded, err := artifact.Load(
		bundle,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return loaded, nil
}

// WithPlugins makes a compile give its script the names plugins supply, as
// well as those of every plugin of the runtime's own. It is how a host gives a
// script plugins of its own, file among them, and it takes every one of them
// at once: a second WithPlugins is refused, and the compile returns an error
// that names it. A call with no plugin, or only nil ones, is no call.
//
// A name one of these shares with another plugin is refused when a compile
// builds its environment, naming both. A nil plugin, or an interface holding a nil
// pointer, is skipped.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 13:12: takes the plugins themselves, so a host builds no
//     registry
//   - 2026-10-02 15:56: adds the plugins to every plugin registered by import,
//     rather than replacing them
//   - 2026-10-03 00:12: an option of a compile, there being no Compiler
//   - 2026-10-03 08:27: artifact's own, which now adds to the runtime's plugins too
//   - 2026-11-04 23:50: skips a nil plugin (C86)
//   - 2026-11-04 23:50: skips a typed-nil plugin too (C86)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 10:40: a second WithPlugins is refused, not a replacement (C75, Q235)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
func WithPlugins(plugins ...common.Plugin) common.CompileOption {
	return artifact.WithPlugins(plugins...)
}

// Derive reads source, and every module it loads, into the flow it describes.
//
// The reverse of Emit, so a user interface can show an existing script as a
// flow, edit it, and hand it back. A function the walk cannot model travels as
// its body text, so a flow is never short of what the script does, and what
// cannot travel at all is refused. Its modules come from the source, as they
// do for Compile.
//
// Returns ErrMain for a module, ErrCarry for what a flow cannot carry
// (a constant, a signature, a load it cannot inline, a top-level statement of a
// loaded module), ErrDuplicate for two modules that declare one name, ErrShadow
// for a declaration that hides a builtin another file reads, and ErrCycle for
// modules that load each other.
//
// Revisions:
//   - 2026-10-02 23:51: initial creation
//   - 2026-10-03 00:12: returns the flow alone, refusing a cycle of loads
//     rather than reporting it beside the flow
//   - 2026-11-02 17:53: refuses a statement and a shadowed builtin it left out
//     or let through (C43)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrMain and ErrCarry, formerly ErrNoMain and ErrNotCarried (C121,
//     Q348)
func Derive(source *common.Source) (*workflowpb.Flow, error) {
	return graph.Of(source)
}

// Check refuses a flow that could not become the script it describes: a name
// declared twice, a call passing a function arguments it does not take or both
// kinds of argument, constants naming each other, a thread joined or cancelled
// where it was not spawned, a name that resolves to nothing, or a name that is
// not a Starlark identifier or is a keyword (ErrForm). A value of a flow is
// checked too: a Value with no kind, a null kind that holds anything but null and
// a float that is NaN or infinite are ErrForm. So is a text or a body that does not
// parse as Starlark: the text of main and a function body as a suite, an init text as
// statements. Check does not resolve a name that such a text reads, and it does not
// compile, so a run can still fail on a name nothing declares. A constant with no value
// and no call, an argument with no name, an operand with no source, and a statement
// that lacks what it is written from (a spawn with no call, a count below zero, a match
// with no cases, a call that binds a result where the form has none) are ErrForm too.
//
// Emit runs it before it writes anything, so a host need not; a host that wants
// a flow from a user interface refused as it arrives, before it is written,
// calls it itself.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 11:55: refuses a name that is no identifier (C118, B259, Q300)
//   - 2026-11-13 10:21: refuses a value that is none (C132, Q2, Q407, Q421)
//   - 2026-11-13 19:30: refuses a text or a body that is no Starlark, and a constant, an
//     argument, an operand or a statement that states nothing (C133, B295, B296, Q449, Q453)
func Check(flow *workflowpb.Flow) error {
	return graph.Check(flow)
}

// Read is the flow that the JSON in data holds, once Check accepts it: the decoder
// reads it, a JSON null where a value stands is refused, and then Check refuses
// what it refuses (see Check: a name declared twice, a call with the wrong
// arguments, a text that is no Starlark, and the rest). A host that takes a flow from
// a user interface calls Read, then Emit, and need not call Check.
//
// What the flow promises is what Check promises: every name is declared once, a
// call fits the function it names, every value is a value, every constant, argument,
// operand and statement states what Emit writes it from, every text and body parses,
// and the names a statement uses resolve. So Emit writes the flow that Read returns.
// Read does not compile the flow, so Compile can still refuse the script of a flow that
// Read returned. It does so for a name that a text or a body reads and nothing declares,
// for a break, a continue, a return or a load where Starlark does not allow one, for a
// text that defines main again, and for text nested too deep, among other things.
//
// A JSON null where a value stands is refused, since protojson reads it as no
// value at all: an argument default that is null would read as no default. So is a
// null kind that holds more than null. A flow written before its values were tagged,
// "west" where {"string": "west"} stands now, is refused with a hint that says what a
// value is. Empty or blank input has the decoder's words and no hint.
//
// The error for a flow that does not read is ErrForm, and holds the decoder's own
// error beside it, so errors.Is names both. Its words quote at most 100 bytes of the
// input. The error for a flow that reads and fails Check is Check's error. The
// flow is nil when there is an error.
//
// Read takes the whole of data, whatever its size. A host that takes data from an
// untrusted source bounds len(data) before the call, with a limit that suits it.
// Read holds the decoded flow, and the memory of one flow is several times len(data):
// about 21 times for a flow of a million small values, and 7 for one large string
// (measured, C133). The depth of a flow is bounded by the decoder (about 4,998 nested
// lists).
//
// Revisions:
//   - 2026-11-13 17:38: initial creation (C133, Q4, Q433, Q439, Q441, Q442, Q444, Q445)
func Read(data []byte) (*workflowpb.Flow, error) {
	return reader.Read(data)
}

// Emit is the Starlark script flow describes, ready for Compile, once Check
// accepts the flow.
//
// It checks first, so a flow a user interface sent becomes a script in one
// call, and is refused with what Check returns when it could not become one.
// It does not compile what it writes, so a name that a body reads and nothing
// declares is a compile error with a line number. A statement it has no form for
// is ErrForm, and so is a function with no body or a body that does not parse.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-10-03 16:29: checks the flow before writing it
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-13 19:30: a body that does not parse is refused by Check (C133, B295, Q449)
func Emit(flow *workflowpb.Flow) ([]byte, error) {
	err := graph.Check(flow)
	if err != nil {
		return nil, err
	}

	return graph.Emit(flow)
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself: something to wait for and to ask about. A host
// that would only wait calls Eval instead, which performs the run the same
// way, so every run option means the same on every run.
//
// Cancelling ctx is the one way to stop the run, and Wait then returns
// ErrCancelled.
//
// Nothing holds the run on the caller's behalf. What is worth keeping about a
// finished run, and for how long, is the host's decision - so the run is the
// caller's to hold and to let go of.
//
// Status is the run's graph, which is what a user interface draws. What the
// run prints goes to the logger its options name.
//
// The error is returned when Start cannot begin the run: ErrArtifact for an
// artifact lark did not compile, a nil one included. The run is then nil and
// must not be used.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
//   - 2026-09-20 19:55: takes options, so a graph can be supplied without
//     leaving the facade
//   - 2026-09-23 23:20: returns the run rather than an id into a store, which
//     no longer exists
//   - 2026-10-02 13:12: takes no options, what a run prints being its
//     context's transcript
//   - 2026-10-02 16:08: takes the run's options, its script's arguments among
//     them
//   - 2026-10-03 00:21: the one way to run, an artifact having no Run or Invoke of its
//     own, and stopped by its context alone
//   - 2026-11-07 17:40: also returns an error, always nil for now (C91 wave 0)
//   - 2026-11-07 23:04: takes Artifact, and returns ErrNotCompiled with a nil run for a nil or
//     foreign artifact (C91w6, Q118, Q125)
//   - 2026-11-07 23:23: returns common.Execution, nil with an error and not to be
//     used then (C91w7, Q101, Q233)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: uses ErrArtifact and Eval, formerly ErrNotCompiled and Evaluate (C121,
//     Q338, Q348)
func Start(
	ctx context.Context,
	art common.Artifact,
	opts ...common.RunOption,
) (common.Execution, error) {
	return observe.Start(
		ctx,
		art,
		opts...,
	)
}

// Eval evaluates art's entry point on the caller's goroutine and returns
// once the run is over: the value main returned, the run's graph as it ended,
// and each line its script printed, with the thread and the function that
// printed it.
//
// It starts no goroutine for the run itself, which a host that would only
// Start and Wait pays for in a goroutine started and a waiter woken. The
// script's own threads still run on goroutines of their own. Every run option
// means here what it means to Start, and a print still goes to the logger the
// options name.
//
// Cancelling ctx stops the run, and the error is then ErrCancelled. The result
// is never nil: for a run that failed its Value is nil, its graph and its
// prints are the run's up to its end, and the error is the one Wait gives. An
// artifact lark did not compile, a nil one included, gives ErrArtifact and
// an empty result.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 23:04: takes Artifact, and returns ErrNotCompiled for a nil or foreign artifact
//     (C91w6, Q118)
//   - 2026-11-07 23:23: returns common.Result (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: renamed from Evaluate; uses ErrArtifact, formerly ErrNotCompiled; calls
//     observe.Eval (C121, Q338, Q348, Q350)
func Eval(
	ctx context.Context,
	art common.Artifact,
	opts ...common.RunOption,
) (*common.Result, error) {
	return observe.Eval(
		ctx,
		art,
		opts...,
	)
}

// Run compiles source with the runtime's own plugins, runs its main, and
// returns what main returned once the run is over: Compile and Eval in one
// call, for a host that wants the result and nothing more.
//
// Every run option means here what it means to Eval, which Run calls. A
// host handing a script plugins of its own compiles it with Compile and runs it
// with Eval or Start, since what a compile is given and what a run is
// given are not one list; so does a host that reads the graph or the prints.
//
// Returns what Compile refuses a script with, ErrSource for a nil source, and
// otherwise what Eval returns. A nil option is skipped.
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
//   - 2026-10-28 23:50: runs on the caller's goroutine through Evaluate,
//     rather than through Start and Wait
//   - 2026-11-04 23:50: names the nil source and the nil option it survives
//     (C86)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrSource and Eval, formerly ErrNoSource and Evaluate (C121, Q338,
//     Q348)
func Run(
	ctx context.Context,
	source *common.Source,
	opts ...common.RunOption,
) (starlark.Value, error) {
	art, err := Compile(source)
	if err != nil {
		return nil, err
	}

	result, err := Eval(
		ctx,
		art,
		opts...,
	)

	return result.Value, err
}

// Export is a Starlark value as the protobuf value google.protobuf.Value, so a
// host reads what a run returned without a type switch on go.starlark.net.
//
// It is the conversion the remote plugin host uses for the arguments it sends,
// so one rule serves both (Q9). The mapping:
//
//	Starlark                  protobuf
//	None                      null
//	bool                      bool
//	int                       number, when -2^53 <= n <= 2^53
//	float                     number
//	string                    string
//	bytes                     string, the bytes unchanged
//	list, tuple, set          list, in the order the value holds
//	dict with string keys     struct
//
// An int is a float64 on the wire, which holds integers exactly only up to 2^53.
// An int beyond that, 2^53+1 and everything past it, and the same on the
// negative side, is refused rather than rounded (Q260). A tuple and a set arrive
// as a list, and bytes as a string, so the result does not say which they were.
//
// A function, a thread handle, a module and a dict keyed by anything but a
// string are refused: the error matches unpack.ErrConvert under
// errors.Is (remote.ErrValue was merged into it, Q251). A container that holds
// itself returns trail.ErrCyclic, nesting deeper than common.DEPTH
// returns trail.ErrDeep, and a nil v returns unpack.ErrConvert.
//
// Two values export that a protobuf encoder then refuses. Bytes that are not
// valid UTF-8 export as a string, and proto.Marshal fails on that string. NaN
// and the infinities export as numbers, and protojson fails on them. Export does
// not check either, so a host that encodes the result checks first.
//
// The walk looks at ctx every common.POLL elements, so cancelling ctx stops the
// conversion of a large value; the error is then the context's own.
//
// Check: errors.Is(err, unpack.ErrConvert) for a refused value, and
// v.AsInterface() on the result for a Go value (nil, bool, float64, string,
// []any, map[string]any).
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73, Q234, Q257, Q258, Q259, Q260)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert and ErrCyclic and ErrDeep, formerly ErrNotProtoValue and
//     ErrSelfContaining and ErrTooDeep (C121, Q349)
func Export(
	ctx context.Context,
	v starlark.Value,
) (*structpb.Value, error) {
	return wire.Value(
		trail.New(ctx),
		v,
	)
}

// WithArgs supplies a run's script with its arguments, each field of args the
// value of the argument the script declares under that name.
//
// A script declares one at module level - host = arg("host", "localhost") -
// and every run of one artifact may be handed different values, because a run
// initialises the script itself. The arguments are the script's, not the
// runtime's, so a run is handed them rather than finding them on its context.
//
// A map of Go values, so a host imports nothing to supply them, and JSON it
// holds is one json.Decoder with UseNumber away. Each must be a value a lark
// Value can hold, by the strict rules of protobuf JSON: an integer type, or a
// json.Number written as an integer, is an exact int that fits int64; a float
// type, or a json.Number with a fraction or an exponent, is a float; a string is
// valid UTF-8 text and is never read as base64; a []byte is bytes. One that is not
// fails the run when it starts, with ErrJSON. A second WithArgs is refused, and the
// run returns an error that names it. A nil map is not given: it is no call.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-02 13:12: reads the JSON object itself, absorbing Parsed
//   - 2026-10-02 16:08: an option of a run, rather than a context carrying the
//     arguments
//   - 2026-10-02 16:18: takes a Struct, a JSON object by its type, rather than
//     text to parse
//   - 2026-10-03 16:30: takes a map of Go values, the Struct built inside
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 11:25: a second WithArgs is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-10 17:15: uses ErrJSON, formerly ErrNotJSON (C121, Q348)
//   - 2026-11-13 13:03: the arguments are read by the strict rules of protobuf JSON (C132, Q437,
//     Q438)
func WithArgs(args map[string]any) common.RunOption {
	return artifact.WithArgs(args)
}

// WithLog writes what a run's script prints to w, one line a print: the time,
// the level, the thread and the function that printed it, and then the line,
// as the lark command prints them. It is the one log option: a host that wants
// records reads the lines it wrote. A second WithLog is refused, with an error
// that names it. A nil writer is no call.
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 10:40: a run given WithLogger too is refused (C75, Q235)
//   - 2026-11-08 11:25: a second WithLog is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-10 17:15: the one log option, since WithLogger is dropped (C121, Q340)
func WithLog(w io.Writer) common.RunOption {
	return artifact.WithLog(w)
}

// WithChanges sends a started run's graph, as it changes, to watch: an option
// of Start. Each Change is the JSON Patch that takes a copy of the graph from
// before one step to after it, and a copy starts as {}, so a user interface
// patching its copy shows which function each thread is executing as it
// happens. watch is called one change at a time, in order, on the goroutine of
// the thread whose step it was; Status still answers for the whole graph. A
// second WithChanges is refused, and a nil watch is no call.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 11:25: a second WithChanges is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
func WithChanges(watch func(change *workflowpb.Change)) common.RunOption {
	return artifact.WithChanges(watch)
}

// WithTracking keeps a started run's graph as the run goes, so Status answers
// at any moment with the functions called so far. Without it, and without
// WithChanges, Status answers an empty running graph until the run ends, and
// the graph of the whole run after it, the same graph either way. The live
// graph costs a lock every thread of the run waits for at each call.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 17:15: renamed from WithLiveGraph; calls artifact.WithTracking (C121, Q341,
//     Q350)
func WithTracking() common.RunOption {
	return artifact.WithTracking()
}

// WithMemoryWatch turns on the memory watch for a run; it is off by default. It
// reads the memory in use on the machine and in this process's cgroup, and at 95
// percent of the tighter it ends the run the way assert does, with an error that
// wraps memwatch.ErrLimit ("memory limit reached"). A system without
// /proc/meminfo or cgroup v2 refuses the run with memwatch.ErrUnavailable.
// abandon, when not nil, is called if the run has not ended 500 ms after the
// trip; nil leaves the process alone. A second abandon is refused, and a nil
// abandon is no call.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 11:25: a second WithMemoryWatch is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func WithMemoryWatch(abandon func(error)) common.RunOption {
	return artifact.WithMemoryWatch(abandon)
}
