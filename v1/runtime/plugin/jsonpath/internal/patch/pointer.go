package patch

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

var (
	// ErrPointer is returned for a pointer that is not RFC 6901.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrPointer = errors.New("not a json pointer")

	// ErrMissing is returned when a pointer names something that is not there.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrMissing = errors.New("no such path")

	// ErrKind is returned when a step asks for a member of something that has
	// no members, or an index of something that is not a list.
	//
	// Revisions:
	//   - 2026-09-20 00:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
	ErrKind = errors.New("cannot be indexed")
)

// Steps splits an RFC 6901 pointer into its unescaped reference tokens.
//
// The empty pointer is the whole document and yields no steps. Anything else
// must start with a slash: RFC 6901 has no relative pointers, and accepting one
// would silently treat "a/b" as the two steps a caller probably meant rather
// than the error they made.
//
// Unescaping is ~1 before ~0, and the order is the specification's. Doing it
// the other way turns "~01" into "/" instead of "~1".
//
// A "~" must be followed by 0 or 1. RFC 6901 section 3: "escaped = "~" ( "0" /
// "1" )", with "~" excluded from "unescaped", and "It is an error condition if
// a JSON Pointer value does not conform to this syntax". Section 7 lists
// "Invalid pointer syntax" among the error conditions. Accepting a lone "~"
// would let "/b~" and "/b~0" name one key, two spellings for one thing.
//
// Revisions:
//   - 2026-09-20 00:46: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 16:45: refuse a "~" that is not followed by 0 or 1 (C22)
//   - 2026-11-02 17:12: the refusals live in _Check, the decoding in _Unescape (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _Steps to Steps, for
//     the tests in tests/ (C120b, Q315, Q314)
func Steps(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}

	err := _Check(pointer)
	if err != nil {
		return nil, err
	}

	var (
		raw = strings.Split(
			strings.TrimPrefix(
				pointer,
				spec.SEPARATOR,
			),
			spec.SEPARATOR,
		)
		steps = make(
			[]string,
			0,
			len(raw),
		)
	)

	for _, token := range raw {
		steps = append(
			steps,
			_Unescape(token),
		)
	}

	return steps, nil
}

// _Check refuses a non-empty pointer that is not RFC 6901: one with no leading
// slash, or with a "~" not followed by 0 or 1. It reads the whole pointer, so
// the answer does not depend on the document it is later walked in.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation, the refusals of _Steps moved here so
//     _Steps and _Locate share one text (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Check(pointer string) error {
	if !strings.HasPrefix(
		pointer,
		spec.SEPARATOR,
	) {
		return fmt.Errorf(
			"%s does not start with %q: %w",
			unpack.QuotePrefix(pointer),
			spec.SEPARATOR,
			ErrPointer,
		)
	}

	if !_Escaped(pointer) {
		return fmt.Errorf(
			"%s has a %q not followed by 0 or 1: %w",
			unpack.QuotePrefix(pointer),
			spec.LITERAL_0,
			ErrPointer,
		)
	}

	return nil
}

// _Unescape decodes one reference token: ~1 before ~0, the order of RFC 6901
// section 4. A token with no "~" comes back as it is, with no copy.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation, from the body of the _Steps loop (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Unescape(token string) string {
	token = strings.ReplaceAll(
		token,
		spec.ESCAPED_1,
		spec.LITERAL_1,
	)

	return strings.ReplaceAll(
		token,
		spec.ESCAPED_0,
		spec.LITERAL_0,
	)
}

// _Nested is Steps for a pointer the patch applies by recursion, which
// refuses one with more steps than common.DEPTH.
//
// _Descend and _Delete call themselves once for each step, so the number of
// steps is the depth of the recursion, and a script can write a pointer of
// millions of steps in one line. The refusal is trail.ErrDeep, which ends
// the run as assert does. The count comes before the split, so a refused
// pointer allocates nothing.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func _Nested(pointer string) ([]string, error) {
	if strings.Count(
		pointer,
		spec.SEPARATOR,
	) > common.DEPTH {
		return nil, trail.ErrDeep
	}

	return Steps(pointer)
}

// _Escaped reports whether every "~" in a reference token starts "~0" or "~1",
// the two escapes of RFC 6901 section 3.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Escaped(token string) bool {
	for at := 0; at < len(token); at++ {
		if token[at] != spec.LITERAL_0[0] {
			continue
		}

		at++

		if at == len(token) || (token[at] != '0' && token[at] != '1') {
			return false
		}
	}

	return true
}

// Walk returns the value at the given steps, starting from doc.
//
// Returns ErrMissing when a step names nothing, and ErrKind when a step asks a
// value for something it cannot hold - a member of a list, or an index of a
// dict.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-04 22:29: quotes the steps escaped, as _Locate does (C59, Q150)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath and renamed _Walk to Walk, for
//     the tests in tests/ (C120b, Q315, Q314)
func Walk(
	doc starlark.Value,
	steps []string,
) (starlark.Value, error) {
	at := doc

	for index, step := range steps {
		next, err := _Step(
			at,
			step,
		)
		if err != nil {
			// RFC 6901 section 3 spells a step with ~ before /, in one pass, so a
			// written "~1" is not read back as a slash.
			var (
				escaper = strings.NewReplacer(
					spec.LITERAL_0,
					spec.ESCAPED_0,
					spec.LITERAL_1,
					spec.ESCAPED_1,
				)
				escaped = make(
					[]string,
					index+1,
				)
			)

			for at := range escaped {
				escaped[at] = escaper.Replace(steps[at])
			}

			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(spec.SEPARATOR+strings.Join(
					escaped,
					spec.SEPARATOR,
				)),
				err,
			)
		}

		at = next
	}

	return at, nil
}

// _Locate returns the value a pointer names in doc, and whether it is there.
//
// It is a cursor over the pointer. Each token is a substring of the pointer,
// found with IndexByte, so no slice of steps is built; only a token that holds
// "~" is copied, once. A miss anywhere is found == false and builds no error.
// A pointer that is not RFC 6901 is ErrPointer, whatever the document holds.
// A step that asks a list for something that is not an index is ErrPointer
// too, but only when the walk reaches it, as Walk did.
//
// The error quotes the pointer up to the failing step as the script wrote it,
// escapes and all (RFC 6901 section 3), so the text names a place the script can
// find in its own pointer. Walk writes its steps back the same way.
//
// Reader check: TestQueries_ANoneForEveryKindOfMiss, TestErrors_KeepTheirText
// and TestExtract_AMissCostsAsMuchAsAHit in locate_test.go.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36, D5 and the _Locate design of
//     poc/research/simd_json.md)
//   - 2026-11-04 21:49: quotes a slice of the pointer, not a second parse of it, so the
//     error shows the escaped steps (C59, A4f, Q141)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Locate(
	doc starlark.Value,
	pointer string,
) (starlark.Value, bool, error) {
	if pointer == "" {
		return doc, true, nil
	}

	err := _Check(pointer)
	if err != nil {
		return nil, false, err
	}

	var (
		at   = doc
		rest = pointer[len(spec.SEPARATOR):]
	)

	for more := true; more; {
		var (
			token = rest
			cut   = strings.IndexByte(
				rest,
				spec.SEPARATOR[0],
			)
		)

		if cut >= 0 {
			token, rest = rest[:cut], rest[cut+len(spec.SEPARATOR):]
		}

		more = cut >= 0

		next, found, err := _Find(
			at,
			_Unescape(token),
		)
		if err != nil {
			// On the last token rest was not advanced, so the token ends the pointer.
			end := len(pointer)
			if more {
				end -= len(rest) + len(spec.SEPARATOR)
			}

			return nil, false, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(pointer[:end]),
				err,
			)
		}

		if !found {
			return nil, false, nil
		}

		at = next
	}

	return at, true, nil
}

// _Find is _Step for a reader: a step that names nothing is found == false and
// builds no error.
//
// A list asked for something that is not an index is still ErrPointer, the
// script being wrong.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation, the lookup of _Step (C36)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Find(
	at starlark.Value,
	step string,
) (starlark.Value, bool, error) {
	switch holder := at.(type) {
	case *starlark.Dict:
		value, found, err := holder.Get(starlark.String(step))
		if err != nil {
			return nil, false, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(step),
				err,
			)
		}

		return value, found, nil

	case *starlark.List:
		index, err := _Parse(step)
		if err != nil {
			return nil, false, err
		}

		if index >= holder.Len() {
			return nil, false, nil
		}

		return holder.Index(index), true, nil

	default:
		return nil, false, nil
	}
}

// _Step returns the member or element of at named by step.
//
// It is _Find with the reason for a miss added: the error is built here, on
// the path that writes, and not in _Find, on the path that reads.
//
// Revisions:
//   - 2026-09-20 00:49: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 17:12: built on _Find; the reason for a miss moved to _Missed (C36)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Step(
	at starlark.Value,
	step string,
) (starlark.Value, error) {
	value, found, err := _Find(
		at,
		step,
	)
	if err != nil || found {
		return value, err
	}

	return nil, _Missed(
		at,
		step,
	)
}

// _Missed gives the error for a step that _Find did not find in at: ErrMissing
// for a dict or a list, ErrKind for a value that holds no members.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation, from the error branches of _Step (C36)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Missed(
	at starlark.Value,
	step string,
) error {
	switch holder := at.(type) {
	case *starlark.Dict:
		return ErrMissing

	case *starlark.List:
		_, err := _Index(
			step,
			holder.Len(),
		)
		if err == nil {
			return ErrMissing
		}

		return err

	default:
		return fmt.Errorf(
			"%s %w",
			at.Type(),
			ErrKind,
		)
	}
}

// _Index reads step as a list index, and refuses one outside a list of length.
//
// Revisions:
//   - 2026-09-20 00:50: initial creation
//   - 2026-09-21 08:09: digits only, so a signed step is not an index
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 17:12: the reading of the step moved to _Parse (C36)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Index(
	step string,
	length int,
) (int, error) {
	index, err := _Parse(step)
	if err != nil {
		return 0, err
	}

	if index >= length {
		return 0, fmt.Errorf(
			"%d is outside a list of %d: %w",
			index,
			length,
			ErrMissing,
		)
	}

	return index, nil
}

// _Parse reads step as a list index, refusing anything RFC 6901 does not allow.
//
// Leading zeros are refused because the specification spells an index as a
// digit sequence with no leading zero, so "01" is not 1 - it is not an index at
// all, and treating it as one would let two spellings name one element. A sign
// is refused for the same reason: strconv accepts "+1", and the specification
// does not.
//
// Revisions:
//   - 2026-09-20 00:50: initial creation, as part of _Index
//   - 2026-09-21 08:09: digits only, so a signed step is not an index
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 17:12: split from _Index, so a reader can find the index and
//     decide a miss itself (C36)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Parse(step string) (int, error) {
	if !_Digits(step) || (len(step) > 1 && strings.HasPrefix(
		step,
		"0",
	)) {
		return 0, fmt.Errorf(
			"%s is not an index: %w",
			unpack.QuotePrefix(step),
			ErrPointer,
		)
	}

	index, err := strconv.Atoi(step)
	if err != nil {
		return 0, fmt.Errorf(
			"%s is not an index: %w",
			unpack.QuotePrefix(step),
			ErrPointer,
		)
	}

	return index, nil
}

// _Digits reports whether step is one or more decimal digits.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Digits(step string) bool {
	if step == "" {
		return false
	}

	for _, char := range step {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
