package graph

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Invocation is one call as a script writes it: the function and its
// arguments.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Invocation(call *workflowpb.Call) (string, error) {
	args, err := g._Args(call)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s(%s)", call.GetFunction(), args), nil
}

// _Site is one call as it is handed to spawn, wrapped in a lambda when it
// passes arguments.
//
// A lambda appears only where a call has arguments. spawn takes none to pass
// on, so a site that has them closes over them; a site without them is the
// function itself, which has a name and reports it.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Site(call *workflowpb.Call) (string, error) {
	if len(call.GetArgs()) == 0 {
		return call.GetFunction(), nil
	}

	invocation, err := g._Invocation(call)
	if err != nil {
		return "", err
	}

	return LAMBDA + invocation, nil
}

// _Args is a call's arguments as Starlark writes them, comma separated.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Args(call *workflowpb.Call) (string, error) {
	var written []string

	for _, arg := range call.GetArgs() {
		value, err := _Value(arg)
		if err != nil {
			return "", fmt.Errorf("%s: %w", call.GetFunction(), err)
		}

		written = append(written, value)
	}

	return strings.Join(written, SEPARATOR), nil
}

// _Value is one JSON argument as a Starlark literal.
//
// A number is the kind that needs a decision: JSON has one and Starlark has
// two, and 3 and 3.0 differ under // and %. A whole number is written as an
// integer, because a UI that wrote 3 meant three and writes 3.0 for a float.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _Value(value *structpb.Value) (string, error) {
	switch kind := value.GetKind().(type) {
	case *structpb.Value_NullValue, nil:
		return NONE, nil

	case *structpb.Value_StringValue:
		return strconv.Quote(kind.StringValue), nil

	case *structpb.Value_NumberValue:
		return _Number(kind.NumberValue), nil

	case *structpb.Value_BoolValue:
		if kind.BoolValue {
			return TRUE, nil
		}

		return FALSE, nil

	case *structpb.Value_ListValue:
		return _List(kind.ListValue)

	case *structpb.Value_StructValue:
		return _Struct(kind.StructValue)
	}

	return "", fmt.Errorf("%T: %w", value.GetKind(), ErrNoValue)
}

// _Number is a JSON number as Starlark writes it.
//
// FormatFloat with the shortest representation gives 3 for a whole number and
// 3.5 otherwise, which is the distinction Starlark cares about and JSON does
// not carry.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _Number(number float64) string {
	return strconv.FormatFloat(number, WHOLE, FRACTION, 64)
}

// _Sorted is a struct's keys in order.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _Sorted(fields *structpb.Struct) []string {
	keys := make([]string, 0, len(fields.GetFields()))

	for key := range fields.GetFields() {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// _List is a JSON array as a Starlark list.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _List(list *structpb.ListValue) (string, error) {
	var written []string

	for _, item := range list.GetValues() {
		value, err := _Value(item)
		if err != nil {
			return "", err
		}

		written = append(written, value)
	}

	return "[" + strings.Join(written, SEPARATOR) + "]", nil
}

// _Struct is a JSON object as a Starlark dict.
//
// Keys are written in sorted order. A map has none of its own, and a generator
// whose output changes between runs is one whose result cannot be compared.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _Struct(fields *structpb.Struct) (string, error) {
	var written []string

	for _, key := range _Sorted(fields) {
		value, err := _Value(fields.GetFields()[key])
		if err != nil {
			return "", err
		}

		written = append(written, strconv.Quote(key)+": "+value)
	}

	return "{" + strings.Join(written, SEPARATOR) + "}", nil
}
