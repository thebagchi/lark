package deep

import (
	"go.starlark.net/starlark"
)

const (
	// WORD is what a value costs before its contents: an interface header and
	// whatever the interpreter keeps beside it. Every value pays this, which is
	// what makes a list of a million empty strings cost something rather than
	// nothing.
	WORD = 16

	// ENTRY is what one slot in a container costs on top of what it holds - a
	// slice element, or a map bucket's share of a key and a value.
	ENTRY = 24

	// NUMBER is what a number costs. An int is arbitrary precision in Starlark
	// and a float is eight bytes, so this is the small case and a very large
	// integer is undercounted. Naming the undercount is better than pretending
	// to measure it: big.Int does not say what it holds.
	NUMBER = 16
)

// Size is roughly what value occupies, for a caller that has to charge for
// holding it.
//
// An estimate, and deliberately a rough one. Go will not say what a value costs
// and Starlark's own types do not either, so this counts what can be counted -
// the bytes in a string, the slots in a container - and adds a fixed word for
// everything else. It is used to decide whether a store may take another value,
// where being within a factor of two is enough and being exact is impossible.
//
// Over-counts rather than under-counts wherever it can, so a budget built on it
// refuses slightly early rather than slightly late. The one place it
// under-counts is an integer too large for a machine word, which is noted on
// NUMBER.
//
// A value that contains itself terminates, the same way Copy and IsData do.
//
// Revisions:
//   - 2026-09-27 01:20: initial creation
//   - 2026-10-29 01:22: builds the seen map only for a container, so a scalar
//     costs no allocation
func Size(value starlark.Value) int64 {
	// A scalar is counted without the map, so the map is built only for what
	// can hold something. A store weighs every value it takes, and most are
	// one number or one word.
	switch value.(type) {
	case *starlark.List, *starlark.Dict, starlark.Tuple, *starlark.Set:
		return _Weigh(
			value,
			map[starlark.Value]bool{},
		)
	}

	return _Weigh(
		value,
		nil,
	)
}

// _Weigh is Size with the set of values already counted, so a cycle ends.
//
// Revisions:
//   - 2026-09-27 01:20: initial creation
func _Weigh(
	value starlark.Value,
	seen map[starlark.Value]bool,
) int64 {
	switch held := value.(type) {
	case starlark.NoneType, starlark.Bool:
		return WORD

	case starlark.Int, starlark.Float:
		return NUMBER

	case starlark.String:
		return WORD + int64(len(held))

	case starlark.Bytes:
		return WORD + int64(len(held))

	case *starlark.List:
		return _Inside(
			held,
			seen,
		)

	case *starlark.Set:
		return _Members(
			held,
			seen,
		)

	case starlark.Tuple:
		return _Slots(
			held,
			seen,
		)

	case *starlark.Dict:
		return _Keyed(
			held,
			seen,
		)
	}

	// Something this does not know, which a store would have refused: one word,
	// so an unknown value is never free.
	return WORD
}

// _Slots is a tuple and everything in it.
//
// Not marked as seen, because a tuple is a Go slice and a slice cannot be a map
// key - the mark would panic rather than prevent anything. It needs no mark
// either: a tuple is immutable and copied by value, so a cycle back to it runs
// through some list or dict on the way, and that one is marked.
//
// Revisions:
//   - 2026-09-27 01:32: initial creation, after marking one panicked
func _Slots(
	held starlark.Tuple,
	seen map[starlark.Value]bool,
) int64 {
	total := WORD + int64(len(held))*ENTRY

	for _, one := range held {
		total += _Weigh(
			one,
			seen,
		)
	}

	return total
}

// _Inside is a list and everything in it.
//
// Read by index rather than through an iterator, which allocates one for
// each list and gives the same elements in the same order.
//
// Revisions:
//   - 2026-09-27 01:20: initial creation
//   - 2026-10-29 01:22: a list only, read by index, so the walk allocates no
//     iterator
func _Inside(
	held *starlark.List,
	seen map[starlark.Value]bool,
) int64 {
	if seen[held] {
		return 0
	}

	seen[held] = true

	total := WORD + int64(held.Len())*ENTRY

	for index := range held.Len() {
		total += _Weigh(
			held.Index(index),
			seen,
		)
	}

	return total
}

// _Members is a set and everything in it.
//
// Through Elements rather than an iterator, which allocates one for each set.
// Both give the members in the order they went in.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation, split from _Inside
func _Members(
	held *starlark.Set,
	seen map[starlark.Value]bool,
) int64 {
	if seen[held] {
		return 0
	}

	seen[held] = true

	total := WORD + int64(held.Len())*ENTRY

	for member := range held.Elements() {
		total += _Weigh(
			member,
			seen,
		)
	}

	return total
}

// _Keyed is a dictionary, its keys and its values.
//
// Through Entries rather than Items, which builds a slice of every pair first,
// and on a dict of 4096 members that slice was all the count allocated.
//
// Revisions:
//   - 2026-09-27 01:20: initial creation
//   - 2026-10-03 17:08: counts the pairs without first building a slice of them
func _Keyed(
	held *starlark.Dict,
	seen map[starlark.Value]bool,
) int64 {
	if seen[held] {
		return 0
	}

	seen[held] = true

	total := WORD + int64(held.Len())*ENTRY

	for key, value := range held.Entries() {
		total += _Weigh(
			key,
			seen,
		)
		total += _Weigh(
			value,
			seen,
		)
	}

	return total
}
