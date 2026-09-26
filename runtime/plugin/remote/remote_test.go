// Proofs for a plugin that lives across a socket: that a script cannot tell,
// that a conflict is still the registry's to refuse, and that nothing which
// cannot cross ever does.
//
// The plugin here dials in from this process rather than from another one. The
// socket, the stream and the marshalling are the real ones; what a separate
// process adds is dying without saying so, which testing/ proves with a binary.
package remote_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/plugin"
	_ "github.com/thebagchi/lark/runtime/plugin/core"
	"github.com/thebagchi/lark/runtime/plugin/flow"
	"github.com/thebagchi/lark/runtime/plugin/remote"
)

const (
	// TOKEN is what the host issues and a plugin must present.
	TOKEN = "a-token-only-these-two-know"

	// SCRIPT is what a failure calls the script it was given.
	SCRIPT = "remote.star"

	// CLOCK is the plugin the tests attach, and NOW and ADD what it supplies.
	CLOCK = "clock"
	NOW   = "clock.now"
	ADD   = "clock.add"

	// TICK is what now answers with.
	TICK = "tick"

	// _SETTLED is how long to wait for a plugin to be installed, or for one
	// that was refused to have failed. Enormous next to the milliseconds
	// either takes.
	_SETTLED = 10 * time.Second

	// _STALLED is how long a one-second timeout is given to return before the
	// call is taken not to be watching its caller at all. Generous, because
	// what is being told apart is "a moment late" from "never".
	_STALLED = 15 * time.Second
)

// _Listening is a listener with a registry of its own, holding everything the
// default one holds.
//
// Its own registry rather than DEFAULT, because a plugin that comes and goes
// has no business in the one every compiler in the process shares.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func _Listening(t *testing.T) (*remote.Listener, string) {
	t.Helper()

	// A short path, because a unix socket has a low length limit and a test
	// name is not short.
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

	listener, err := remote.Listen(socket, TOKEN, registry)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = listener.Close()
	})

	return listener, socket
}

// _Answering is a plugin dialling in from this process, announcing named and
// answering with answer until the test ends.
//
// Returns a function that closes the stream, which is how a plugin leaves.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func _Answering(
	t *testing.T,
	socket string,
	token string,
	called string,
	named []string,
	answer func(*pluginpb.Ask) *pluginpb.Answer,
) func() {
	t.Helper()

	held, err := grpc.NewClient(
		"unix:"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var dialer net.Dialer

			return dialer.DialContext(ctx, "unix", socket)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := pluginpb.NewServiceClient(held).Register(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = stream.Send(&pluginpb.Request{
		Of: &pluginpb.Request_Register{
			Register: &pluginpb.Register{Token: token, Name: called, Names: named},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			from, err := stream.Recv()
			if err != nil {
				return
			}

			ask := from.GetAsk()
			if ask == nil {
				continue
			}

			err = stream.Send(&pluginpb.Request{
				Of: &pluginpb.Request_Answer{Answer: answer(ask)},
			})
			if err != nil {
				return
			}
		}
	}()

	leave := func() {
		_ = stream.CloseSend()
		_ = held.Close()
	}

	t.Cleanup(leave)

	return leave
}

// _Clock answers the two names the tests use.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func _Clock(ask *pluginpb.Ask) *pluginpb.Answer {
	switch ask.GetName() {
	case NOW:
		return &pluginpb.Answer{Id: ask.GetId(), Result: structpb.NewStringValue(TICK)}

	case ADD:
		total := float64(0)

		for _, given := range ask.GetArgs() {
			total += given.GetNumberValue()
		}

		return &pluginpb.Answer{Id: ask.GetId(), Result: structpb.NewNumberValue(total)}
	}

	return &pluginpb.Answer{Id: ask.GetId(), Failed: "no such name: " + ask.GetName()}
}

// _Installed waits until the listener is carrying count names.
//
// A registration crosses a socket, so it has not happened yet when the call
// that started it returns.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func _Installed(t *testing.T, listener *remote.Listener, count int) {
	t.Helper()

	deadline := time.Now().Add(_SETTLED)

	for time.Now().Before(deadline) {
		if len(listener.Names()) >= count {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("only %d names were installed, want %d", len(listener.Names()), count)
}

// _Ran compiles and runs src against the listener's registry.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func _Ran(t *testing.T, listener *remote.Listener, src string) (string, error) {
	t.Helper()

	built, err := runtime.NewCompiler(runtime.WithPlugins(listener.Registry())).
		Compile(SCRIPT, []byte(src))
	if err != nil {
		return "", err
	}

	value, err := built.Run(context.Background())
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// TestRemote_AScriptCannotTell is the whole point: the names read like any
// other module's.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_AScriptCannotTell(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, TOKEN, CLOCK, []string{NOW, ADD}, _Clock)
	_Installed(t, listener, 2)

	got, err := _Ran(t, listener, `
def main():
    return [clock.now(), clock.add(2, 3), type(clock)]
`)
	if err != nil {
		t.Fatalf("calling across a socket: %v", err)
	}

	if got != `["tick", 5.0, "module"]` {
		t.Fatalf("got %s", got)
	}
}

// TestRemote_AConflictIsStillTheRegistrysToRefuse checks that no second clash
// check appeared.
//
// The adapter is an ordinary Plugin, so Environment's existing merge is what
// refuses. A remote registration that grew its own check would be a second
// place for the same rule to be wrong.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_AConflictIsStillTheRegistrysToRefuse(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, TOKEN, CLOCK, []string{NOW}, _Clock)
	_Answering(t, socket, TOKEN, "clock2", []string{NOW}, _Clock)
	_Installed(t, listener, 2)

	_, err := _Ran(t, listener, "def main():\n    return clock.now()\n")
	if !errors.Is(err, plugin.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

// TestRemote_AFunctionNeverCrosses is the type check at the boundary, in both
// the places it happens.
//
// What the source shows is refused at compile, by one Check written here and
// shared by every remote plugin. What only the value knows is refused when it
// runs, by the marshalling. Neither reaches the socket, which is why a plugin
// never has to have an opinion about a lambda.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_AFunctionNeverCrosses(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, TOKEN, CLOCK, []string{NOW, ADD}, _Clock)
	_Installed(t, listener, 2)

	cases := map[string]string{
		"a declared function": "def helper():\n    return 1\n\n" +
			"def main():\n    return clock.add(helper)\n",
		"in parentheses": "def helper():\n    return 1\n\n" +
			"def main():\n    return clock.add((helper))\n",
		"a lambda": "def main():\n    return clock.add(lambda: 1)\n",
		"one the source cannot see": "def main():\n" +
			"    held = [clock.now]\n    return clock.add(held[0])\n",
		"inside a list": "def helper():\n    return 1\n\n" +
			"def main():\n    return clock.add([1, helper])\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := _Ran(t, listener, src)
			if !errors.Is(err, remote.ErrArgument) {
				t.Fatalf("got %v, want ErrArgument", err)
			}
		})
	}

	// And data still crosses.
	got, err := _Ran(t, listener, "def main():\n    return clock.add(1, 2, 3)\n")
	if err != nil {
		t.Fatalf("data was refused: %v", err)
	}

	if got != "6.0" {
		t.Fatalf("got %s", got)
	}
}

// TestRemote_AStrangerInstallsNothing is the token doing its work.
//
// A registered plugin's names go into every script compiled with the registry,
// and file reaches whatever this process reaches. So a process that merely
// found the socket must not be able to put a name into a script.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_AStrangerInstallsNothing(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, "not-the-token", CLOCK, []string{NOW}, _Clock)

	// Long enough that an accepted registration would have landed.
	time.Sleep(200 * time.Millisecond)

	held := listener.Names()
	if len(held) != 0 {
		t.Fatalf("a stranger installed %v", held)
	}

	_, err := _Ran(t, listener, "def main():\n    return clock.now()\n")
	if err == nil {
		t.Fatal("clock was reachable after a refused registration")
	}
}

// TestRemote_APluginThatLeavesFailsItsNames is what a script sees when the
// other process is no longer there.
//
// The names stay, because a run has already built its environment and making
// them vanish partway turns one failure into a stranger one. The call fails
// rather than waiting on a stream nobody is reading.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_APluginThatLeavesFailsItsNames(t *testing.T) {
	listener, socket := _Listening(t)

	leave := _Answering(t, socket, TOKEN, CLOCK, []string{NOW}, _Clock)
	_Installed(t, listener, 1)

	got, err := _Ran(t, listener, "def main():\n    return clock.now()\n")
	if err != nil {
		t.Fatalf("before leaving: %v", err)
	}

	if got != `"tick"` {
		t.Fatalf("before leaving, got %s", got)
	}

	leave()

	deadline := time.Now().Add(_SETTLED)

	for time.Now().Before(deadline) {
		_, err = _Ran(t, listener, "def main():\n    return clock.now()\n")
		if err != nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !errors.Is(err, remote.ErrGone) {
		t.Fatalf("after leaving: %v, want ErrGone", err)
	}
}

// TestRemote_WhatThePluginRefusedReachesTheScript keeps a plugin's own failure
// from reading as this package's.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_WhatThePluginRefusedReachesTheScript(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, TOKEN, CLOCK, []string{NOW}, func(ask *pluginpb.Ask) *pluginpb.Answer {
		return &pluginpb.Answer{Id: ask.GetId(), Failed: "the clock is broken"}
	})
	_Installed(t, listener, 1)

	_, err := _Ran(t, listener, "def main():\n    return clock.now()\n")
	if !errors.Is(err, remote.ErrRemote) {
		t.Fatalf("got %v, want ErrRemote", err)
	}

	if !strings.Contains(err.Error(), "the clock is broken") {
		t.Fatalf("the refusal lost what the plugin said: %v", err)
	}

	if !strings.Contains(err.Error(), NOW) {
		t.Fatalf("the refusal does not say which name: %v", err)
	}
}

// TestRemote_ThreadsShareOnePlugin is why an ask carries an id.
//
// Several threads of one run call the same plugin at once, so their asks and
// answers interleave on one stream. Each caller has to be handed its own
// answer, which is what the id pairs.
//
// Revisions:
//   - 2026-09-25 06:40: initial creation
func TestRemote_ThreadsShareOnePlugin(t *testing.T) {
	listener, socket := _Listening(t)

	_Answering(t, socket, TOKEN, CLOCK, []string{ADD}, _Clock)
	_Installed(t, listener, 1)

	// Each thread sends a different pair, so an answer handed to the wrong
	// caller shows up as a wrong number. Twenty threads all adding the same
	// thing would pass however badly the ids were paired.
	got, err := _Ran(t, listener, `
def adder(n):
    def go():
        return clock.add(n, n)

    return go

def main():
    held = [spawn(adder(i)) for i in range(20)]

    return [join(h)[0] for h in held]
`)
	if err != nil {
		t.Fatalf("twenty threads through one plugin: %v", err)
	}

	want := starlark.NewList(nil)

	for i := range 20 {
		err = want.Append(starlark.Float(float64(i * 2)))
		if err != nil {
			t.Fatal(err)
		}
	}

	if got != want.String() {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestRemote_ATimeoutCutsAStalledCall is the debt every blocking builtin owes,
// paid by a plugin call.
//
// The plugin takes the ask and never answers. Without waiting on the caller's
// context the call waited the plugin out: measured with a real process stopped
// mid-call, timeout(2) had not returned after twenty-five seconds, and an
// interrupt could not reach it either.
//
// Deadlined, because a regression here does not fail - it hangs, and a suite
// that has to be killed says nothing about which test was wrong.
//
// Revisions:
//   - 2026-09-26 01:38: initial creation
func TestRemote_ATimeoutCutsAStalledCall(t *testing.T) {
	listener, socket := _Listening(t)

	stuck := make(chan struct{})

	t.Cleanup(func() {
		close(stuck)
	})

	_Answering(t, socket, TOKEN, CLOCK, []string{NOW}, func(*pluginpb.Ask) *pluginpb.Answer {
		// Takes the ask, answers nothing, until the test is over.
		<-stuck

		return nil
	})
	_Installed(t, listener, 1)

	type outcome struct {
		got string
		err error
	}

	answered := make(chan outcome, 1)

	go func() {
		got, err := _Ran(t, listener, `
def ask():
    return clock.now()

def main():
    return timeout(1, ask)
`)

		answered <- outcome{got: got, err: err}
	}()

	select {
	case held := <-answered:
		if !errors.Is(held.err, flow.ErrTimeout) {
			t.Fatalf("got %v, want ErrTimeout", held.err)
		}

	case <-time.After(_STALLED):
		t.Fatalf("a timeout of one second did not return within %v: the call is "+
			"not waiting on its caller", _STALLED)
	}
}
