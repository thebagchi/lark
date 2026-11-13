package common

// SOCKET_ENV and TOKEN_ENV are how a launched plugin is told where to dial
// and what to present.
//
// The environment rather than the command line, because the token is the
// only thing standing between a local process and putting names into every
// script compiled against this registry. An argument would be in the
// process table, readable by anyone on the machine.
//
// Revisions:
//   - 2026-09-26 00:31: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved here from remote (C91w8c)
const (
	SOCKET_ENV = "LARK_PLUGIN_SOCKET"
	TOKEN_ENV  = "LARK_PLUGIN_TOKEN"
)

// Loading is what a Load did.
//
// Skipped is per-plugin: one binary that will not start does not stop the
// others, and a host decides what to say about it. An error from Load itself
// is the directory being unreadable, which is a different kind of wrong - it
// means nothing was even looked at.
//
// Revisions:
//   - 2026-09-26 00:31: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved here from remote (C91w8c)
type Loading struct {
	Loaded  []string
	Skipped []error
}

// Listener is the host's socket, and the plugins attached to it.
//
// Made by remote.Listen, which returns an error - this is what replaces init
// for a remote plugin. A blank import cannot fail, cannot wait, and cannot
// close a socket when the host exits; a call from main can do all three.
//
// Close stops serving, kills every plugin the listener started and drops what
// attached. Live is the name of every plugin with a stream open now. Load
// starts every plugin in dir matching glob and waits for each to say what it
// supplies. Names is what every attached plugin announced, for the
// compile-time check. Plugins is what a compiler is given.
//
// The call that a plugin's own stream makes, Register, is not here: only the
// listener's gRPC server calls it, so a host never needs it.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: an interface here; the struct is host.Listener (C91w8c, Q101)
type Listener interface {
	Close() error
	Live() []string
	Load(
		dir string,
		glob string,
	) (*Loading, error)
	Names() []string
	Plugins() []Plugin
}
