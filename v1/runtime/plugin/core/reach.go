package core

import (
	"go.starlark.net/starlark"
)

// _Reach walks a value to every function it can reach, visiting each once.
//
// The function spawn is handed is not the only one a new thread can call. A
// function can be a parameter's default, sit in a list that is one, be what
// another closure captured, or be the receiver of a bound method - and each of
// those runs on the new thread if the one it was handed calls it.
//
// Only the values that can alias or cycle are remembered, and those are
// pointers. A tuple is a slice and cannot be a map key; it also cannot contain
// itself except through one of the others.
type _Reach struct {
	visit func(*starlark.Function) error
	seen  map[starlark.Value]bool
}

// _Walk calls visit with every function value reaches, value included, and
// stops at the first error visit returns.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Walk(value starlark.Value, visit func(*starlark.Function) error) error {
	reach := &_Reach{
		visit: visit,
		seen:  make(map[starlark.Value]bool),
	}

	return reach._Value(value)
}

// _Value walks into one value.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Value(value starlark.Value) error {
	switch held := value.(type) {
	case *starlark.Function:
		return r._Function(held)

	case starlark.Tuple:
		return r._Iterable(held)

	case *starlark.List, *starlark.Set:
		return r._Container(held)

	case *starlark.Dict:
		return r._Dict(held)

	case *starlark.Builtin:
		return r._Receiver(held)
	}

	return nil
}

// _First reports whether value is met for the first time, and remembers it.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _First(value starlark.Value) bool {
	if r.seen[value] {
		return false
	}

	r.seen[value] = true

	return true
}

// _Container walks a list's or a set's elements the first time it is met.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Container(value starlark.Value) error {
	if !r._First(value) {
		return nil
	}

	iterable, ok := value.(starlark.Iterable)
	if !ok {
		return nil
	}

	return r._Iterable(iterable)
}

// _Dict walks a dict's keys and values the first time it is met.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Dict(dict *starlark.Dict) error {
	if !r._First(dict) {
		return nil
	}

	for _, item := range dict.Items() {
		err := r._Iterable(item)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Receiver walks what a bound method is bound to.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Receiver(method *starlark.Builtin) error {
	if method.Receiver() == nil {
		return nil
	}

	return r._Value(method.Receiver())
}

// _Iterable walks every element.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Iterable(iterable starlark.Iterable) error {
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

// _Function visits a function, then walks what its defaults and its captured
// variables hold.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func (r *_Reach) _Function(fn *starlark.Function) error {
	if !r._First(fn) {
		return nil
	}

	err := r.visit(fn)
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
