package remote

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"sort"
	"sync"

	"google.golang.org/grpc"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	// UNIX is the only network a listener is offered.
	//
	// An attached plugin's names go into every script compiled with the
	// listener's plugins, beside whatever else its host hands that compiler -
	// file among them, which reaches whatever this process reaches. A TCP port
	// would therefore put the filesystem behind whoever can dial it, so there
	// is no TCP option to reach for and no default to get wrong.
	UNIX = "unix"

	// SOCKET is the mode a socket is created with: this user and nobody else.
	SOCKET = 0o600

	// _PENDING is how many asks may wait for the stream writer before a
	// caller blocks. Small on purpose: a plugin that is not keeping up should
	// slow its callers rather than grow a queue nobody is bounded by.
	_PENDING = 16
)

// ERR_TOKEN is returned for a stream that did not present the token.
var ERR_TOKEN = errors.New("a plugin presented the wrong token")

// ERR_FIRST is returned for a stream whose first message was not a Register.
var ERR_FIRST = errors.New("a plugin spoke before it announced itself")

// Listener is the host's socket, and the plugins attached to it.
//
// Made by Listen, which returns an error - this is what replaces init for a
// remote plugin. A blank import cannot fail, cannot wait, and cannot close a
// socket when the host exits; a call from main can do all three.
//
// attached is each plugin's adapter by name, and installed the same adapters
// in the order their plugins first attached, which is the order Plugins hands
// a compile; both are held under guard, with what this listener started.
type Listener struct {
	pluginpb.UnimplementedServiceServer

	token  string
	server *grpc.Server
	socket string

	guard     sync.Mutex
	attached  map[string]*_Remote
	installed []plugin.Plugin
	started   []*exec.Cmd
}

// Listen serves on socket, keeping whatever attaches for Plugins to hand a
// compiler.
//
// The socket is a path, not an address. It is removed first if something stale
// is there, and created for this user alone.
//
// Returns the listener, which the caller closes. Never panics.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-10-02 17:12: keeps what attaches itself, a host handing a compiler
//     its plugins with WithPlugins and nothing else
func Listen(socket string, token string) (*Listener, error) {
	err := os.Remove(socket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clearing %s: %w", socket, err)
	}

	held, err := net.Listen(UNIX, socket)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", socket, err)
	}

	err = os.Chmod(socket, SOCKET)
	if err != nil {
		return nil, fmt.Errorf("securing %s: %w", socket, err)
	}

	listener := &Listener{
		token:    token,
		server:   grpc.NewServer(),
		socket:   socket,
		attached: map[string]*_Remote{},
	}

	pluginpb.RegisterServiceServer(listener.server, listener)

	go func() {
		// Serve returns when the server is stopped, which Close does.
		_ = listener.server.Serve(held)
	}()

	return listener, nil
}

// Close stops serving, kills every plugin this listener started, and drops
// every plugin attached to it.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-09-25 23:56: sees off what it started, stopping the server first so
//     a plugin is told before it is killed
//   - 2026-09-26 00:45: a socket the listener already unlinked is not a failure
func (l *Listener) Close() error {
	// The server first: that closes every stream, which is how a plugin learns
	// its host has gone and the reason most of them need no killing.
	l.server.Stop()
	l._Stop()

	l.guard.Lock()
	attached := l.attached
	l.guard.Unlock()

	// The adapters stay in the map, because they are still in the registry and
	// their names are still in environments built from them. What ends is each
	// one's stream.
	for _, remote := range attached {
		_, gone := remote._Stream()
		remote._Left(gone)
	}

	// Go's unix listener unlinks the socket when it closes, so by here the file
	// is usually gone already and removing it is tidying up after a case that
	// did not happen. Reporting that as a failure made a run that worked say it
	// had failed.
	err := os.Remove(l.socket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// Register is a plugin's whole conversation: it announces itself, then answers
// asks until the stream closes.
//
// Returning ends the stream, which is how a plugin that fails the token is
// refused before any name of its is taken.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-09-25 06:34: named for the rpc, which is Register
//   - 2026-10-03 08:28: installs nothing itself, _Adapter installing a new plugin as it
//     makes its adapter
func (l *Listener) Register(stream pluginpb.Service_RegisterServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}

	announced := first.GetRegister()
	if announced == nil {
		return ERR_FIRST
	}

	if announced.GetToken() != l.token {
		return ERR_TOKEN
	}

	remote := l._Adapter(announced.GetName())

	asks, gone, err := remote._Attach(announced.GetNames())
	if err != nil {
		return err
	}

	defer remote._Left(gone)

	err = stream.Send(&pluginpb.Response{
		Of: &pluginpb.Response_Accepted{Accepted: &pluginpb.Accepted{}},
	})
	if err != nil {
		return err
	}

	return _Carry(stream, remote, asks, gone)
}

// _Carry runs the stream until it closes: asks out, answers in.
//
// Two directions on one stream, so the sending half runs beside the receiving
// half. The receiving half is this goroutine, because its ending is what ends
// the call.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Carry(
	stream pluginpb.Service_RegisterServer,
	remote *_Remote,
	asks chan *pluginpb.Ask,
	gone chan struct{},
) error {
	var sending sync.WaitGroup

	sending.Add(1)

	go func() {
		defer sending.Done()

		for {
			select {
			case ask := <-asks:
				err := stream.Send(&pluginpb.Response{
					Of: &pluginpb.Response_Ask{Ask: ask},
				})
				if err != nil {
					remote._Left(gone)

					return
				}

			case <-gone:
				return
			}
		}
	}()

	defer sending.Wait()

	for {
		held, err := stream.Recv()
		if err != nil {
			remote._Left(gone)

			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}

		answer := held.GetAnswer()
		if answer != nil {
			remote._Answered(answer)
		}
	}
}

// Names is what every attached plugin announced, for the compile-time check.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (l *Listener) Names() []string {
	l.guard.Lock()
	attached := make([]*_Remote, 0, len(l.attached))

	for _, remote := range l.attached {
		attached = append(attached, remote)
	}
	l.guard.Unlock()

	held := []string{}

	for _, remote := range attached {
		_, names := remote._Named()
		held = append(held, names...)
	}

	sort.Strings(held)

	return held
}

// _Adapter is the adapter for this plugin name, made and installed if this is
// the first time the name has been seen.
//
// Installed into this listener's own list, never DEFAULT, and only the first
// time. A plugin that restarts reuses the adapter already installed, because a
// second one would clash with the first for as long as the host lived.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
//   - 2026-10-03 08:28: installs a new adapter itself, there being no registry for
//     the caller to install it in
func (l *Listener) _Adapter(name string) *_Remote {
	l.guard.Lock()
	defer l.guard.Unlock()

	held, found := l.attached[name]
	if found {
		return held
	}

	held = &_Remote{
		name:    name,
		waiting: map[uint64]chan *pluginpb.Answer{},
	}

	l.attached[name] = held
	l.installed = append(l.installed, held)

	return held
}

// Plugins is every plugin this listener has installed, for a compiler to be
// given.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation, as Registry, returning the registry
//     itself
//   - 2026-10-02 13:12: the plugins, since a compiler takes those rather than a
//     registry
//   - 2026-10-03 08:28: a copy of the list it installs into, under its lock
func (l *Listener) Plugins() []plugin.Plugin {
	l.guard.Lock()
	defer l.guard.Unlock()

	return slices.Clone(l.installed)
}

// Live is the name of every plugin with a stream open now.
//
// Different from Names, which is what this listener carries: a plugin that has
// died leaves its names behind, because they are in environments already built
// from them, and only reappears here when a process attaches for it again. So
// this is the question "is my plugin up", and Names is "what can a script
// call".
//
// Revisions:
//   - 2026-09-26 01:56: initial creation
func (l *Listener) Live() []string {
	l.guard.Lock()
	attached := make([]*_Remote, 0, len(l.attached))

	for _, remote := range l.attached {
		attached = append(attached, remote)
	}
	l.guard.Unlock()

	held := []string{}

	for _, remote := range attached {
		if remote._Live() {
			name, _ := remote._Named()
			held = append(held, name)
		}
	}

	sort.Strings(held)

	return held
}
