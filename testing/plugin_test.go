// Regression probes for a plugin that really is another process: that a script
// calls it without knowing, and that killing it fails its names rather than
// this one.
//
// Here rather than in runtime/plugin/remote because these build a binary and
// start it. What a separate process adds over a client in this one is dying
// without saying so, and nothing short of a real process can be killed.
package testing_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/plugin/remote"
)

const (
	// PLUGIN_TOKEN is what the host issues and the plugin must present.
	PLUGIN_TOKEN = "a-token-only-these-two-know"

	// _ATTACHED is how long to wait for the plugin to announce itself, and
	// _NOTICED how long for the host to notice it gone. Both enormous next to
	// the milliseconds they take.
	_ATTACHED = 20 * time.Second
	_NOTICED  = 20 * time.Second

	// PROC is where this system publishes what every process is running, and
	// CMDLINE the arguments it was given. Read to prove a secret is not among
	// them.
	PROC    = "/proc"
	CMDLINE = "cmdline"
)

// _Clock builds the clock plugin and answers with the path to it.
//
// Built rather than assumed, because a stale binary would prove the wrong
// thing and bin/ is gitignored.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
func _Clock(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "clock")
	build := exec.Command("go", "build", "-o", binary, "./cmd/clock")
	build.Dir = _ModuleRoot(t)

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("building clock: %v\n%s", err, out)
	}

	return binary
}

// _Host is a listener with a registry holding everything the default one does.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
func _Host(t *testing.T) (*remote.Listener, string) {
	t.Helper()

	// A short path: a unix socket has a low length limit, and a test name is
	// not short.
	dir, err := os.MkdirTemp("", "lk")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	socket := filepath.Join(dir, "s")
	registry := plugin.New()

	for _, held := range plugin.DEFAULT.Registered() {
		registry.Register(held)
	}

	listener, err := remote.Listen(socket, PLUGIN_TOKEN, registry)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = listener.Close()
	})

	return listener, socket
}

// _Attached starts the plugin and waits for it to be answering.
//
// Waits on Live rather than on Names, because a restart does not add a name:
// the adapter outlives the process, so the names were already there and only
// the connection is new. Counting names was how this first read as a
// replacement never arriving when it had.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
//   - 2026-09-26 01:56: waits on Live, so a restart is seen
func _Attached(t *testing.T, listener *remote.Listener, binary string, socket string) *exec.Cmd {
	t.Helper()

	held := exec.Command(binary)
	held.Stderr = os.Stderr

	// Where to dial and what to present, in the environment rather than in
	// arguments: an argument is in the process table, and the token is the one
	// thing between a local process and putting names into every script.
	held.Env = append(os.Environ(),
		remote.SOCKET_ENV+"="+socket,
		remote.TOKEN_ENV+"="+PLUGIN_TOKEN,
	)

	err := held.Start()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = held.Process.Kill()
		_ = held.Wait()
	})

	deadline := time.Now().Add(_ATTACHED)

	for time.Now().Before(deadline) {
		if len(listener.Live()) > 0 {
			return held
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("the plugin never announced itself")

	return nil
}

// TestPlugin_AnotherProcessSuppliesNames is the whole idea from outside: a
// script calls a name, and a different program answers.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
func TestPlugin_AnotherProcessSuppliesNames(t *testing.T) {
	listener, socket := _Host(t)
	_Attached(t, listener, _Clock(t), socket)

	built, err := runtime.NewCompiler(runtime.WithPlugins(listener.Registry())).
		Compile("remote.star", []byte("def main():\n    return clock.add(2, 3)\n"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := built.Run(t.Context())
	if err != nil {
		t.Fatalf("calling another process: %v", err)
	}

	if got.String() != "5.0" {
		t.Fatalf("got %s, want 5.0", got)
	}
}

// TestPlugin_KillingItFailsItsNamesAndNotTheHost is what a client inside this
// process cannot prove, and what happens after.
//
// The plugin is killed outright, so it says no goodbye and closes nothing. The
// call has to fail rather than wait on a stream nobody is reading, and this
// process has to still be here to notice - a plugin taking its host down with
// it would make every plugin a liability.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
//   - 2026-09-26 01:52: a replacement works, where it used to be refused as a
//     conflict with the plugin that had died
func TestPlugin_KillingItFailsItsNamesAndNotTheHost(t *testing.T) {
	listener, socket := _Host(t)
	held := _Attached(t, listener, _Clock(t), socket)

	compiler := runtime.NewCompiler(runtime.WithPlugins(listener.Registry()))
	src := []byte("def main():\n    return clock.add(1, 1)\n")

	built, err := compiler.Compile("remote.star", src)
	if err != nil {
		t.Fatal(err)
	}

	got, err := built.Run(t.Context())
	if err != nil {
		t.Fatalf("before the kill: %v", err)
	}

	if got.String() != "2.0" {
		t.Fatalf("before the kill, got %s", got)
	}

	err = held.Process.Kill()
	if err != nil {
		t.Fatal(err)
	}

	_ = held.Wait()

	deadline := time.Now().Add(_NOTICED)

	for time.Now().Before(deadline) {
		_, err = built.Run(t.Context())
		if err != nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !errors.Is(err, remote.ErrGone) {
		t.Fatalf("after the kill: %v, want ErrGone", err)
	}

	// The names are still there. A run has already built its environment, so
	// making them vanish partway would turn one failure into a stranger one:
	// an undefined name where the script plainly reads clock.add.
	if len(listener.Names()) == 0 {
		t.Fatal("the names went away with the process")
	}

	// And a replacement works. This is what the adapter outliving its
	// connection is for: the registry has no removal, so a second adapter for
	// the same name would have clashed with the dead one for as long as the
	// host lived, and a plugin could be started exactly once.
	_Attached(t, listener, _Clock(t), socket)

	built, err = compiler.Compile("again.star", src)
	if err != nil {
		t.Fatalf("compiling after a restart: %v", err)
	}

	got, err = built.Run(t.Context())
	if err != nil {
		t.Fatalf("calling a restarted plugin: %v", err)
	}

	if got.String() != "2.0" {
		t.Fatalf("after the restart, got %s", got)
	}
}

// TestLoad_ADirectoryOfPluginsIsStartedAndKeptReady is the whole feature as a
// host uses it: point at a directory, and what is in it becomes names.
//
// The directory also holds a file that matches the glob and is not a program,
// because a glob reads a name rather than the execute bit - go-plugin's own
// Discover says as much about its own. One plugin loads, one is skipped, and
// the host runs.
//
// Revisions:
//   - 2026-09-26 00:22: initial creation
func TestLoad_ADirectoryOfPluginsIsStartedAndKeptReady(t *testing.T) {
	listener, _ := _Host(t)

	dir := t.TempDir()

	build := exec.Command("go", "build", "-o", filepath.Join(dir, "lark-clock.bin"), "./cmd/clock")
	build.Dir = _ModuleRoot(t)

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("building the plugin: %v\n%s", err, out)
	}

	// Matches lark-*.bin, is not a program.
	err = os.WriteFile(filepath.Join(dir, "lark-broken.bin"), []byte("not a program"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// And one that is a program but is not ours to start.
	err = os.WriteFile(filepath.Join(dir, "other.bin"), []byte("not ours"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	loading, err := listener.Load(dir, "")
	if err != nil {
		t.Fatalf("loading %s: %v", dir, err)
	}

	if len(loading.Loaded) != 1 {
		t.Fatalf("loaded %v, want one", loading.Loaded)
	}

	if len(loading.Skipped) != 1 {
		t.Fatalf("skipped %v, want one", loading.Skipped)
	}

	t.Logf("skipped: %v", loading.Skipped[0])

	// The names it announced work, which is what loading was for.
	built, err := runtime.NewCompiler(runtime.WithPlugins(listener.Registry())).
		Compile("loaded.star", []byte("def main():\n    return clock.add(2, 3)\n"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := built.Run(t.Context())
	if err != nil {
		t.Fatalf("calling a loaded plugin: %v", err)
	}

	if got.String() != "5.0" {
		t.Fatalf("got %s, want 5.0", got)
	}
}

// TestLark_PluginsFlag is the feature from a command line, which is the only
// place most people will meet it.
//
// Built and executed rather than called, because the exit code is part of what
// is being checked and go run reports its own.
//
// Revisions:
//   - 2026-09-26 00:48: initial creation
func TestLark_PluginsFlag(t *testing.T) {
	root := _ModuleRoot(t)
	dir := t.TempDir()

	lark := filepath.Join(dir, "lark")
	plugins := filepath.Join(dir, "plugins")

	err := os.Mkdir(plugins, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct{ out, from string }{
		{lark, "./cmd/lark"},
		{filepath.Join(plugins, "lark-clock.bin"), "./cmd/clock"},
	} {
		build := exec.Command("go", "build", "-o", item.out, item.from)
		build.Dir = root

		out, err := build.CombinedOutput()
		if err != nil {
			t.Fatalf("building %s: %v\n%s", item.from, err, out)
		}
	}

	script := filepath.Join(dir, "run.star")

	err = os.WriteFile(script, []byte("def main():\n    print(clock.add(2, 3))\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Without the flag the name is not there, and the failure says so rather
	// than reading as a plugin that went away.
	code, out := _LarkCode(t, lark, "-s", script)
	if code == 0 {
		t.Fatalf("no -p: exit 0, want a failure\n%s", out)
	}

	if !strings.Contains(string(out), "undefined: clock") {
		t.Fatalf("no -p: %s, want an undefined name", out)
	}

	// With it, the plugin is started, answers, and the run succeeds.
	code, out = _LarkCode(t, lark, "-s", script, "-p", plugins)
	if code != 0 {
		t.Fatalf("-p: exit %d\n%s", code, out)
	}

	if !strings.Contains(string(out), "5.0") {
		t.Fatalf("-p: %s, want 5.0 from the plugin", out)
	}

	// A directory with nothing in it is not a failure, and leaves the name
	// undefined rather than present and broken.
	empty := filepath.Join(dir, "none")

	err = os.Mkdir(empty, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	code, out = _LarkCode(t, lark, "-s", script, "-p", empty)
	if code == 0 || !strings.Contains(string(out), "undefined: clock") {
		t.Fatalf("-p on an empty directory: exit %d\n%s", code, out)
	}
}

// TestPlugin_SocketIsPrivateAndGoesAway is the socket as a file: who may reach
// it, and that it does not outlast the listener.
//
// Mode 0600 is the whole of the access control. A registered plugin's names go
// into every script compiled against that registry and file reaches whatever
// this process reaches, so the socket being readable by another user would put
// the filesystem behind them.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func TestPlugin_SocketIsPrivateAndGoesAway(t *testing.T) {
	dir, err := os.MkdirTemp("", "lk")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	socket := filepath.Join(dir, "s")

	listener, err := remote.Listen(socket, PLUGIN_TOKEN, plugin.New())
	if err != nil {
		t.Fatal(err)
	}

	about, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}

	if about.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %o, want 0600", about.Mode().Perm())
	}

	err = listener.Close()
	if err != nil {
		t.Fatalf("closing: %v", err)
	}

	_, err = os.Stat(socket)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the socket is still there after close: %v", err)
	}
}

// TestPlugin_TheTokenIsNowhereAnyoneCanRead is the security claim tested
// rather than trusted.
//
// The token is the only thing between a local process and putting names into
// every script compiled against that registry. It is passed in the environment
// for that reason, so it must appear in no process's argument list - which
// /proc publishes to every user on the machine.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func TestPlugin_TheTokenIsNowhereAnyoneCanRead(t *testing.T) {
	listener, socket := _Host(t)
	binary := _Clock(t)

	_Attached(t, listener, binary, socket)

	// The search has to be able to find something before its not finding the
	// token means anything. The plugin is running and was started by path, so
	// it is there to be found.
	if !_ArgsContain(t, binary) {
		t.Fatal("the search cannot see a process it was told is running")
	}

	if _ArgsContain(t, PLUGIN_TOKEN) {
		t.Fatal("the token is in a process argument list, where anyone can read it")
	}
}

// TestPlugin_NothingOutlivesTheHost is the shutdown, checked by pid rather than
// by assumption.
//
// Closing stops the server, which closes every stream and is how a plugin
// learns its host has gone. Only what is still running after that is killed. A
// plugin left behind would be a process nobody is waiting for, holding whatever
// it had open.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func TestPlugin_NothingOutlivesTheHost(t *testing.T) {
	listener, _ := _Host(t)

	dir := t.TempDir()
	named := filepath.Join(dir, "lark-clock.bin")

	build := exec.Command("go", "build", "-o", named, "./cmd/clock")
	build.Dir = _ModuleRoot(t)

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("building the plugin: %v\n%s", err, out)
	}

	loading, err := listener.Load(dir, remote.GLOB)
	if err != nil {
		t.Fatal(err)
	}

	if len(loading.Loaded) != 1 {
		t.Fatalf("loaded %v, want one", loading.Loaded)
	}

	if !_ArgsContain(t, named) {
		t.Fatal("the plugin was reported loaded and is not running")
	}

	err = listener.Close()
	if err != nil {
		t.Fatalf("closing: %v", err)
	}

	// Killed if it did not go on its own, so this is bounded by LEAVING plus
	// the moment the kill takes.
	deadline := time.Now().Add(_NOTICED)

	for time.Now().Before(deadline) {
		if !_ArgsContain(t, named) {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("the plugin was still running after its host closed")
}

// TestPlugin_SeveralThreadsCallOneClock is why an ask carries an id.
//
// Eight threads call one plugin through one stream, so their asks and answers
// interleave. Each caller has to be handed its own answer.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func TestPlugin_SeveralThreadsCallOneClock(t *testing.T) {
	listener, socket := _Host(t)
	_Attached(t, listener, _Clock(t), socket)

	built, err := runtime.NewCompiler(runtime.WithPlugins(listener.Registry())).
		Compile("many.star", []byte(`
def once():
    return clock.add(1, 2)

def main():
    held = [spawn(once) for i in range(8)]

    return join(*held)
`))
	if err != nil {
		t.Fatal(err)
	}

	got, err := built.Run(t.Context())
	if err != nil {
		t.Fatalf("eight threads through one plugin: %v", err)
	}

	if got.String() != "[3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0, 3.0]" {
		t.Fatalf("got %s", got)
	}
}

// TestPlugin_AnExecutableThatIsNotAProgramIsSkipped is the other half of the
// skip: a file that matches and has the execute bit and still is not a program.
//
// TestLoad_ADirectoryOfPluginsIsStartedAndKeptReady covers the one without the
// bit, which fails at fork. This one gets further - the exec itself is refused
// by the kernel for want of a header - so it is a second path to the same
// answer.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func TestPlugin_AnExecutableThatIsNotAProgramIsSkipped(t *testing.T) {
	listener, _ := _Host(t)

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "lark-quiet.bin"), []byte("not a program"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	loading, err := listener.Load(dir, remote.GLOB)
	if err != nil {
		t.Fatal(err)
	}

	if len(loading.Loaded) != 0 {
		t.Fatalf("loaded %v, want none", loading.Loaded)
	}

	if len(loading.Skipped) != 1 {
		t.Fatalf("skipped %v, want one", loading.Skipped)
	}

	t.Logf("skipped: %v", loading.Skipped[0])
}

// _ArgsContain reports whether any process on this machine has needle in its
// arguments.
//
// Reads /proc, which is how the arguments of every process are published to
// every user - the reason a secret must not be one.
//
// Revisions:
//   - 2026-09-26 01:15: initial creation, from QA's probes
func _ArgsContain(t *testing.T, needle string) bool {
	t.Helper()

	held, err := os.ReadDir(PROC)
	if err != nil {
		t.Skipf("no process table to read on this system: %v", err)
	}

	for _, entry := range held {
		if !entry.IsDir() {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(PROC, entry.Name(), CMDLINE))
		if err != nil {
			// A process that ended between the listing and the read.
			continue
		}

		if strings.Contains(string(raw), needle) {
			return true
		}
	}

	return false
}
