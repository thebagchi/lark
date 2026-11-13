package patch

import (
	"errors"
	"fmt"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// Apply applies a list of RFC 6902 operations and returns a new document.
//
// The input is never mutated. Operations apply in order, and the first failure
// stops the patch - a half-applied document is not a document anybody asked
// for.
//
// Every operation goes through one edit, so a container of the input is copied
// once however many operations write under it; thirty-two replaces in a dict
// of 4096 members used to copy the dict thirty-two times.
//
// Revisions:
//   - 2026-09-20 01:14: initial creation
//   - 2026-10-03 16:53: applies every operation through one edit
//   - 2026-11-02 12:28: the edit knows the thread, for the context of a copy (C26)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _PatchJSON to Apply,
//     for the tests in tests/ (C120b, Q315, Q314)
func Apply(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, patch, err := Unpack(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	edit := _NewEdit(thread)

	for index := range patch.Len() {
		op, ok := patch.Index(index).(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf(
				"%s: operation %d is %s: unknown patch operation: %w",
				shared.Called(fn),
				index,
				patch.Index(index).Type(),
				ErrOperation,
			)
		}

		doc, err = edit._Patch(
			doc,
			op,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: operation %d: %w",
				shared.Called(fn),
				index,
				err,
			)
		}
	}

	return doc, nil
}

// Unpack reads the document and the list of operations.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _PatchArgs to Unpack,
//     for the tests in tests/ (C120b, Q315, Q314)
func Unpack(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, *starlark.List, error) {
	var (
		doc   starlark.Value
		patch *starlark.List
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&doc,
		&patch,
	)
	if err != nil {
		return nil, nil, err
	}

	return doc, patch, nil
}

// ExtractJSON returns the value at a pointer, or None when any step is
// missing.
//
// None rather than an error, because a script asking whether an optional field
// is present is the ordinary case. A malformed pointer is still an error: that
// is the script being wrong, not the document.
//
// Revisions:
//   - 2026-09-20 01:15: initial creation
//   - 2026-11-02 17:12: walks with _Locate, so a miss builds no error (C36)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _ExtractJSON to
//     ExtractJSON, for the tests in tests/ (C120b, Q315, Q314)
func ExtractJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, pointer, err := Target(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	value, found, err := _Locate(
		doc,
		pointer,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if !found {
		return starlark.None, nil
	}

	return value, nil
}

// MatchJSON reports whether the value at a pointer deep-equals a value.
//
// A missing path is False rather than an error, which is RFC 6902's test
// semantics read as a question instead of an assertion.
//
// Revisions:
//   - 2026-09-20 01:16: initial creation
//   - 2026-11-02 17:12: walks with _Locate, so a miss builds no error (C36)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _MatchJSON to
//     MatchJSON, for the tests in tests/ (C120b, Q315, Q314)
func MatchJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, pointer, want, err := MatchArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	got, found, err := _Locate(
		doc,
		pointer,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if !found {
		return starlark.False, nil
	}

	same, err := _Equal(
		got,
		want,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return starlark.Bool(same), nil
}

// MatchArgs reads the document, the pointer, and the value a match compares.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _MatchArgs to
//     MatchArgs, for the tests in tests/ (C120b, Q315, Q314)
func MatchArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, string, starlark.Value, error) {
	var (
		doc     starlark.Value
		pointer string
		want    starlark.Value
	)

	const NEEDED = 3

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&doc,
		&pointer,
		&want,
	)
	if err != nil {
		return nil, "", nil, err
	}

	return doc, pointer, want, nil
}

// LenJSON returns the length of what a pointer names, or 0 when it is
// missing.
//
// A value that exists but has no length is an error, because that is a script
// asking a question of the wrong thing - unlike a missing path, which is a
// document simply not having something.
//
// Revisions:
//   - 2026-09-20 01:17: initial creation
//   - 2026-11-02 17:12: walks with _Locate, so a miss builds no error (C36)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _LenJSON to LenJSON,
//     for the tests in tests/ (C120b, Q315, Q314)
func LenJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, pointer, err := Target(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	value, found, err := _Locate(
		doc,
		pointer,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if !found {
		return starlark.MakeInt(spec.NO_VALUE), nil
	}

	measured, ok := value.(starlark.Sequence)
	if ok {
		return starlark.MakeInt(measured.Len()), nil
	}

	switch sized := value.(type) {
	case starlark.String:
		return starlark.MakeInt(sized.Len()), nil

	case *starlark.Dict:
		return starlark.MakeInt(sized.Len()), nil

	default:
		return nil, fmt.Errorf(
			"%s: %s has no length: %w",
			shared.Called(fn),
			value.Type(),
			ErrKind,
		)
	}
}

// FindKey returns the first member named key, searching depth first, or None.
//
// There is no path argument: this is the search a script reaches for when it
// knows a name appears once and does not want to spell out where.
//
// Revisions:
//   - 2026-09-20 01:18: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-02 12:28: walks with a trail: a list that holds itself is the
//     error "cycle in value", a value nested past common.DEPTH ends the run,
//     and the walk stops when the run is cancelled (C26)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _FindKey to FindKey,
//     for the tests in tests/ (C120b, Q315, Q314)
func FindKey(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		doc starlark.Value
		key string
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&doc,
		&key,
	)
	if err != nil {
		return nil, err
	}

	found, err := _Search(
		trail.New(scheduler.Ambient(thread)),
		doc,
		starlark.String(key),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if found == nil {
		return starlark.None, nil
	}

	return found, nil
}

// _Search walks doc depth first for a member named key.
//
// key is a Value, boxed once by the caller: converting a String to a Value
// allocates, and doing it for each dict visited was one allocation of seven a
// dict.
//
// A list is entered in the trail, so one that holds itself is
// trail.ErrCyclic. A scalar counts as an element, so a long list of
// them is walked in steps of common.POLL between looks at the context.
//
// Revisions:
//   - 2026-09-20 01:19: initial creation
//   - 2026-10-03 16:55: searches a dict through _SearchDict
//   - 2026-11-02 12:28: carries the trail, and takes the key as a Value (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func _Search(
	path *common.Trail,
	doc starlark.Value,
	key starlark.Value,
) (starlark.Value, error) {
	switch container := doc.(type) {
	case *starlark.Dict:
		return _SearchDict(
			path,
			container,
			key,
		)

	case *starlark.List:
		return _SearchList(
			path,
			container,
			key,
		)
	}

	return nil, trail.Tick(path)
}

// _SearchList looks below each element of holder in turn.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation, from _Search (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _SearchList(
	path *common.Trail,
	holder *starlark.List,
	key starlark.Value,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		holder,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	for index := range holder.Len() {
		found, err := _Search(
			path,
			holder.Index(index),
			key,
		)
		if err != nil || found != nil {
			return found, err
		}
	}

	return nil, nil
}

// _SearchDict looks for a member named key in holder, and then below each of
// its members in turn.
//
// The members are walked through Entries rather than Items, which builds a
// slice of every pair before the first is looked at: on a miss in a dict of
// 4096 members that slice was all the search allocated. And in a function of
// its own, apart from the recursion over every value: Go moves the key to the
// heap on entry to whatever function loops over Entries, and inside the
// recursion that is every value searched, scalars included - two allocations a
// member rather than none.
//
// Revisions:
//   - 2026-10-03 16:55: initial creation, from _Search
//   - 2026-11-02 12:28: enters the trail, and looks the key up without boxing it
//     again (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _SearchDict(
	path *common.Trail,
	holder *starlark.Dict,
	key starlark.Value,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		holder,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	value, present, err := holder.Get(key)
	if err != nil {
		return nil, err
	}

	if present {
		return value, nil
	}

	for _, member := range holder.Entries() {
		found, failure := _Search(
			path,
			member,
			key,
		)
		if failure != nil || found != nil {
			return found, failure
		}
	}

	return nil, nil
}

// Target reads the document and pointer two of these builtins share.
//
// Revisions:
//   - 2026-09-20 01:20: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-02 17:12: returns the pointer as text; the callers walk it with
//     _Locate and add the builtin name to its refusal (C36)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _Target to Target,
//     for the tests in tests/ (C120b, Q315, Q314)
func Target(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, string, error) {
	var (
		doc     starlark.Value
		pointer string
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&doc,
		&pointer,
	)
	if err != nil {
		return nil, "", err
	}

	return doc, pointer, nil
}

var (
	// ErrOperation is returned for an op this does not implement, and for an
	// op that is missing something it requires. The wrap names which.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrOperation = errors.New("malformed patch operation")

	// ErrTest is returned when a test op does not match.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrTest = errors.New("test failed")

	// ErrInto is returned for a move whose destination is inside its source.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrInto = errors.New("cannot move a value into itself")
)

// _Patch applies one RFC 6902 operation to doc and returns the result.
//
// The document handed to the patch is never mutated: an operation copies a
// container of it along the path it touches, the first time any operation does,
// and leaves everything else shared. That is what Python's jsonpatch does with
// in_place=False, and it is the only safe shape here, since a document may
// already be frozen or reachable from another thread.
//
// Members of op that the operation does not define are not read. RFC 6902
// section 4: "Members that are not explicitly defined for the operation in
// question MUST be ignored". TestPatch_IgnoresUnknownMembers pins it.
//
// Revisions:
//   - 2026-09-20 00:53: initial creation
//   - 2026-09-21 08:09: one keyword per case
//   - 2026-10-03 16:53: a method of _Edit, so a later operation changes in place what
//     an earlier one copied
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 16:45: names the clause that requires unknown members to be ignored (C22)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Patch(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	kind, err := _Field(
		op,
		spec.OP,
	)
	if err != nil {
		return nil, err
	}

	switch kind {
	case spec.ADD:
		fallthrough
	case spec.REPLACE:
		return e._Write(
			doc,
			op,
			kind,
		)

	case spec.REMOVE:
		return e._Removed(
			doc,
			op,
		)

	case spec.MOVE:
		fallthrough
	case spec.COPY:
		return e._Relocate(
			doc,
			op,
			kind,
		)

	case spec.TEST:
		return _Test(
			doc,
			op,
		)

	default:
		return nil, fmt.Errorf(
			"%s: unknown patch operation: %w",
			unpack.QuotePrefix(kind),
			ErrOperation,
		)
	}
}

// _Removed deletes the path an operation names.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Removed(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		spec.PATH,
	)
	if err != nil {
		return nil, err
	}

	return e._Remove(
		doc,
		path,
	)
}

// _Field reads a required string member of an operation.
//
// Revisions:
//   - 2026-09-20 00:54: initial creation
//   - 2026-09-24 16:08: a copy is given its own containers, where it used to
//     share them with the document it came from
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Field(
	op *starlark.Dict,
	name string,
) (string, error) {
	value, found, err := op.Get(starlark.String(name))
	if err != nil || !found {
		return "", fmt.Errorf(
			"%q: patch operation is missing a field: %w",
			name,
			ErrOperation,
		)
	}

	text, ok := value.(starlark.String)
	if !ok {
		return "", fmt.Errorf(
			"%q is %s: patch operation is missing a field: %w",
			name,
			value.Type(),
			ErrOperation,
		)
	}

	return string(text), nil
}

// _Value reads an operation's value member, which add, replace and test
// require.
//
// Revisions:
//   - 2026-09-20 00:55: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Value(op *starlark.Dict) (starlark.Value, error) {
	value, found, err := op.Get(starlark.String(spec.VALUE))
	if err != nil || !found {
		return nil, fmt.Errorf(
			"%q: patch operation is missing a field: %w",
			spec.VALUE,
			ErrOperation,
		)
	}

	return value, nil
}

// _Write applies add or replace.
//
// They differ in one rule: replace requires the path to exist already, and add
// does not. Add to a list index inserts; add to "-" appends.
//
// Revisions:
//   - 2026-09-20 00:56: initial creation
//   - 2026-09-21 08:09: inserts or replaces through two functions rather than
//     one with a flag
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
//   - 2026-11-02 16:14: reads the pointer through _Nested, so a pointer past
//     common.DEPTH is trail.ErrTooDeep (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Write(
	doc starlark.Value,
	op *starlark.Dict,
	kind string,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		spec.PATH,
	)
	if err != nil {
		return nil, err
	}

	value, err := _Value(op)
	if err != nil {
		return nil, err
	}

	steps, err := _Nested(path)
	if err != nil {
		return nil, err
	}

	return e._Written(
		doc,
		steps,
		value,
		kind,
	)
}

// _Written copies value and inserts or replaces it at steps.
//
// The copy is the leaf. _Insert copies every container along the path, and
// this stops a caller who still holds value from changing the result.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 12:28: the copy takes the thread's context and refuses a value
//     nested past common.DEPTH (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Written(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
	kind string,
) (starlark.Value, error) {
	own, err := deep.Copy(
		scheduler.Ambient(e.thread),
		value,
	)
	if err != nil {
		return nil, err
	}

	if kind == spec.ADD {
		return e._Insert(
			doc,
			steps,
			own,
		)
	}

	_, err = Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	return e._Replace(
		doc,
		steps,
		own,
	)
}

// _Remove deletes what path names.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
//   - 2026-11-02 16:14: reads the pointer through _Nested, so a pointer past
//     common.DEPTH is trail.ErrTooDeep (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Remove(
	doc starlark.Value,
	path string,
) (starlark.Value, error) {
	steps, err := _Nested(path)
	if err != nil {
		return nil, err
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf(
			"the whole document: %w",
			ErrPointer,
		)
	}

	_, err = Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	return e._Delete(
		doc,
		steps,
	)
}

// _Relocate applies move or copy.
//
// A move out of a path into its own child is refused: the source would have to
// exist inside the value being moved, and RFC 6902 names this as an error
// rather than leaving the result to an implementation. A move onto itself is
// allowed, as the specification allows it, and changes nothing.
//
// Revisions:
//   - 2026-09-20 00:58: initial creation
//   - 2026-09-21 08:09: inserts through _Insert; a move onto itself is a no-op
//     rather than a refusal
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
//   - 2026-10-03 19:28: a move onto itself returns the document as it was, the whole
//     document included, rather than deleting the value and putting it back
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Relocate(
	doc starlark.Value,
	op *starlark.Dict,
	kind string,
) (starlark.Value, error) {
	from, path, source, target, err := _Route(op)
	if err != nil {
		return nil, err
	}

	if kind == spec.MOVE && _Inside(
		source,
		target,
	) {
		return nil, fmt.Errorf(
			"%s into %s: %w",
			unpack.QuotePrefix(from),
			unpack.QuotePrefix(path),
			ErrInto,
		)
	}

	value, err := Walk(
		doc,
		source,
	)
	if err != nil {
		return nil, err
	}

	return e._Placed(
		&_Place{
			doc:    doc,
			kind:   kind,
			source: source,
			target: target,
			value:  value,
		},
	)
}

// _Route reads from and path, and the steps each one walks.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 16:14: reads both pointers through _Nested, so a pointer past
//     common.DEPTH is trail.ErrTooDeep (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Route(
	op *starlark.Dict,
) (string, string, []string, []string, error) {
	from, err := _Field(
		op,
		spec.FROM,
	)
	if err != nil {
		return "", "", nil, nil, err
	}

	path, err := _Field(
		op,
		spec.PATH,
	)
	if err != nil {
		return "", "", nil, nil, err
	}

	source, err := _Nested(from)
	if err != nil {
		return "", "", nil, nil, err
	}

	target, err := _Nested(path)
	if err != nil {
		return "", "", nil, nil, err
	}

	return from, path, source, target, nil
}

// _Place is a value on its way from one path to another.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
type _Place struct {
	doc    starlark.Value
	kind   string
	source []string
	target []string
	value  starlark.Value
}

// _Placed moves or copies the value onto its target.
//
// A move onto itself returns the document. Deleting the value and putting
// it back would move a member to the end of its dict. A copy gets its own
// value, so a later edit at the target cannot change the source.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 12:28: the copy takes the thread's context and refuses a value
//     nested past common.DEPTH (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Placed(place *_Place) (starlark.Value, error) {
	stays := place.kind == spec.MOVE && slices.Equal(
		place.source,
		place.target,
	)
	if stays {
		return place.doc, nil
	}

	if place.kind == spec.MOVE {
		doc, err := e._Delete(
			place.doc,
			place.source,
		)
		if err != nil {
			return nil, err
		}

		return e._Insert(
			doc,
			place.target,
			place.value,
		)
	}

	own, err := deep.Copy(
		scheduler.Ambient(e.thread),
		place.value,
	)
	if err != nil {
		return nil, err
	}

	return e._Insert(
		place.doc,
		place.target,
		own,
	)
}

// _Inside reports whether target is strictly below source.
//
// Strictly: RFC 6902 forbids a "from" that is a proper prefix of "path", and
// says nothing against the two being equal.
//
// Revisions:
//   - 2026-09-20 00:59: initial creation
//   - 2026-09-21 08:09: a path equal to its source is not inside it
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Inside(
	source []string,
	target []string,
) bool {
	if len(target) <= len(source) {
		return false
	}

	for index, step := range source {
		if target[index] != step {
			return false
		}
	}

	return true
}

// _Test compares what path names against a value, and returns the document
// unchanged when they match.
//
// Revisions:
//   - 2026-09-20 01:00: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Test(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		spec.PATH,
	)
	if err != nil {
		return nil, err
	}

	want, err := _Value(op)
	if err != nil {
		return nil, err
	}

	steps, err := Steps(path)
	if err != nil {
		return nil, err
	}

	got, err := Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	same, err := _Equal(
		got,
		want,
	)
	if err != nil {
		return nil, err
	}

	if !same {
		return nil, fmt.Errorf(
			"%s is %s, not %s: %w",
			unpack.ShortText(path),
			unpack.ShortText(got.String()),
			unpack.ShortText(want.String()),
			ErrTest,
		)
	}

	return doc, nil
}
