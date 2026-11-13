package host

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"slices"
	"sort"
	"sync"

	"google.golang.org/grpc"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// UNIX is the only network a listener is offered.
	//
	// An attached plugin's names go into every script compiled with the
	// listener's plugins, beside whatever else its host hands that compiler -
	// file among them, which reaches whatever this process reaches. A TCP port
	// would therefore put the filesystem behind whoever can dial it, so there
	// is no TCP option to reach for and no default to get wrong.
	//
	// Revisions:
	//   - 2026-09-25 06:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	UNIX = "unix"

	// SOCKET is the mode a socket is created with: this user and nobody else.
	//
	// Revisions:
	//   - 2026-09-25 06:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	SOCKET = 0o600

	// PENDING is how many asks may wait for the stream writer before a
	// caller blocks. Small on purpose: a plugin that is not keeping up should
	// slow its callers rather than grow a queue nobody is bounded by.
	//
	// Revisions:
	//   - 2026-09-25 06:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	PENDING = 16

	// MAX_RECEIVE is the largest message the host accepts from a plugin: 4 MiB.
	//
	// It equals grpc v1.84.0 defaultServerMaxReceiveMessageSize
	// (google.golang.org/grpc@v1.84.0/server.go:61). A plugin chooses the size
	// of each answer, so this is the most memory one answer can make the host
	// hold. It is stated so the bound is visible and a grpc upgrade cannot
	// change it unseen.
	//
	// A larger message makes grpc fail the stream with ResourceExhausted. The
	// receive loop in Register then returns that error after calling _Left,
	// which closes the plugin's gone channel. A script that asks that plugin
	// next sees ErrGone ("the plugin that supplied this name has gone"), from
	// the gone case in the ask offer in remote.go. The script never sees
	// ResourceExhausted.
	//
	// Revisions:
	//   - 2026-11-02 11:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	MAX_RECEIVE = 1024 * 1024 * 4

	// MAX_SEND is the largest message the host sends to a plugin: math.MaxInt32.
	//
	// It equals grpc v1.84.0 defaultServerMaxSendMessageSize (server.go:62).
	// The host keeps the grpc default here, so it limits nothing beyond grpc.
	// It is stated for the same reason as MAX_RECEIVE. Lowering it would
	// change behavior for large arguments, so that needs a decision.
	//
	// Revisions:
	//   - 2026-11-02 11:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	MAX_SEND = math.MaxInt32
)

// ErrToken is returned for a stream that did not present the token.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
var (
	ErrToken = errors.New("a plugin presented the wrong token")
)

// ErrFirst is returned for a stream whose first message was not a Register.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
var (
	ErrFirst = errors.New("a plugin spoke before it announced itself")
)

// Listener is the host's socket, and the plugins attached to it.
//
// Made by Listen, which returns an error - this is what replaces init for a
// remote plugin. A blank import cannot fail, cannot wait, and cannot close a
// socket when the host exits; a call from main can do all three.
//
// attached is each plugin's adapter by name, and installed the same adapters
// in the order their plugins first attached, which is the order Plugins hands
// a compile; both are held under guard, with what this listener started.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote; implements common.Listener (C91w8c)
//   - 2026-11-08 02:10: cleanup is a directory Close removes (C84, Q240)
//   - 2026-11-08 10:43: closed is a channel Close closes, so a Load that waits ends (C109, Q253)
type Listener struct {
	guard sync.Mutex
	pluginpb.UnimplementedServiceServer
	token     string
	server    *grpc.Server
	socket    string
	cleanup   string
	attached  map[string]*_Remote
	installed []common.Plugin
	started   []*exec.Cmd
	closed    chan struct{}
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
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-10 16:05: cuts the socket path and the path in its errors to common.QUOTE_LIMIT
//     bytes (C117, B239, Q4)
//   - 2026-11-11 00:42: cuts the address in the error of the system too (C122, B269)
func Listen(
	socket string,
	token string,
) (*Listener, error) {
	err := os.Remove(socket)
	if err != nil && !errors.Is(
		err,
		os.ErrNotExist,
	) {
		return nil, fmt.Errorf(
			"clearing %s: %w",
			unpack.ShortText(socket),
			unpack.ShortPath(err),
		)
	}

	held, err := net.Listen(
		UNIX,
		socket,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"listening on %s: %w",
			unpack.ShortText(socket),
			unpack.ShortPath(err),
		)
	}

	return _Serving(
		socket,
		token,
		held,
	)
}

// _Serving secures the socket and answers on it until Close.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:54: states the message size limits as RECEIVE and SEND
//   - 2026-11-02 11:18: renamed to MAX_RECEIVE and MAX_SEND; comments state
//     what a script observes
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:43: makes the closed channel (C109, Q253)
//   - 2026-11-10 16:05: cuts the socket path and the path in its error to common.QUOTE_LIMIT
//     bytes (C117, B239, Q4)
func _Serving(
	socket string,
	token string,
	held net.Listener,
) (*Listener, error) {
	err := os.Chmod(
		socket,
		SOCKET,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"securing %s: %w",
			unpack.ShortText(socket),
			unpack.ShortPath(err),
		)
	}

	listener := &Listener{
		token: token,
		server: grpc.NewServer(
			grpc.MaxRecvMsgSize(MAX_RECEIVE),
			grpc.MaxSendMsgSize(MAX_SEND),
		),
		socket:   socket,
		attached: map[string]*_Remote{},
		closed:   make(chan struct{}),
	}

	pluginpb.RegisterServiceServer(
		listener.server,
		listener,
	)

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
//   - 2026-11-04 23:50: a nil Listener has nothing to close (C86 fix round 2)
//   - 2026-11-04 23:50: a zero Listener has no server, so it has nothing to close
//     either (C86 fix round 3)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 02:10: removes the directory ListenTemp made, even after an error (C84, Q240)
//   - 2026-11-08 10:43: closes the closed channel first, so a Load that waits
//     for a quiet plugin returns (C109, Q253)
func (l *Listener) Close() error {
	if l == nil || l.server == nil {
		return nil
	}

	l._Shut()

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
	if errors.Is(
		err,
		os.ErrNotExist,
	) {
		err = nil
	}

	if l.cleanup == "" {
		return err
	}

	// RemoveAll of a directory already gone is not an error, so a second Close
	// is safe.
	return errors.Join(
		err,
		os.RemoveAll(l.cleanup),
	)
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
//   - 2026-11-04 23:50: a nil listener or stream is a stream that ended before it announced
//     anything: io.EOF (C86 fix round 3)
//   - 2026-11-04 23:50: a zero listener, which has no server and no adapters, is the
//     same (C86 fix round 3)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:43: compares the token in constant time (C109, Q254)
func (l *Listener) Register(stream pluginpb.Service_RegisterServer) error {
	if l == nil || l.server == nil || stream == nil {
		return io.EOF
	}

	first, err := stream.Recv()
	if err != nil {
		return err
	}

	announced := first.GetRegister()
	if announced == nil {
		return ErrFirst
	}

	// Lengths that differ return 0 at once, which leaks only the length.
	if subtle.ConstantTimeCompare(
		[]byte(announced.GetToken()),
		[]byte(l.token),
	) != 1 {
		return ErrToken
	}

	remote := l._Adapter(announced.GetName())

	var (
		asks chan *pluginpb.Ask
		gone chan struct{}
	)

	asks, gone, err = remote._Attach(announced.GetNames())
	if err != nil {
		return err
	}

	defer remote._Left(gone)

	err = stream.Send(&pluginpb.Response{
		Of: &pluginpb.Response_Accepted{
			Accepted: &pluginpb.Accepted{},
		},
	})
	if err != nil {
		return err
	}

	return _Carry(
		stream,
		remote,
		asks,
		gone,
	)
}

// _Carry runs the stream until it closes: asks out, answers in.
//
// Two directions on one stream, so the sending half runs beside the receiving
// half. The receiving half is this goroutine, because its ending is what ends
// the call.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func _Carry(
	stream pluginpb.Service_RegisterServer,
	remote *_Remote,
	asks chan *pluginpb.Ask,
	gone chan struct{},
) error {
	var sending sync.WaitGroup

	sending.Add(1)

	go _Sending(
		&sending,
		&_Pipe{
			stream: stream,
			remote: remote,
			asks:   asks,
			gone:   gone,
		},
	)

	defer sending.Wait()

	for {
		held, err := stream.Recv()
		if err != nil {
			remote._Left(gone)

			if errors.Is(
				err,
				io.EOF,
			) {
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

// _Pipe is one attached stream and the asks it sends.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
type _Pipe struct {
	stream pluginpb.Service_RegisterServer
	remote *_Remote
	asks   chan *pluginpb.Ask
	gone   chan struct{}
}

// _Sending writes each ask until the stream or the attach ends.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func _Sending(
	sending *sync.WaitGroup,
	pipe *_Pipe,
) {
	defer sending.Done()

	for {
		select {
		case ask := <-pipe.asks:
			err := pipe.stream.Send(&pluginpb.Response{
				Of: &pluginpb.Response_Ask{
					Ask: ask,
				},
			})
			if err != nil {
				pipe.remote._Left(pipe.gone)

				return
			}

		case <-pipe.gone:
			return
		}
	}
}

// Names is what every attached plugin announced, for the compile-time check.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-11-04 23:50: a nil Listener carries none (C86 fix round 2)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func (l *Listener) Names() []string {
	if l == nil {
		return nil
	}

	l.guard.Lock()
	attached := make(
		[]*_Remote,
		0,
		len(l.attached),
	)

	for _, remote := range l.attached {
		attached = append(
			attached,
			remote,
		)
	}
	l.guard.Unlock()

	held := []string{}

	for _, remote := range attached {
		_, names := remote._Named()
		held = append(
			held,
			names...,
		)
	}

	sort.Strings(held)

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
//   - 2026-11-04 23:50: a nil Listener installs none (C86 fix round 2)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func (l *Listener) Plugins() []common.Plugin {
	if l == nil {
		return nil
	}

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
//   - 2026-11-04 23:50: a nil Listener has none up (C86 fix round 2)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func (l *Listener) Live() []string {
	if l == nil {
		return nil
	}

	l.guard.Lock()
	attached := make(
		[]*_Remote,
		0,
		len(l.attached),
	)

	for _, remote := range l.attached {
		attached = append(
			attached,
			remote,
		)
	}
	l.guard.Unlock()

	held := []string{}

	for _, remote := range attached {
		if remote._Live() {
			name, _ := remote._Named()
			held = append(
				held,
				name,
			)
		}
	}

	sort.Strings(held)

	return held
}

// _Shut closes the closed channel, once. A second call finds it closed.
//
// Under guard, so _Start sees either "not closed, and its process is in
// started before _Stop reads that list" or "closed".
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
func (l *Listener) _Shut() {
	l.guard.Lock()
	defer l.guard.Unlock()

	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
}

// _Adapter is the adapter for this plugin name, made and installed if this is
// the first time the name has been seen.
//
// Installed into this listener's own list, never the registered list, and only the first
// time. A plugin that restarts reuses the adapter already installed, because a
// second one would clash with the first for as long as the host lived.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
//   - 2026-10-03 08:28: installs a new adapter itself, there being no registry for
//     the caller to install it in
//   - 2026-11-07 19:00: comment says the registered list, not DEFAULT (C91w1)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
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
	l.installed = append(
		l.installed,
		held,
	)

	return held
}
