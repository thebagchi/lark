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

// _Attached starts the plugin and waits for its names to arrive.
//
// Waits for the count to grow rather than to be more than zero. A listener that
// has already carried a plugin still reports its names after it died, so "more
// than none" is true before the new one has said anything - which is how this
// first read as the host having stopped serving when it had not.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
func _Attached(t *testing.T, listener *remote.Listener, binary string, socket string) *exec.Cmd {
	t.Helper()

	before := len(listener.Names())

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
		if len(listener.Names()) > before {
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
// process cannot prove.
//
// The plugin is killed outright, so it says no goodbye and closes nothing. The
// call has to fail rather than wait on a stream nobody is reading, and this
// process has to still be here to notice - a plugin taking its host down with
// it would make every plugin a liability.
//
// Revisions:
//   - 2026-09-25 07:02: initial creation
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

	// And the host is still serving: another plugin can attach and work.
	_Attached(t, listener, _Clock(t), socket)

	// The second plugin supplies the same names as the first, which the
	// registry refuses - the conflict is the proof that it attached.
	_, err = compiler.Compile("again.star", src)
	if !errors.Is(err, plugin.ErrConflict) {
		t.Fatalf("a second plugin after the kill: %v, want ErrConflict", err)
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
