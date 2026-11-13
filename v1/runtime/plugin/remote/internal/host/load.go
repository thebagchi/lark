package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// WINDOWS is the platform whose binaries carry a different extension.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	WINDOWS = "windows"

	// GLOB is what a plugin directory is searched with when a host names no
	// pattern of its own.
	//
	// A prefix as well as an extension, the way terraform-provider-aws is
	// named: the extension says what kind of artefact a file is, per CLAUDE.md,
	// and the prefix says whose it is. A directory can then hold binaries that
	// are nothing to do with this runtime without any of them being started.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	GLOB = "lark-*.bin"

	// GLOB_WINDOWS is GLOB for a platform whose binaries end in .exe.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote; own comment (C91w8c, Q228)
	GLOB_WINDOWS = "lark-*.exe"

	// ANNOUNCED is how long a plugin has to say what it supplies before it is
	// taken to have failed. Generous: a process has to start, dial and send
	// one message, which is milliseconds.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	ANNOUNCED = 10 * time.Second

	// LEAVING is how long a plugin is given to exit on its own once the socket
	// it was reading has closed, before it is killed.
	//
	// Asked before it is made to. A plugin holding a file or a connection
	// should get the chance to put it down, and the only thing waiting is a
	// host that is shutting down anyway. Two seconds is what go-plugin allows
	// for the same moment.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	LEAVING = 2 * time.Second

	// POLL is how often a wait looks.
	//
	// Revisions:
	//   - 2026-09-26 00:31: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
	POLL = 5 * time.Millisecond
)

// ErrQuiet is returned for a plugin that started but never announced itself.
//
// Revisions:
//   - 2026-09-26 00:31: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
var (
	ErrQuiet = errors.New("started but never announced itself")
)

// Discovered is every plugin binary in dir matching glob, in a fixed order.
//
// An empty glob means this platform's binary extension. Matching a name rather
// than testing the execute bit is what go-plugin does, and it is enough here:
// a match that will not start is skipped and reported, which is the same path
// a broken binary takes anyway.
//
// Only glob is a pattern. dir is the directory as it is written, so a name
// holding [, * or ? is that directory (B265): the names in it are listed and
// each is matched against glob. A glob is matched against a name, so one with a
// path separator in it matches nothing. A directory that cannot be read has no
// match, as with filepath.Glob; Load says why.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-10 16:05: cuts the directory and pattern in the error to common.QUOTE_LIMIT bytes
//     (C117, B239, Q4)
//   - 2026-11-11 00:34: lists dir and matches the names, so dir is no pattern (C122, B265)
func Discovered(
	dir string,
	glob string,
) ([]string, error) {
	if glob == "" {
		glob = GLOB

		if runtime.GOOS == WINDOWS {
			glob = GLOB_WINDOWS
		}
	}

	// filepath.Glob checks the pattern before it looks at a name, and so does
	// this: a bad glob is an error for an empty directory too.
	_, err := filepath.Match(
		glob,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf(
			"searching %s for %s: %w",
			unpack.ShortText(dir),
			unpack.ShortText(glob),
			err,
		)
	}

	listed := dir
	if listed == "" {
		listed = "."
	}

	// ReadDir sorts by name, so two plugins supplying one name report the same
	// clash on every run rather than whichever the filesystem offered first.
	entries, err := os.ReadDir(listed)
	if err != nil {
		return nil, nil
	}

	var held []string

	for _, entry := range entries {
		matched, err := filepath.Match(
			glob,
			entry.Name(),
		)
		if err != nil || !matched {
			continue
		}

		held = append(
			held,
			filepath.Join(
				dir,
				entry.Name(),
			),
		)
	}

	return held, nil
}

// Load starts every plugin in dir matching glob and waits for each to say what
// it supplies.
//
// The host starts them, so it decides what code runs; the plugin still dials
// back, so nothing has to reach it and there is no port and no address to
// know. Each is told where to dial and what to present through its
// environment.
//
// One that will not start, or starts and stays quiet, is skipped rather than
// fatal - a host that wants it to be fatal has the list and can say so.
//
// Returns an error only when dir itself could not be read, and io.EOF for a nil
// or zero Listener, as Register does, and for a Listener that Close has closed,
// whether before the call or while it waits for a quiet plugin. Never panics.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
//   - 2026-11-04 23:50: a nil or zero Listener is refused, since there is no
//     socket to hand a plugin (C86 fix round 3)
//   - 2026-11-04 23:50: the refusal is io.EOF, as Register's, with no new
//     sentinel (Q110, C86 fix round 4)
//   - 2026-11-07 19:54: moved to package host from remote; returns common.Loading (C91w8c)
//   - 2026-11-08 10:43: returns io.EOF once the listener is closed (C109, Q253)
//   - 2026-11-10 11:50: io.EOF for a listener closed before the call, and an
//     error for a dir that could not be read, as this doc says (C117, B242, B243)
func (l *Listener) Load(
	dir string,
	glob string,
) (*common.Loading, error) {
	if l == nil || l.server == nil || l._Closed() {
		return nil, io.EOF
	}

	found, err := Discovered(
		dir,
		glob,
	)
	if err != nil {
		return nil, err
	}

	// filepath.Glob answers nothing for a directory it cannot read, so an
	// empty answer is checked: it is a directory with no plugin, or no
	// directory (B243).
	if len(found) == 0 {
		_, err = os.ReadDir(dir)
		if err != nil {
			return nil, unpack.ShortPath(err)
		}
	}

	loading := &common.Loading{}

	for _, named := range found {
		err := l._Start(named)
		if err != nil {
			if l._Closed() {
				return nil, io.EOF
			}

			loading.Skipped = append(
				loading.Skipped,
				err,
			)

			continue
		}

		loading.Loaded = append(
			loading.Loaded,
			named,
		)
	}

	return loading, nil
}

// _Start runs one plugin and waits for it to be answering.
//
// Waits on Live rather than on Names, and the difference only shows when a
// plugin comes back. A restart adds no name: the adapter outlives the process,
// so its names were already there and only the connection is new. Watching the
// name count meant a second Load sat out the whole of ANNOUNCED and then
// reported a plugin that was already answering as one that never announced
// itself.
//
// Live grows either way. A plugin that died is not live, so its replacement
// takes the count from n to n+1 exactly as a plugin nobody had seen before does.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
//   - 2026-09-30 06:47: waits on Live, so a restarted plugin is seen rather than
//     timed out
//   - 2026-10-24 16:05: kills a quiet process before returning, so it does not
//     survive past the Load call that failed
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:43: returns io.EOF when the listener closes; Close's _Stop then
//     owns the process, so Wait runs once (C109, Q253)
//   - 2026-11-10 16:05: cuts the plugin path and its error in the starting error to
//     common.QUOTE_LIMIT bytes (C117, B239, Q4)
func (l *Listener) _Start(named string) error {
	if l._Closed() {
		return io.EOF
	}

	var (
		before = len(l.Live())
		held   = exec.Command(named)
	)

	held.Stderr = os.Stderr

	held.Env = append(
		os.Environ(),
		common.SOCKET_ENV+"="+l.socket,
		common.TOKEN_ENV+"="+l.token,
	)

	err := held.Start()
	if err != nil {
		return fmt.Errorf(
			"starting %s: %w",
			unpack.ShortText(named),
			unpack.ShortPath(err),
		)
	}

	l.guard.Lock()

	if l._Closed() {
		// Close has read started already, so nobody else will see off held.
		l.guard.Unlock()

		_ = held.Process.Kill()
		_ = held.Wait()

		return io.EOF
	}

	l.started = append(
		l.started,
		held,
	)
	l.guard.Unlock()

	deadline := time.Now().Add(ANNOUNCED)

	for time.Now().Before(deadline) {
		if len(l.Live()) > before {
			return nil
		}

		select {
		case <-l.closed:
			// _Stop sees off held, as it does every started plugin.
			return io.EOF

		case <-time.After(POLL):
		}
	}

	return l._Quiet(
		named,
		held,
	)
}

// _Closed reports whether Close has been called.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q253)
func (l *Listener) _Closed() bool {
	select {
	case <-l.closed:
		return true

	default:
		return false
	}
}

// _Quiet kills a process that started and never announced itself.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-10 16:05: cuts the plugin path in the error to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
func (l *Listener) _Quiet(
	named string,
	held *exec.Cmd,
) error {
	_ = held.Process.Kill()
	_ = held.Wait()

	l.guard.Lock()
	l.started = _Without(
		l.started,
		held,
	)
	l.guard.Unlock()

	return fmt.Errorf(
		"%s: %w",
		unpack.ShortText(named),
		ErrQuiet,
	)
}

// _Without is started with held removed.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func _Without(
	started []*exec.Cmd,
	held *exec.Cmd,
) []*exec.Cmd {
	kept := make(
		[]*exec.Cmd,
		0,
		len(started),
	)

	for _, cmd := range started {
		if cmd != held {
			kept = append(
				kept,
				cmd,
			)
		}
	}

	return kept
}

// _Stop sees off every plugin this listener started.
//
// A plugin outliving its host is a process nobody is waiting for, so they go
// when it does. Asked before made to: the caller has already stopped serving,
// so each one is reading a closed stream and will exit on its own, and one that
// has a file or a connection open should get the chance to put it down. Only
// what is still there after LEAVING is killed.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
//   - 2026-09-26 00:12: waits before it kills, as go-plugin does
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
func (l *Listener) _Stop() {
	l.guard.Lock()
	started := l.started
	l.started = nil
	l.guard.Unlock()

	done := make(
		chan *exec.Cmd,
		len(started),
	)

	for _, held := range started {
		go func() {
			_ = held.Wait()

			done <- held
		}()
	}

	var (
		left     = len(started)
		deadline = time.After(LEAVING)
	)

	for left > 0 {
		select {
		case <-done:
			left--

		case <-deadline:
			// Whatever is still running stopped listening. Killing it wakes
			// the Wait above, which is what lets this return.
			for _, held := range started {
				_ = held.Process.Kill()
			}

			for range left {
				<-done
			}

			return
		}
	}
}
