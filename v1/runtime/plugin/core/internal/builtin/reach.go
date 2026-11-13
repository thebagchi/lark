package builtin

import (
	"context"
	"fmt"
	"sync"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _NoReach is the error of a walk asked to go on with nothing to go on with: a
// nil walk, a nil function, a nil dict, a nil method, a nil iterable, or a
// function to visit with no visit to call. It wraps scheduler.ErrRun.
//
// Revisions:
//   - 2026-11-08 13:25: initial creation, ErrNoReach merged into ErrNoRun (C98, Q249)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func _NoReach() error {
	return fmt.Errorf(
		"nothing to walk: %w",
		scheduler.ErrRun,
	)
}

// Reach walks a value to every function it can reach, visiting each once.
//
// The function spawn is handed is not the only one a new thread can call. A
// function can be a parameter's default, sit in a list that is one, be what
// another closure captured, or be the receiver of a bound method - and each of
// those runs on the new thread if the one it was handed calls it.
//
// Only the values that can alias or cycle are remembered, and those are
// pointers. A tuple is a slice and cannot be a map key; it also cannot contain
// itself except through one of the others.
//
// path counts the depth, because a script can build a default argument nested
// a million deep in a few lines, and the walk would overflow the stack of Go.
// The walk keeps seen for the cycles, so path does not look for them.
//
// One Reach belongs to one walk on one goroutine: _Walk makes it, and nothing
// else holds it. The mutex is not for a use, it is for a misuse: a caller that
// shares one anyway cannot make seen or path race. The methods of Reach take
// the mutex once, at the entry, and run the walk on a Descent, which is the
// same value with no lock. A recursion therefore never takes the mutex twice,
// and a walk pays for one lock, not one per element. A Reach made with new has
// no visit, no seen and no path; seen and path are made on first use.
//
// Revisions:
//   - 2026-09-30 20:58: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Reach to Reach; renamed
//     the fields seen to Seen, path to Path, for the tests in tests/ (C120b, Q315, Q314)
type Reach struct {
	mutex sync.Mutex
	visit func(*starlark.Function) error
	Seen  map[starlark.Value]bool
	Path  *common.Trail
}

// Descent is a Reach seen from inside a walk that already holds its mutex.
// Every method on it assumes the caller holds that mutex.
//
// Revisions:
//   - 2026-11-03 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Descent to Descent, for
//     the tests in tests/ (C120b, Q315, Q314)
type Descent Reach

// _Walk calls visit with every function value reaches, value included, and
// stops at the first error visit returns.
//
// A value nested past common.DEPTH is trail.ErrDeep, which ends the run.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 12:28: takes the context of the walk, and counts the depth (C26)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func _Walk(
	ctx context.Context,
	value starlark.Value,
	visit func(*starlark.Function) error,
) error {
	reach := &Reach{
		visit: visit,
		Seen:  make(map[starlark.Value]bool),
		Path:  trail.Bounded(ctx),
	}

	return reach.Value(value)
}

// Value walks into one value, with the mutex held for the whole walk. A nil
// receiver is scheduler.ErrRun.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a shared _Reach cannot race (C52, B062, B063)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Value to Value, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (r *Reach) Value(value starlark.Value) error {
	if r == nil {
		return _NoReach()
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return (*Descent)(r)._Value(value)
}

// First reports whether value is met for the first time, with the mutex held.
// A nil receiver meets nothing for the first time.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a shared _Reach cannot race (C52, B060, B063, B069)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _First to First, for the
//     tests in tests/ (C120b, Q315, Q314)
func (r *Reach) First(value starlark.Value) bool {
	if r == nil {
		return false
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return (*Descent)(r)._First(value)
}

// Container walks a list's or a set's elements, with the mutex held for the
// whole walk. A nil receiver is scheduler.ErrRun.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a shared _Reach cannot race (C52, B063)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Container to Container,
//     for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (r *Reach) Container(value starlark.Value) error {
	if r == nil {
		return _NoReach()
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return (*Descent)(r)._Container(value)
}

// Iterable walks every element, with the mutex held for the whole walk. A nil
// receiver is scheduler.ErrRun.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a shared _Reach cannot race (C52, B064)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Iterable to Iterable,
//     for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (r *Reach) Iterable(iterable starlark.Iterable) error {
	if r == nil {
		return _NoReach()
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return (*Descent)(r)._Iterable(iterable)
}

// Function visits a function and walks what it holds, with the mutex held for
// the whole walk. A nil receiver is scheduler.ErrRun.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a shared _Reach cannot race (C52, B060, B063)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Function to Function,
//     for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (r *Reach) Function(fn *starlark.Function) error {
	if r == nil {
		return _NoReach()
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return (*Descent)(r).Function(fn)
}

// Tuple walks a tuple's elements. A tuple is a slice and cannot be on a path
// of pointers, so it enters the trail as nil.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-03 21:57: runs on a _Descent, and takes its path from _Path (C52, B062, B063)
//   - 2026-11-04 00:30: a nil receiver is ErrNoReach (C54, B080)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Tuple to Tuple, for the
//     tests in tests/ (C120b, Q315, Q314)
func (r *Descent) Tuple(tuple starlark.Tuple) error {
	if r == nil {
		return _NoReach()
	}

	err := trail.Enter(
		r._Path(),
		nil,
	)
	if err != nil {
		return err
	}

	defer trail.Leave(r._Path())

	return r._Iterable(tuple)
}

// Dict walks a dict's keys and values the first time it is met.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 12:28: enters the trail, and walks the pairs through Entries
//     so a pair does not count as a level of depth (C26)
//   - 2026-11-03 21:57: a nil dict is ErrNoReach (C52, B062)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Dict to Dict, for the
//     tests in tests/ (C120b, Q315, Q314)
func (r *Descent) Dict(dict *starlark.Dict) error {
	if dict == nil {
		return _NoReach()
	}

	if !r._First(dict) {
		return nil
	}

	err := trail.Enter(
		r._Path(),
		dict,
	)
	if err != nil {
		return err
	}

	defer trail.Leave(r._Path())

	for key, value := range dict.Entries() {
		err = r._Value(key)
		if err != nil {
			return err
		}

		err = r._Value(value)
		if err != nil {
			return err
		}
	}

	return nil
}

// Receiver walks what a bound method is bound to.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-03 21:57: a nil method is ErrNoReach (C52, B062)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Receiver to Receiver,
//     for the tests in tests/ (C120b, Q315, Q314)
func (r *Descent) Receiver(method *starlark.Builtin) error {
	if method == nil {
		return _NoReach()
	}

	if method.Receiver() == nil {
		return nil
	}

	return r._Value(method.Receiver())
}

// Function visits a function, then walks what its defaults and its captured
// variables hold.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-03 21:57: a nil function or no visit is ErrNoReach (C52, B060)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Function to Function,
//     for the tests in tests/ (C120b, Q315, Q314)
func (r *Descent) Function(fn *starlark.Function) error {
	if fn == nil || r.visit == nil {
		return _NoReach()
	}

	if !r._First(fn) {
		return nil
	}

	err := trail.Enter(
		r._Path(),
		fn,
	)
	if err != nil {
		return err
	}

	defer trail.Leave(r._Path())

	err = r.visit(fn)
	if err != nil {
		return err
	}

	for idx := range fn.NumParams() {
		fallback := fn.ParamDefault(idx)
		if fallback == nil {
			continue
		}

		err = r._Value(fallback)
		if err != nil {
			return err
		}
	}

	for idx := range fn.NumFreeVars() {
		_, held := fn.FreeVar(idx)
		if held == nil {
			continue
		}

		err = r._Value(held)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Value walks into one value.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 12:28: a value that holds nothing counts as an element (C26)
//   - 2026-11-03 21:57: ticks a path made on first use (C52, B062)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func (r *Descent) _Value(value starlark.Value) error {
	switch held := value.(type) {
	case *starlark.Function:
		return r.Function(held)

	case starlark.Tuple:
		return r.Tuple(held)

	case *starlark.List, *starlark.Set:
		return r._Container(held)

	case *starlark.Dict:
		return r.Dict(held)

	case *starlark.Builtin:
		return r.Receiver(held)
	}

	return trail.Tick(r._Path())
}

// _Path returns the path of the walk, making it on first use.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation, so a _Reach made with new can walk (C52, B062)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func (r *Descent) _Path() *common.Trail {
	if r.Path == nil {
		r.Path = trail.Bounded(context.Background())
	}

	return r.Path
}

// _First reports whether value is met for the first time, and remembers it.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-03 21:57: makes seen on first use (C52, B060, B069)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func (r *Descent) _First(value starlark.Value) bool {
	if r.Seen == nil {
		r.Seen = make(map[starlark.Value]bool)
	}

	if r.Seen[value] {
		return false
	}

	r.Seen[value] = true

	return true
}

// _Container walks a list's or a set's elements the first time it is met.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-03 21:57: runs on a _Descent, and takes its path from _Path (C52, B062, B063)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func (r *Descent) _Container(value starlark.Value) error {
	if !r._First(value) {
		return nil
	}

	err := trail.Enter(
		r._Path(),
		value,
	)
	if err != nil {
		return err
	}

	defer trail.Leave(r._Path())

	iterable, ok := value.(starlark.Iterable)
	if !ok {
		return nil
	}

	return r._Iterable(iterable)
}

// _Iterable walks every element.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-03 21:57: a nil iterable is ErrNoReach (C52, B064)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func (r *Descent) _Iterable(iterable starlark.Iterable) error {
	if iterable == nil {
		return _NoReach()
	}

	iter := iterable.Iterate()
	defer iter.Done()

	var elem starlark.Value

	for iter.Next(&elem) {
		err := r._Value(elem)
		if err != nil {
			return err
		}
	}

	return nil
}
