// Package encode holds the rendering side of the json plugin: the encode
// builtins, the escaping and indenting of their output and the repair of broken
// text. The plugin type and its registration stay in package json.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package encode

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"strings"

	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/json/internal/spec"
)

// Repair replaces encode and encode_indent in members with the versions that
// write DEL as \u007f. See _Encode.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92, B121, Q111)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/json and renamed _Repair to Repair, for
//     the tests in tests/ (C120b, Q315, Q314)
func Repair(members starlark.StringDict) {
	for name, call := range map[string]func(
		*starlark.Thread,
		*starlark.Builtin,
		starlark.Tuple,
		[]starlark.Tuple,
	) (starlark.Value, error){
		spec.ENCODE:        _Encode,
		spec.ENCODE_INDENT: Indent,
	} {
		library, ok := members[name].(*starlark.Builtin)
		if !ok {
			continue
		}

		members[name] = starlark.NewBuiltin(
			shared.Called(library),
			func(
				thread *starlark.Thread,
				_ *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				return call(
					thread,
					library,
					args,
					kwargs,
				)
			},
		)
	}
}

// _Encode calls the library's encode, then writes each \x7f escape as \u007f.
//
// Bug B121 (Q111). The library quotes a string of printable ASCII with
// strconv.AppendQuote, and its test for printable (isPrintableASCII) lets DEL
// (0x7f) through. AppendQuote writes DEL as \x7f. RFC 8259 section 7 has no
// \x escape, so json.decode refused the output. Any other string goes through
// encoding/json, which writes DEL raw; section 7 allows that, so it stays.
//
// Why a rewrite of the output is safe: the only \x the library can write is
// that one. AppendQuote writes \x only for a byte below 0x20 or for 0x7f, and
// the library sends every byte below 0x20 to encoding/json (which writes
// \u00XX). A backslash of the user is always written as \\, so a \x that
// starts an escape is the library's. See _Escaped.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92, B121, Q111)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/json (C120b, Q315)
func _Encode(
	thread *starlark.Thread,
	library *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	value, err := library.CallInternal(
		thread,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	text, ok := value.(starlark.String)
	if !ok {
		return value, nil
	}

	repaired, changed := _Escaped(string(text))
	if !changed {
		return value, nil
	}

	return starlark.String(repaired), nil
}

// Indent is the library's encode_indent, with the same repair as
// _Encode.
//
// The library's encode_indent calls its own encode, not the member, and then
// runs encoding/json.Indent, which refuses \x7f. So the repair must come before
// the indent, and the arguments are read here as the library reads them: the
// keywords prefix and indent, and the value as the one positional argument. An
// error from encode names json.encode; this function names it json.encode_indent,
// as the library does.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92, B121, Q111)
//   - 2026-11-04 23:49: takes the library's encode from its module, to fit four arguments
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/json and renamed _EncodeIndent to Indent,
//     for the tests in tests/ (C120b, Q315, Q314)
func Indent(
	thread *starlark.Thread,
	library *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	encode, ok := starlarkjson.Module.Members[spec.ENCODE].(*starlark.Builtin)
	if !ok {
		return library.CallInternal(
			thread,
			args,
			kwargs,
		)
	}

	var (
		prefix = ""
		indent = "\t"
	)

	err := starlark.UnpackArgs(
		shared.Called(library),
		nil,
		kwargs,
		"prefix?",
		&prefix,
		"indent?",
		&indent,
	)
	if err != nil {
		return nil, err
	}

	value, err := _Encode(
		thread,
		encode,
		args,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s%s",
			shared.Called(library),
			strings.TrimPrefix(
				err.Error(),
				shared.Called(encode),
			),
		)
	}

	return _Indented(
		shared.Called(library),
		string(value.(starlark.String)),
		prefix,
		indent,
	)
}

// _Indented runs encoding/json.Indent over text, as the library does, and
// names the builtin in its error.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/json (C120b, Q315)
func _Indented(
	name string,
	text string,
	prefix string,
	indent string,
) (starlark.Value, error) {
	var out bytes.Buffer

	err := stdjson.Indent(
		&out,
		[]byte(text),
		prefix,
		indent,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			name,
			err,
		)
	}

	return starlark.String(out.String()), nil
}

// _Escaped returns text with each \x7f escape written as \u007f, and whether
// it changed anything. Text with no change comes back as it is, with no
// allocation; the caller then keeps the value it already holds, so a string
// without DEL pays nothing (a new starlark.String would box with an allocation).
//
// The scan reads a backslash with the byte after it as one pair, so the text
// of a user, written as \\x7f, is two pairs and stays. See _Encode.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92, B121, Q111)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/json (C120b, Q315)
func _Escaped(text string) (string, bool) {
	var out *strings.Builder

	for index := 0; index < len(text); index++ {
		if text[index] != '\\' || index+1 == len(text) {
			if out != nil {
				out.WriteByte(text[index])
			}

			continue
		}

		if strings.HasPrefix(
			text[index:],
			`\x7f`,
		) {
			if out == nil {
				out = &strings.Builder{}

				out.Grow(len(text) + len(`\u007f`) - len(`\x7f`))
				out.WriteString(text[:index])
			}

			out.WriteString(`\u007f`)

			index += len(`\x7f`) - 1

			continue
		}

		if out != nil {
			out.WriteByte(text[index])
			out.WriteByte(text[index+1])
		}

		index++
	}

	if out == nil {
		return text, false
	}

	return out.String(), true
}
