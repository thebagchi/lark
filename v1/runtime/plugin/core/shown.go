package core

import (
	"context"
	"errors"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Shown is the builtin that stands for one of go.starlark.net's builtins that
// write a value as text: str, repr, print and fail.
//
// The library writes a tuple by recursing in Go, with no depth bound, so a
// script that nests a tuple a few million deep ends the process with a stack
// overflow, which nothing can recover (B009). The builtin walks its arguments
// first, through the walker every other guard uses, and refuses a value nested
// past common.DEPTH with trail.ErrDeep, after its name. Anything shallower
// goes to the library's own builtin, so the text is the library's, byte for
// byte.
//
// The universe builtin is read when this is called, not copied, so the library
// stays the one place that knows how a value is written.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-03 06:29: also stands for fail (C47, Q61)
//   - 2026-11-04 21:49: built by shared.Guarded, which names the builtin in the error (C59)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func _Shown(name string) *starlark.Builtin {
	return shared.Guarded(
		starlark.Universe[name].(*starlark.Builtin),
		func(
			thread *starlark.Thread,
			args starlark.Tuple,
		) error {
			return _Checked(
				scheduler.Ambient(thread),
				args,
			)
		},
	)
}

// _Checked walks every argument that holds other values, and returns
// trail.ErrDeep for one nested past common.DEPTH. It returns the error of
// the context when the run is cancelled during a long walk.
//
// The walk goes twice at most. The first walk looks at the depth only: it
// keeps no path, so it costs one step for each element. A container that holds
// itself looks deeper than the limit to that walk, so a value past the limit
// is walked again with the cycle check, which skips the cycle. Only a value
// that is past the limit, or holds itself, pays for the second walk.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func _Checked(
	ctx context.Context,
	args starlark.Tuple,
) error {
	err := _Walked(
		ctx,
		trail.Bounded,
		args,
	)
	if !errors.Is(
		err,
		trail.ErrDeep,
	) {
		return err
	}

	return _Walked(
		ctx,
		trail.New,
		args,
	)
}

// _Walked walks the arguments that hold other values with a trail made by
// made. An argument that holds nothing, such as a number or a string, costs a
// type switch and no allocation: the trail is made when the first container is
// met.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _Walked(
	ctx context.Context,
	made func(context.Context) *common.Trail,
	args starlark.Tuple,
) error {
	var path *common.Trail

	for _, arg := range args {
		if !_Holds(arg) {
			continue
		}

		if path == nil {
			path = made(ctx)
		}

		err := _Down(
			path,
			arg,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Holds reports whether a value can hold other values, so it can nest.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _Holds(value starlark.Value) bool {
	switch value.(type) {
	case *starlark.List, starlark.Tuple, *starlark.Dict, *starlark.Set:
		return true
	}

	return false
}

// _Down walks one value and what it holds.
//
// A container that holds itself is not an error here: the library writes it as
// "...", so the walk does not go into it again. The cycle is what stops the
// recursion, so only a chain of different containers reaches the limit.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _Down(
	path *common.Trail,
	value starlark.Value,
) error {
	switch held := value.(type) {
	case *starlark.List:
		return _InList(
			path,
			held,
		)
	case *starlark.Set:
		return _InSet(
			path,
			held,
		)
	case *starlark.Dict:
		return _InDict(
			path,
			held,
		)
	case starlark.Tuple:
		return _InTuple(
			path,
			held,
		)
	}

	return trail.Tick(path)
}

// _Entered enters a container on the trail. It reports false, with no error,
// for a container already on the path: the library writes that one as "...".
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func _Entered(
	path *common.Trail,
	container any,
) (bool, error) {
	err := trail.Enter(
		path,
		container,
	)
	if errors.Is(
		err,
		trail.ErrCyclic,
	) {
		return false, nil
	}

	return err == nil, err
}

// _InList walks a list's elements.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _InList(
	path *common.Trail,
	list *starlark.List,
) error {
	entered, err := _Entered(
		path,
		list,
	)
	if !entered {
		return err
	}

	defer trail.Leave(path)

	for index := range list.Len() {
		err = _Down(
			path,
			list.Index(index),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _InTuple walks a tuple's elements. A tuple is a slice and cannot be on a path
// of pointers, so it enters the trail as nil.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _InTuple(
	path *common.Trail,
	tuple starlark.Tuple,
) error {
	_, err := _Entered(
		path,
		nil,
	)
	if err != nil {
		return err
	}

	defer trail.Leave(path)

	for _, element := range tuple {
		err = _Down(
			path,
			element,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _InSet walks a set's members.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _InSet(
	path *common.Trail,
	set *starlark.Set,
) error {
	entered, err := _Entered(
		path,
		set,
	)
	if !entered {
		return err
	}

	defer trail.Leave(path)

	iterator := set.Iterate()
	defer iterator.Done()

	var member starlark.Value

	for iterator.Next(&member) {
		err = _Down(
			path,
			member,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _InDict walks a dict's keys and values. The pairs go through Entries, which
// builds no slice of them.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
func _InDict(
	path *common.Trail,
	dict *starlark.Dict,
) error {
	entered, err := _Entered(
		path,
		dict,
	)
	if !entered {
		return err
	}

	defer trail.Leave(path)

	for key, value := range dict.Entries() {
		err = _Down(
			path,
			key,
		)
		if err != nil {
			return err
		}

		err = _Down(
			path,
			value,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
