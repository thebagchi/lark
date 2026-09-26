// Package remote is how a plugin that lives in another process becomes names a
// script can call.
//
// A plugin elsewhere dials this host, says what it is called and what it
// supplies, and answers calls of those names down the same stream. What the
// runtime sees is an ordinary plugin.Plugin: Name and Values, neither of which
// mentions a socket. That is the whole of the idea - the wire is how an
// implementation is filled, not a second kind of plugin.
//
// A plugin process does not import this package, or any of this module. It
// speaks the schema in proto/plugin.proto and nothing else, which is what lets
// one be written in a language this repository does not.
//
// What cannot cross does not: a function, a thread handle and a module are
// refused here, so a plugin is never handed one. The argument for that, and the
// decisions this package follows, are in .doc/plugin.md and
// .doc/impl/lark-plugin-grpc.md.
package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// SEPARATOR is what divides a module from a name in what a plugin
	// announces: "clock.now".
	SEPARATOR = "."
)

var (
	// ErrGone is returned when the plugin that supplied a name has left.
	ErrGone = errors.New("the plugin that supplied this name has gone")

	// ErrAttached is returned to a plugin claiming a name another process is
	// already answering for.
	ErrAttached = errors.New("a plugin of this name is already attached")

	// ErrRenamed is returned to a returning plugin announcing different names
	// from the ones it announced before.
	ErrRenamed = errors.New("a plugin may not change what it supplies by restarting")

	// ErrRemote is returned for a failure the plugin itself reported.
	ErrRemote = errors.New("the plugin refused")

	// ErrArgument is returned for an argument that cannot cross a socket.
	ErrArgument = errors.New("not data a plugin can be given")
)

// _Remote is one plugin as the host sees it, across however many times its
// process has attached.
//
// This is what makes the wire look like the registry: Name and Values are all
// the runtime ever asks for, and neither says anything about gRPC.
//
// One of these per plugin name, not per connection, and that is the whole of
// how a plugin can be restarted. The registry only ever appends - it has no
// removal, deliberately, because a name that could be taken back is a name a
// script cannot rely on - so a second adapter for a returning plugin would
// clash with the first forever. Instead the adapter outlives the process and
// takes a new stream when one arrives.
type _Remote struct {
	name string

	guard   sync.Mutex
	names   []string
	asks    chan *pluginpb.Ask
	gone    chan struct{}
	live    bool
	waiting map[uint64]chan *pluginpb.Answer

	ticket atomic.Uint64
}

// _Attach gives this plugin a new stream to answer on.
//
// Refuses a second stream while one is live, so two processes cannot both claim
// one name and answer half its calls each. Refuses a returning plugin that
// announces different names, because the names are already in environments that
// were built from them.
//
// Returns the channels this stream is to use. They are handed back rather than
// read from the struct, so a later attach cannot make an earlier stream's
// goroutines write to the wrong place.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
func (r *_Remote) _Attach(names []string) (chan *pluginpb.Ask, chan struct{}, error) {
	r.guard.Lock()
	defer r.guard.Unlock()

	if r.live {
		return nil, nil, fmt.Errorf("%s: %w", r.name, ErrAttached)
	}

	if r.names != nil && !_Same(r.names, names) {
		return nil, nil, fmt.Errorf("%s: was %v, now %v: %w",
			r.name, r.names, names, ErrRenamed)
	}

	r.names = names
	r.asks = make(chan *pluginpb.Ask, _PENDING)
	r.gone = make(chan struct{})
	r.live = true

	return r.asks, r.gone, nil
}

// _Stream is the channels a call should use, read together so they belong to
// the same attach.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
func (r *_Remote) _Stream() (chan *pluginpb.Ask, chan struct{}) {
	r.guard.Lock()
	defer r.guard.Unlock()

	return r.asks, r.gone
}

// _Same reports whether two announcements name the same set.
//
// Order is not a difference: what a plugin supplies is a set, and a plugin that
// listed its names differently on restart has not changed what it supplies.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
func _Same(held []string, given []string) bool {
	if len(held) != len(given) {
		return false
	}

	seen := map[string]int{}

	for _, name := range held {
		seen[name]++
	}

	for _, name := range given {
		seen[name]--
	}

	for _, count := range seen {
		if count != 0 {
			return false
		}
	}

	return true
}

// Name is what a conflict report points at.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) Name() string {
	return r.name
}

// Values is one builtin per announced name, with dotted names gathered into
// the module they name.
//
// A plugin announcing "clock.now" supplies a module, because every in-process
// plugin here supplies one and a remote plugin that could not would be the odd
// one out.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) Values() starlark.StringDict {
	r.guard.Lock()
	names := r.names
	r.guard.Unlock()

	held := starlark.StringDict{}
	modules := map[string]starlark.StringDict{}

	for _, named := range names {
		module, member, dotted := strings.Cut(named, SEPARATOR)

		if !dotted {
			held[named] = r._Builtin(named, named)

			continue
		}

		if modules[module] == nil {
			modules[module] = starlark.StringDict{}
		}

		modules[module][member] = r._Builtin(named, module+SEPARATOR+member)
	}

	for module, members := range modules {
		held[module] = &starlarkstruct.Module{Name: module, Members: members}
	}

	return held
}

// _Builtin is one announced name, as a script calls it.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) _Builtin(named string, shown string) *starlark.Builtin {
	return starlark.NewBuiltin(shown, func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		if len(kwargs) > 0 {
			return nil, fmt.Errorf("%s: keyword arguments do not cross: %w",
				fn.Name(), ErrArgument)
		}

		sent, err := _Sendable(fn.Name(), args)
		if err != nil {
			return nil, err
		}

		return r._Ask(thread, fn.Name(), named, sent)
	})
}

// _Sendable is the arguments as the wire can carry them, or the reason they
// cannot be.
//
// This is the whole type check, and it is here rather than on the plugin: a
// value that cannot become a protobuf Value has no business leaving, and the
// question "is this data" is one deep.IsData already answers for the store.
// The plugin never sees a lambda, so it never has to have an opinion about
// one.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Sendable(who string, args starlark.Tuple) ([]*structpb.Value, error) {
	sent := make([]*structpb.Value, 0, len(args))

	for index, given := range args {
		bad, ok := deep.IsData(given)
		if !ok {
			return nil, fmt.Errorf("%s: argument %d holds %s: %w",
				who, index, bad.Type(), ErrArgument)
		}

		held, err := _Value(given)
		if err != nil {
			return nil, fmt.Errorf("%s: argument %d: %w", who, index, err)
		}

		sent = append(sent, held)
	}

	return sent, nil
}

// _Ask sends one call and waits for its answer.
//
// The ticket is what lets several threads of one run call the same plugin at
// once: answers come back down one stream in whatever order the plugin
// finishes them, so each is claimed by the caller that asked.
//
// Waits on the caller's context as well, which every blocking builtin owes -
// the same debt scheduler.Wait names. Without it a timeout around a plugin call
// waited the plugin out: measured with the process stopped mid-call, timeout(2)
// had not returned after twenty-five seconds, and an interrupt could not reach
// it either. A plugin that never answers would have held the run for as long as
// it liked.
//
// Returns ErrCancelled wrapping the context's error when the caller is
// cancelled first. The plugin is not told: the answer it eventually sends is
// dropped by _Answered, because nobody is waiting for it any more.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-09-26 01:34: waits on the caller's context
func (r *_Remote) _Ask(
	thread *starlark.Thread,
	who string,
	named string,
	args []*structpb.Value,
) (starlark.Value, error) {
	// A thread with no run cannot be cancelled, so it waits on a context that
	// never closes rather than being told about runs by a call to a plugin.
	ctx, err := scheduler.Context(thread)
	if err != nil {
		ctx = context.Background()
	}

	id := r.ticket.Add(1)
	answers := make(chan *pluginpb.Answer, 1)

	r.guard.Lock()
	r.waiting[id] = answers
	r.guard.Unlock()

	defer func() {
		r.guard.Lock()
		delete(r.waiting, id)
		r.guard.Unlock()
	}()

	ask := &pluginpb.Ask{Id: id, Name: named, Args: args}

	// Read together, so both belong to the same attach: a plugin that restarts
	// between these two lines would otherwise have this call writing to one
	// stream and watching another.
	asks, gone := r._Stream()
	if asks == nil {
		return nil, fmt.Errorf("%s: %w", who, ErrGone)
	}

	select {
	case asks <- ask:
	case <-gone:
		return nil, fmt.Errorf("%s: %w", who, ErrGone)
	case <-ctx.Done():
		return nil, _Cancelled(who, ctx)
	}

	select {
	case answer := <-answers:
		if answer.GetFailed() != "" {
			return nil, fmt.Errorf("%s: %s: %w", who, answer.GetFailed(), ErrRemote)
		}

		return _Starlark(answer.GetResult())

	case <-gone:
		return nil, fmt.Errorf("%s: %w", who, ErrGone)
	case <-ctx.Done():
		return nil, _Cancelled(who, ctx)
	}
}

// _Cancelled is what a call answers with when its caller was cancelled first.
//
// Revisions:
//   - 2026-09-26 01:34: initial creation
func _Cancelled(who string, ctx context.Context) error {
	return fmt.Errorf("%s: %w: %w", who, scheduler.ErrCancelled, ctx.Err())
}

// _Answered hands an answer to whoever is waiting for it.
//
// An answer nobody waits for is dropped rather than raised: the caller has
// already given up, which a cancelled run does, and there is nothing left to
// tell.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) _Answered(answer *pluginpb.Answer) {
	r.guard.Lock()
	waiting, found := r.waiting[answer.GetId()]
	r.guard.Unlock()

	if !found {
		return
	}

	select {
	case waiting <- answer:
	default:
	}
}

// _Left marks the given stream as gone, so every call waiting on it fails
// rather than blocking on something nobody is reading.
//
// Takes the stream it is ending rather than reading the current one. An old
// connection tearing down after a new one has attached would otherwise close
// the new one's channel and leave a live plugin unreachable. Idempotent for the
// same reason: a stream can be reported gone by its reader and its writer both.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-09-26 01:48: ends one stream rather than the plugin, so a restart is
//     not undone by the connection it replaced
func (r *_Remote) _Left(gone chan struct{}) {
	r.guard.Lock()
	defer r.guard.Unlock()

	if r.gone != gone {
		return
	}

	if !r.live {
		return
	}

	r.live = false

	close(gone)
}

// _Named is this plugin's name and what it currently supplies.
//
// Revisions:
//   - 2026-09-26 01:48: initial creation
func (r *_Remote) _Named() (string, []string) {
	r.guard.Lock()
	defer r.guard.Unlock()

	return r.name, r.names
}

// _Live reports whether a process is answering for this plugin now.
//
// Revisions:
//   - 2026-09-26 01:56: initial creation
func (r *_Remote) _Live() bool {
	r.guard.Lock()
	defer r.guard.Unlock()

	return r.live
}
