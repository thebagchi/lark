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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/runtime/plugin/deep"
)

const (
	// SEPARATOR is what divides a module from a name in what a plugin
	// announces: "clock.now".
	SEPARATOR = "."
)

var (
	// ErrGone is returned when the plugin that supplied a name has left.
	ErrGone = errors.New("the plugin that supplied this name has gone")

	// ErrRemote is returned for a failure the plugin itself reported.
	ErrRemote = errors.New("the plugin refused")

	// ErrArgument is returned for an argument that cannot cross a socket.
	ErrArgument = errors.New("not data a plugin can be given")
)

// _Remote is one attached plugin, seen by the host as an ordinary Plugin.
//
// This is what makes the wire look like the registry: Name and Values are all
// the runtime ever asks for, and neither says anything about gRPC.
type _Remote struct {
	name  string
	names []string

	asking  sync.Mutex
	asks    chan *pluginpb.Ask
	waiting map[uint64]chan *pluginpb.Answer
	ticket  atomic.Uint64
	gone    chan struct{}
	once    sync.Once
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
	held := starlark.StringDict{}
	modules := map[string]starlark.StringDict{}

	for _, named := range r.names {
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

		return r._Ask(fn.Name(), named, sent)
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
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) _Ask(
	who string,
	named string,
	args []*structpb.Value,
) (starlark.Value, error) {
	id := r.ticket.Add(1)
	answers := make(chan *pluginpb.Answer, 1)

	r.asking.Lock()
	r.waiting[id] = answers
	r.asking.Unlock()

	defer func() {
		r.asking.Lock()
		delete(r.waiting, id)
		r.asking.Unlock()
	}()

	ask := &pluginpb.Ask{Id: id, Name: named, Args: args}

	select {
	case r.asks <- ask:
	case <-r.gone:
		return nil, fmt.Errorf("%s: %w", who, ErrGone)
	}

	select {
	case answer := <-answers:
		if answer.GetFailed() != "" {
			return nil, fmt.Errorf("%s: %s: %w", who, answer.GetFailed(), ErrRemote)
		}

		return _Starlark(answer.GetResult())

	case <-r.gone:
		return nil, fmt.Errorf("%s: %w", who, ErrGone)
	}
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
	r.asking.Lock()
	waiting, found := r.waiting[answer.GetId()]
	r.asking.Unlock()

	if !found {
		return
	}

	select {
	case waiting <- answer:
	default:
	}
}

// _Left marks this plugin as gone, once, so every waiting call fails rather
// than blocking on a stream nobody is reading.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) _Left() {
	r.once.Do(func() {
		close(r.gone)
	})
}
