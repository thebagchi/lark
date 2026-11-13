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
//
// Revisions:
//   - 2026-09-30 21:12: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package event

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/plugin/event/internal/latch"
	"github.com/thebagchi/lark/v1/runtime/plugin/event/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrPosted is latch.ErrPosted.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrPosted = latch.ErrPosted
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
//
// Revisions:
//   - 2026-09-30 21:12: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Event struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (e *_Event) Name() string {
	return spec.NAME
}

// Values returns the event module.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (e *_Event) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.POST: starlark.NewBuiltin(
					spec.NAME+"."+spec.POST,
					_Post,
				),
				spec.WAIT: starlark.NewBuiltin(
					spec.NAME+"."+spec.WAIT,
					_Wait,
				),
			},
		},
	}
}

// Check refuses a post of something the source already shows is not data.
//
// Only what is visible, as the store's check is: event.post("k", helper) names
// a function this file declares, and a lambda is one written in place - both
// certain before anything runs, and a script author would rather hear it then
// than from a run that stops. Everything else is refused when it runs, by the
// same rule.
//
// This is a Checking, which the compiler asks every plugin for; it does not
// ask about a file that has taken the name event for itself.
//
// Revisions:
//   - 2026-09-30 22:41: initial creation
//   - 2026-11-04 19:30: refuses a source it cannot read (C87 fix round 1)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func (e *_Event) Check(tree *syntax.File) error {
	call, named, err := deep.ShowsCode(
		tree,
		spec.NAME,
		spec.POST,
	)
	if err != nil {
		return err
	}

	if call == nil {
		return nil
	}

	return fmt.Errorf(
		"%s.%s at %s posts %s: not data an event can carry: %w",
		spec.NAME,
		spec.POST,
		call.Lparen,
		named,
		unpack.ErrConvert,
	)
}

// _Post says the event named has happened, and hands every waiter the value.
//
// Takes a value rather than defaulting one, because state.set does and an event
// carrying nothing is a thing a script can say with None.
//
// Returns ErrPosted for a second post of one name, and unpack.ErrConvert for a value
// another thread could not read. Answers with None, as state.set does. Never
// panics.
//
// Both stop the whole run, the way a failed assertion does, rather
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
//   - 2026-11-01 13:00: no longer refuses for memory
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 11:32: uses latch.PostArgs, which moved to internal/latch (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Post(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	name, value, err := latch.PostArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	err = _PostData(
		thread,
		fn,
		name,
		value,
	)
	if err != nil {
		return nil, err
	}

	return _Posted(
		thread,
		fn,
		name,
		value,
	)
}

// _Posted stores the event, and stops the run when it was already posted.
//
// A second post is a mistake in the script rather than a limit it reached.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-01 13:00: the only refusal left is a second post
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 11:32: uses names that moved to internal/latch and are exported there (C120b,
//     Q315)
func _Posted(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	name string,
	value starlark.Value,
) (starlark.Value, error) {
	events, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	value.Freeze()

	err = events.Post(
		name,
		value,
	)
	if err != nil {
		return nil, scheduler.Fail(
			thread,
			fmt.Errorf(
				"%s %s: %w",
				shared.Called(fn),
				unpack.QuotePrefix(name),
				err,
			),
		)
	}

	return starlark.None, nil
}

// _Wait waits until the event named has been posted, for at most the number of
// milliseconds given, and answers with the pair (value, err).
//
//	value, err = event.wait("loaded", 5000)
//
// Milliseconds because every duration a script states is, read by the one
// reader sleep and timeout use.
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
//   - 2026-10-02 01:41: reads its bound in milliseconds, as every duration is
//   - 2026-10-24 16:05: refuses while a name is held, as join does, so a wait
//     inside an update cannot deadlock against the thread that would post
//   - 2026-11-10 11:32: uses latch.WaitBound and latch.NotUpdating, which moved to
//     internal/latch (C120b, Q315)
//   - 2026-11-13 01:24: uses NotUpdating, which moved to plugin/internal/common (C131, Q417)
func _Wait(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	name, bound, err := latch.WaitBound(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	err = shared.NotUpdating(
		thread,
		fn,
	)
	if err != nil {
		return nil, err
	}

	return _Waited(
		thread,
		fn,
		name,
		bound,
	)
}

// _Waited waits on the named event and answers with the pair the script reads.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-01 13:00: takes the named latch without a charge
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 11:32: uses names that moved to internal/latch and are exported there (C120b,
//     Q315)
func _Waited(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	name string,
	bound time.Duration,
) (starlark.Value, error) {
	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	events, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return _WaitAnswer(
		ctx,
		&_WaitCall{
			fn:    fn,
			name:  name,
			held:  events.Named(name),
			bound: bound,
		},
	)
}

// _WaitCall is the name, the latch, and the clock bound a wait shares.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 11:32: uses latch.Latch, which moved to internal/latch (C120b, Q315)
type _WaitCall struct {
	fn    *starlark.Builtin
	name  string
	held  *latch.Latch
	bound time.Duration
}

// _WaitAnswer waits out the bound and turns the result into the pair.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses names that moved to internal/latch and are exported there (C120b,
//     Q315)
func _WaitAnswer(
	ctx context.Context,
	call *_WaitCall,
) (starlark.Value, error) {
	value, expired, err := call.held.Await(
		ctx,
		call.bound,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s %s: %w",
			shared.Called(call.fn),
			unpack.QuotePrefix(call.name),
			err,
		)
	}

	if expired {
		return _Answer(
			starlark.None,
			starlark.String(spec.EXPIRED),
		), nil
	}

	return _Answer(
		value,
		starlark.None,
	), nil
}

// _PostData refuses a value the store would not keep, and stops the run.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 12:28: a value nested past common.DEPTH is an error that ends
//     the run (C26)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _PostData(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	name string,
	value starlark.Value,
) error {
	bad, ok, err := deep.IsData(
		scheduler.Ambient(thread),
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"%s %s: %w",
			shared.Called(fn),
			unpack.QuotePrefix(name),
			err,
		)
	}

	if ok {
		return nil
	}

	return scheduler.Fail(
		thread,
		fmt.Errorf(
			"%s %s: %s: not data an event can carry: %w",
			shared.Called(fn),
			unpack.QuotePrefix(name),
			bad.Type(),
			unpack.ErrConvert,
		),
	)
}

// _Answer is the pair a wait answers with.
//
// Revisions:
//   - 2026-09-30 21:00: initial creation
func _Answer(
	value starlark.Value,
	failed starlark.Value,
) starlark.Value {
	return starlark.Tuple{value, failed}
}
