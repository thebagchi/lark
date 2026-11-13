// Package json gives a script the json module. Importing it is what enables it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package json

import (
	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/json/internal/encode"
	"github.com/thebagchi/lark/v1/runtime/plugin/json/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
func init() {
	plugin.Register(&_Json{})
}

// _Json is the plugin. Empty: what it contributes is a value the library
// already builds, so there is nothing to hold.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Json struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (j *_Json) Name() string {
	return spec.NAME
}

// Values returns the one name this plugin supplies.
//
// The module comes from go.starlark.net rather than being written here: it is
// the library's own encoder, it is already a dependency, and a second JSON
// implementation in the same process is two behaviours a script could tell
// apart.
//
// Three members are replaced. Encode and encode_indent write DEL as \u007f, see
// _Encode. Decode checks the nesting of its text first, because
// the library's decoder recurses on it with no bound, and a text of five
// million brackets ends the process with a Go stack overflow. See _Decode.
//
// The library accepts more than RFC 8259 writes, and lark documents it rather
// than wrapping it: section 9 says "A JSON parser MAY accept non-JSON forms or
// extensions". Four extensions, each pinned by TestJSON_DecodeKeepsTheLibrarysExtensions:
// a raw control character inside a string of ASCII only (section 7 says it MUST be
// escaped); a number such as "1." or "1.e3" (section 6: frac = decimal-point
// 1*DIGIT); duplicate names, where the last wins (section 4: names SHOULD be
// unique); and an unpaired surrogate escape, which becomes U+FFFD (section 8.2). One
// refusal: a byte order mark (section 8.1 lets a parser ignore one; this does not).
//
// The import is go.starlark.net/lib/json, not the older go.starlark.net/
// starlarkjson, which the library deprecated. The linter found that; the POC
// this was lifted from still uses the old path, because nothing lints .poc.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
//   - 2026-11-02 12:28: decode checks the nesting of its text first (C26)
//   - 2026-11-02 16:45: documents the extensions of the library with their clauses (C22)
//   - 2026-11-04 23:49: encode and encode_indent write DEL as \u007f (C92, B121, Q111)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses encode.Repair, which moved to internal/encode (C120b, Q315)
func (j *_Json) Values() starlark.StringDict {
	members := make(
		starlark.StringDict,
		len(starlarkjson.Module.Members),
	)

	for name, member := range starlarkjson.Module.Members {
		members[name] = member
	}

	library, ok := members[spec.DECODE].(*starlark.Builtin)
	if ok {
		members[spec.DECODE] = shared.Guarded(
			library,
			_Depth,
		)
	}

	encode.Repair(members)

	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name:    starlarkjson.Module.Name,
			Members: members,
		},
	}
}

// _Depth checks that the text of a decode nests no deeper than
// trail.DEPTH, and is the check shared.Guarded runs before the library's
// decode.
//
// The library decodes by recursion, with no depth bound. A text of
// "[" repeated five million times, then "]" as often, overflows the Go stack:
// a fatal error that nothing can recover. The check counts brackets outside
// strings through the shared walker, so the limit, the error and the look at
// the context are the walker's. A text that is not JSON passes: the library
// reports it. A text too deep is trail.ErrDeep, which ends the run the way
// assert does.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation, as _Decode (C26)
//   - 2026-11-04 21:49: only checks; shared.Guarded calls the library and names the
//     builtin in the error (C59)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func _Depth(
	thread *starlark.Thread,
	args starlark.Tuple,
) error {
	if len(args) == 0 {
		return nil
	}

	text, ok := args[0].(starlark.String)
	if !ok {
		return nil
	}

	return _Nested(
		trail.Bounded(scheduler.Ambient(thread)),
		string(text),
	)
}

// _Nested enters the trail for each open bracket outside a string and leaves
// it for each close bracket, and returns the first error.
//
// A close bracket with nothing open is left to the library: Leave ignores it.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func _Nested(
	path *common.Trail,
	text string,
) error {
	var (
		quoted  bool
		escaped bool
	)

	for index := range len(text) {
		next := text[index]

		switch {
		case escaped:
			escaped = false

		case quoted:
			escaped = next == '\\'
			quoted = next != '"'

		case next == '"':
			quoted = true

		case next == '[' || next == '{':
			err := trail.Enter(
				path,
				nil,
			)
			if err != nil {
				return err
			}

		case next == ']' || next == '}':
			trail.Leave(path)
		}
	}

	return nil
}
