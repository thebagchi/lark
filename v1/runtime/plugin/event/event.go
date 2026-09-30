// Package event gives a script the event module. Importing it is what enables
// it.
//
// One thread says something happened; another waits until it has.
//
//	event.post("loaded", {"rows": 12})
//
//	value, err = event.wait("loaded", 5)
//	if err:
//	    fail(err)
//
// An event is a latch. It is posted once and stays posted, so a waiter arriving
// afterwards does not wait at all, and every waiter sees it. A signal that only
// woke whoever happened to be waiting would lose a post that arrived first,
// which is a deadlock that depends on scheduling - the worst kind to find.
//
// A module rather than two top-level names, so a host that does not want a
// script able to block on another thread leaves the import out. Nothing bounds
// how long a wait lasts except the timeout the script itself chose.
package event

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// NAME is the module, and the names it holds.
	NAME = "event"
	POST = "post"
	WAIT = "wait"

	// EXPIRED is what a wait that ran out puts in the second half of its
	// answer. A message rather than a bool, because it is what a script prints
	// or fails with.
	EXPIRED = "timed out"
)

var (
	// ErrPosted is returned for posting an event that has already been posted.
	//
	// A latch says a thing happened, and a thing happens once. Posting twice is
	// either two different things sharing a name or the same thing reported
	// twice, and both are mistakes a script author would rather hear about.
	ErrPosted = errors.New("this event has already been posted")

	// ErrNotData is returned for a value an event cannot carry, for the reason
	// a store cannot hold one: another thread reads it, and code means nothing
	// to whoever did not write it.
	ErrNotData = errors.New("not data an event can carry")
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func init() {
	plugin.Register(&_Event{})
}

// _Event is the plugin. Empty: the events themselves belong to a run, not to
// the plugin, because two runs in one process must not see each other's.
type _Event struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func (e *_Event) Name() string {
	return NAME
}

// Values returns the event module.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func (e *_Event) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				POST: starlark.NewBuiltin(NAME+"."+POST, _Post),
				WAIT: starlark.NewBuiltin(NAME+"."+WAIT, _Wait),
			},
		},
	}
}

// _Post says the event named has happened, and hands every waiter the value.
//
// Takes a value rather than defaulting one, because state.set does and an event
// carrying nothing is a thing a script can say with None.
//
// Returns ErrPosted for a second post of one name, ErrNotData for a value
// another thread could not read, and ErrMemory when the run cannot afford to
// hold it. Answers with None, as state.set does. Never panics.
//
// The first two stop the whole run, the way a failed assertion does, rather
// than only the thread that posted. A thread nobody joins fails silently - its
// error reaches the report and never becomes the run's result - so a spawned
// worker posting twice, or posting a function, would otherwise say nothing, and
// whoever waited for it would see only a timeout. The mistake is in the script,
// and a script author wants to hear about it.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
//   - 2026-09-30 21:55: a second post, or one that is not data, stops the whole
//     run rather than the thread
func _Post(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name  string
		value starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &name, &value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	bad, ok := deep.IsData(value)
	if !ok {
		return nil, scheduler.Fail(thread, fmt.Errorf(
			"%s %q: %s: %w", fn.Name(), name, bad.Type(), ErrNotData))
	}

	events, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	value.Freeze()

	err = events._Post(scheduler.Allowance(thread), name, value)

	// A second post is a mistake in the script rather than a limit it reached,
	// so it stops the run as a value that is not data does. The budget refusing
	// fails only the thread, as it does for the store.
	if errors.Is(err, ErrPosted) {
		return nil, scheduler.Fail(thread, fmt.Errorf("%s %q: %w", fn.Name(), name, err))
	}

	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	return starlark.None, nil
}

// _Wait waits until the event named has been posted, and answers with the pair
// (value, err).
//
//	value, err = event.wait("loaded", 5)
//
// err is None when the event was posted and a message when the wait ran out. A
// pair rather than a raise, because running out of time is an answer a script
// acts on rather than a fault - and rather than the value alone, because an
// event posted as None and a wait that expired would otherwise look the same.
//
// Waits on the caller's context as well as the clock, which every blocking
// builtin owes: a timeout around this, or an interrupt, has to reach it.
//
// Raises rather than answering when the caller was cancelled, because then there
// is no script left to read the pair. Never panics.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func _Wait(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name    string
		seconds starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &name, &seconds)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	bound, err := scheduler.Duration(seconds)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	events, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	held, err := events._Named(scheduler.Allowance(thread), name)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	value, expired, err := held._Await(ctx, bound)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	if expired {
		return _Answer(starlark.None, starlark.String(EXPIRED)), nil
	}

	return _Answer(value, starlark.None), nil
}

// _Answer is the pair a wait answers with.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func _Answer(value starlark.Value, failed starlark.Value) starlark.Value {
	return starlark.Tuple{value, failed}
}
