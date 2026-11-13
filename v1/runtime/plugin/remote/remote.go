// Package remote is how a plugin that lives in another process becomes names a
// script can call.
//
// A plugin elsewhere dials this host, says what it is called and what it
// supplies, and answers calls of those names down the same stream. What the
// runtime sees is an ordinary common.Plugin: Name and Values, neither of which
// mentions a socket. That is the whole of the idea - the wire is how an
// implementation is filled, not a second kind of plugin.
//
// A plugin process does not import this package, or any of this module. It
// speaks the schema in proto/plugin.proto and nothing else, which is what lets
// one be written in a language this repository does not.
//
// What cannot cross does not: a function, a thread handle and a module are
// refused here, so a plugin is never handed one - the wire carries what
// google.protobuf.Value can name and nothing else.
//
// Why the host listens and the plugin dials, rather than the other way round: the
// host decides what code runs, and nothing has to reach a plugin, so a plugin
// needs no port and no address anyone has to know. Why not a shared object opened
// in this process: Go's own plugin documentation argues against the mechanism,
// the race detector cannot see inside one while the test gate is -race, and
// HashiCorp declined it for the same memory-isolation reason after starting from
// it.
//
// The behavior lives in remote/internal/host; this package forwards to it. A host
// holds the listener as common.Listener and the result of a Load as common.Loading.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: thin forwards to remote/internal/host (C91w8c)
//   - 2026-11-08 02:10: ListenTemp (C84, Q240)
package remote

import (
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
)

// ErrGone is returned when the plugin that supplied a name has left.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrGone (C91w8c)
var (
	ErrGone = host.ErrGone
)

// ErrAttached is returned to a plugin claiming a name another process is
// already answering for.
//
// Revisions:
//   - 2026-09-26 09:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrAttached (C91w8c)
var (
	ErrAttached = host.ErrAttached
)

// ErrRenamed is returned to a returning plugin announcing different names
// from the ones it announced before.
//
// Revisions:
//   - 2026-09-26 09:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrRenamed (C91w8c)
var (
	ErrRenamed = host.ErrRenamed
)

// ErrRemote is returned for a failure the plugin itself reported.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrRemote (C91w8c)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
var (
	ErrRemote = host.ErrRemote
)

// ErrToken is returned for a stream that did not present the token.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrToken (C91w8c)
var (
	ErrToken = host.ErrToken
)

// ErrFirst is returned for a stream whose first message was not a Register.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrFirst (C91w8c)
var (
	ErrFirst = host.ErrFirst
)

// ErrQuiet is returned for a plugin that started but never announced itself.
//
// Revisions:
//   - 2026-09-26 00:31: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: the same error as host.ErrQuiet (C91w8c)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
var (
	ErrQuiet = host.ErrQuiet
)

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
//   - 2026-11-07 19:54: returns common.Listener; the work is host.Listen (C91w8c)
func Listen(
	socket string,
	token string,
) (common.Listener, error) {
	held, err := host.Listen(
		socket,
		token,
	)
	if err != nil {
		return nil, err
	}

	return held, nil
}

// Discovered is every plugin binary in dir matching glob, in a fixed order.
//
// An empty glob means this platform's binary extension. Matching a name rather
// than testing the execute bit is what go-plugin does, and it is enough here:
// a match that will not start is skipped and reported, which is the same path
// a broken binary takes anyway.
//
// Only glob is a pattern: dir is the directory as it is written, whatever
// characters its name holds.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
//   - 2026-11-07 19:54: forwards to host.Discovered (C91w8c)
//   - 2026-11-11 00:34: says dir is no pattern (C122, B265)
func Discovered(
	dir string,
	glob string,
) ([]string, error) {
	return host.Discovered(
		dir,
		glob,
	)
}

// ListenTemp serves on a socket in a new private directory, with a new random
// token, for a host that has no wish to choose either.
//
// The directory is for this user alone. Close on the returned listener removes
// the directory and the socket, and a second Close is safe.
//
// Returns the listener, which the caller closes. Never panics.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
func ListenTemp() (common.Listener, error) {
	held, err := host.ListenTemp()
	if err != nil {
		return nil, err
	}

	return held, nil
}
