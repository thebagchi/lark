package remote

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"

	"google.golang.org/grpc"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/runtime/plugin"
)

const (
	// UNIX is the only network a listener is offered.
	//
	// A registered plugin's names go into every script compiled with the
	// registry, and file is in that environment by import - it reaches
	// whatever this process reaches. A TCP port would therefore put the
	// filesystem behind whoever can dial it, so there is no TCP option to
	// reach for and no default to get wrong.
	UNIX = "unix"

	// SOCKET is the mode a socket is created with: this user and nobody else.
	SOCKET = 0o600

	// _PENDING is how many asks may wait for the stream writer before a
	// caller blocks. Small on purpose: a plugin that is not keeping up should
	// slow its callers rather than grow a queue nobody is bounded by.
	_PENDING = 16
)

// ErrToken is returned for a stream that did not present the token.
var ErrToken = errors.New("a plugin presented the wrong token")

// ErrFirst is returned for a stream whose first message was not a Register.
var ErrFirst = errors.New("a plugin spoke before it announced itself")

// Listener is the host's socket, and the plugins attached to it.
//
// Made by Listen, which returns an error - this is what replaces init for a
// remote plugin. A blank import cannot fail, cannot wait, and cannot close a
// socket when the host exits; a call from main can do all three.
type Listener struct {
	pluginpb.UnimplementedServiceServer

	token    string
	registry *plugin.Registry
	server   *grpc.Server
	socket   string

	guard    sync.Mutex
	attached []*_Remote
	started  []*exec.Cmd
}

// Listen serves on socket, installing whatever attaches into registry.
//
// The socket is a path, not an address. It is removed first if something stale
// is there, and created for this user alone.
//
// Returns the listener, which the caller closes. Never panics.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func Listen(socket string, token string, registry *plugin.Registry) (*Listener, error) {
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
		registry: registry,
		server:   grpc.NewServer(),
		socket:   socket,
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
	l.attached = nil
	l.guard.Unlock()

	for _, remote := range attached {
		remote._Left()
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
func (l *Listener) Register(stream pluginpb.Service_RegisterServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}

	announced := first.GetRegister()
	if announced == nil {
		return ErrFirst
	}

	if announced.GetToken() != l.token {
		return ErrToken
	}

	remote := &_Remote{
		name:    announced.GetName(),
		names:   announced.GetNames(),
		asks:    make(chan *pluginpb.Ask, _PENDING),
		waiting: map[uint64]chan *pluginpb.Answer{},
		gone:    make(chan struct{}),
	}

	l.guard.Lock()
	l.attached = append(l.attached, remote)
	l.guard.Unlock()

	// Into the registry the host was given, never DEFAULT. A remote plugin
	// that comes and goes has no business in the registry every compiler in
	// the process shares.
	l.registry.Register(remote)

	defer remote._Left()

	err = stream.Send(&pluginpb.Response{
		Of: &pluginpb.Response_Accepted{Accepted: &pluginpb.Accepted{}},
	})
	if err != nil {
		return err
	}

	return _Carry(stream, remote)
}

// _Carry runs the stream until it closes: asks out, answers in.
//
// Two directions on one stream, so the sending half runs beside the receiving
// half. The receiving half is this goroutine, because its ending is what ends
// the call.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Carry(stream pluginpb.Service_RegisterServer, remote *_Remote) error {
	var sending sync.WaitGroup

	sending.Add(1)

	go func() {
		defer sending.Done()

		for {
			select {
			case ask := <-remote.asks:
				err := stream.Send(&pluginpb.Response{
					Of: &pluginpb.Response_Ask{Ask: ask},
				})
				if err != nil {
					remote._Left()

					return
				}

			case <-remote.gone:
				return
			}
		}
	}()

	defer sending.Wait()

	for {
		held, err := stream.Recv()
		if err != nil {
			remote._Left()

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
	defer l.guard.Unlock()

	held := []string{}

	for _, remote := range l.attached {
		held = append(held, remote.names...)
	}

	return held
}

// Registry is where this listener installs what attaches.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (l *Listener) Registry() *plugin.Registry {
	return l.registry
}
