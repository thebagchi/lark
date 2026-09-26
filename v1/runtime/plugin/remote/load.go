package remote

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// SOCKET_ENV and TOKEN_ENV are how a launched plugin is told where to dial
	// and what to present.
	//
	// The environment rather than the command line, because the token is the
	// only thing standing between a local process and putting names into every
	// script compiled against this registry. An argument would be in the
	// process table, readable by anyone on the machine.
	SOCKET_ENV = "LARK_PLUGIN_SOCKET"
	TOKEN_ENV  = "LARK_PLUGIN_TOKEN"

	// WINDOWS is the platform whose binaries carry a different extension.
	WINDOWS = "windows"

	// GLOB is what a plugin directory is searched with when a host names no
	// pattern of its own.
	//
	// A prefix as well as an extension, the way terraform-provider-aws is
	// named: the extension says what kind of artefact a file is, per CLAUDE.md,
	// and the prefix says whose it is. A directory can then hold binaries that
	// are nothing to do with this runtime without any of them being started.
	GLOB         = "lark-*.bin"
	GLOB_WINDOWS = "lark-*.exe"

	// ANNOUNCED is how long a plugin has to say what it supplies before it is
	// taken to have failed. Generous: a process has to start, dial and send
	// one message, which is milliseconds.
	ANNOUNCED = 10 * time.Second

	// LEAVING is how long a plugin is given to exit on its own once the socket
	// it was reading has closed, before it is killed.
	//
	// Asked before it is made to. A plugin holding a file or a connection
	// should get the chance to put it down, and the only thing waiting is a
	// host that is shutting down anyway. Two seconds is what go-plugin allows
	// for the same moment.
	LEAVING = 2 * time.Second

	// _POLL is how often a wait looks.
	_POLL = 5 * time.Millisecond
)

// ErrQuiet is returned for a plugin that started but never announced itself.
var ErrQuiet = errors.New("started but never announced itself")

// Loading is what a Load did.
//
// Skipped is per-plugin: one binary that will not start does not stop the
// others, and a host decides what to say about it. An error from Load itself
// is the directory being unreadable, which is a different kind of wrong - it
// means nothing was even looked at.
type Loading struct {
	Loaded  []string
	Skipped []error
}

// Discovered is every plugin binary in dir matching glob, in a fixed order.
//
// An empty glob means this platform's binary extension. Matching a name rather
// than testing the execute bit is what go-plugin does, and it is enough here:
// a match that will not start is skipped and reported, which is the same path
// a broken binary takes anyway.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
func Discovered(dir string, glob string) ([]string, error) {
	if glob == "" {
		glob = GLOB

		if runtime.GOOS == WINDOWS {
			glob = GLOB_WINDOWS
		}
	}

	held, err := filepath.Glob(filepath.Join(dir, glob))
	if err != nil {
		return nil, fmt.Errorf("searching %s for %s: %w", dir, glob, err)
	}

	// filepath.Glob sorts, so two plugins supplying one name report the same
	// clash on every run rather than whichever the filesystem offered first.
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
// Returns an error only when dir itself could not be read. Never panics.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
func (l *Listener) Load(dir string, glob string) (*Loading, error) {
	found, err := Discovered(dir, glob)
	if err != nil {
		return nil, err
	}

	loading := &Loading{}

	for _, named := range found {
		err := l._Start(named)
		if err != nil {
			loading.Skipped = append(loading.Skipped, err)

			continue
		}

		loading.Loaded = append(loading.Loaded, named)
	}

	return loading, nil
}

// _Start runs one plugin and waits for its names to arrive.
//
// Waits for the count to grow rather than to be more than zero, because this
// listener may already be carrying others.
//
// Revisions:
//   - 2026-09-25 23:55: initial creation
func (l *Listener) _Start(named string) error {
	before := len(l.Names())

	held := exec.Command(named)
	held.Stderr = os.Stderr
	held.Env = append(os.Environ(),
		SOCKET_ENV+"="+l.socket,
		TOKEN_ENV+"="+l.token,
	)

	err := held.Start()
	if err != nil {
		return fmt.Errorf("starting %s: %w", named, err)
	}

	l.guard.Lock()
	l.started = append(l.started, held)
	l.guard.Unlock()

	deadline := time.Now().Add(ANNOUNCED)

	for time.Now().Before(deadline) {
		if len(l.Names()) > before {
			return nil
		}

		time.Sleep(_POLL)
	}

	return fmt.Errorf("%s: %w", named, ErrQuiet)
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
func (l *Listener) _Stop() {
	l.guard.Lock()
	started := l.started
	l.started = nil
	l.guard.Unlock()

	done := make(chan *exec.Cmd, len(started))

	for _, held := range started {
		go func() {
			_ = held.Wait()

			done <- held
		}()
	}

	left := len(started)
	deadline := time.After(LEAVING)

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
