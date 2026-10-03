package graph

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"
)

// Both directions of one subject live here: reading the value a literal states
// out of a script, and writing a value back as the literal a script would have.

const (
	// WHOLE is the format a number takes when it has no fractional part, and
	// FRACTION when it has. A JSON number is one type where Starlark has two,
	// and the two differ under // and %.
	WHOLE    = 'f'
	FRACTION = -1

	// BITS is the size FormatFloat assumes the number came from.
	BITS = 64
)

// _Arg is the value an expression states, and whether it states one at all.
//
// A literal, or a list or dict of them. A name, an arithmetic expression or a
// call states no value until it runs, which is a classification rather than a
// fault: the call it belongs to passes a name, or is not a statement.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Arg(expr syntax.Expr) (*structpb.Value, bool) {
	switch actual := expr.(type) {
	case *syntax.Literal:
		return _Literal(actual)

	case *syntax.Ident:
		return _Word(actual.Name)

	case *syntax.ListExpr:
		return _Every(actual.List)

	case *syntax.DictExpr:
		return _Pairs(actual.List)
	}

	return nil, false
}

// _Literal is the value a literal states.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Literal(literal *syntax.Literal) (*structpb.Value, bool) {
	switch actual := literal.Value.(type) {
	case string:
		return structpb.NewStringValue(actual), true

	case int64:
		return structpb.NewNumberValue(float64(actual)), true

	case float64:
		return structpb.NewNumberValue(actual), true
	}

	return nil, false
}

// _Word is the value one of Starlark's three bare words states.
//
// True, False and None are parsed as identifiers rather than literals, so
// without this a flow would drop every boolean argument a script passes.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Word(name string) (*structpb.Value, bool) {
	switch name {
	case TRUE:
		return structpb.NewBoolValue(true), true

	case FALSE:
		return structpb.NewBoolValue(false), true

	case NONE:
		return structpb.NewNullValue(), true
	}

	return nil, false
}

// _Every is the value a list states, or none if any element states none.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Every(items []syntax.Expr) (*structpb.Value, bool) {
	list := new(structpb.ListValue)

	for _, item := range items {
		value, ok := _Arg(item)
		if !ok {
			return nil, false
		}

		list.Values = append(list.Values, value)
	}

	return structpb.NewListValue(list), true
}

// _Pairs is the value a dict states, or none if any key or value states none.
//
// Only a string key, because a google.protobuf.Struct has no other kind.
//
// And at most one pair. A Struct is a map and a map has no order, while
// Starlark keeps a dict in the order it was written and prints it that way -
// so a dict of two pairs carried through the schema comes back possibly
// reordered, and a script that printed one would print something else. One
// pair has no order to lose.
//
// This is the schema's limit rather than this function's, and it is refused
// rather than taken quietly: a reordered dict is a flow that lies about what
// its script does, where a refusal is a flow that says less.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 01:32: refuses a dict of more than one pair, whose order the
//     schema cannot carry
func _Pairs(items []syntax.Expr) (*structpb.Value, bool) {
	if len(items) > 1 {
		return nil, false
	}

	fields := &structpb.Struct{Fields: make(map[string]*structpb.Value)}

	for _, item := range items {
		entry, ok := item.(*syntax.DictEntry)
		if !ok {
			return nil, false
		}

		key, ok := _Arg(entry.Key)
		if !ok || key.GetStringValue() == "" {
			return nil, false
		}

		value, ok := _Arg(entry.Value)
		if !ok {
			return nil, false
		}

		fields.Fields[key.GetStringValue()] = value
	}

	return structpb.NewStructValue(fields), true
}

// _Quantity is the number an expression states, and whether it states one.
//
// A sleep or a wrapper wants a number, and _Arg answers for any literal: read
// through GetNumberValue a string or a bool is a zero, so sleep("1") became a
// sleep of nothing rather than a statement the flow does not model.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Quantity(expr syntax.Expr) (float64, bool) {
	value, ok := _Arg(expr)
	if !ok {
		return 0, false
	}

	number, ok := value.GetKind().(*structpb.Value_NumberValue)
	if !ok {
		return 0, false
	}

	return number.NumberValue, true
}

// _Natural is the whole, non-negative number an expression states, when it
// fits an int32, and whether it states one.
//
// What a duration in milliseconds is, which a script and the schema both count,
// and what a loop's count is. A duration that is anything else is one the
// builtin refuses with ERR_DURATION, so it is no statement and the function
// holding it keeps its text.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
func _Natural(expr syntax.Expr) (int32, bool) {
	number, ok := _Quantity(expr)
	if !ok {
		return 0, false
	}

	whole := number >= 0 && number <= math.MaxInt32 && number == math.Trunc(number)
	if !whole {
		return 0, false
	}

	return int32(number), true
}

// _Value is one JSON value as a Starlark expression.
//
// A whole number is written as an integer. JSON has one number and a UI that
// wrote 3 meant three; 3.0 is how it writes a float.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
func _Value(value *structpb.Value) (syntax.Expr, error) {
	if value == nil {
		return _Name(NONE), nil
	}

	switch kind := value.GetKind().(type) {
	case nil:
		return _Name(NONE), nil

	case *structpb.Value_NullValue:
		return _Name(NONE), nil

	case *structpb.Value_StringValue:
		return _Text(strconv.Quote(kind.StringValue)), nil

	case *structpb.Value_NumberValue:
		return _Num(_Number(kind.NumberValue)), nil

	case *structpb.Value_BoolValue:
		return _Name(_Bool(kind.BoolValue)), nil

	case *structpb.Value_ListValue:
		return _List(kind.ListValue)

	case *structpb.Value_StructValue:
		return _Struct(kind.StructValue)

	default:
		return nil, fmt.Errorf("%T: %w", value.GetKind(), ERR_FORM)
	}
}

// _Bool is a JSON boolean as Starlark spells it.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Bool(value bool) string {
	if value {
		return TRUE
	}

	return FALSE
}

// _Number is a JSON number as Starlark writes it.
//
// FormatFloat with the shortest representation gives 3 for a whole number and
// 3.5 otherwise, which is the distinction Starlark cares about and JSON does
// not carry.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Number(number float64) string {
	return strconv.FormatFloat(number, WHOLE, FRACTION, BITS)
}

// _List is a JSON array as a Starlark list.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the list node
func _List(list *structpb.ListValue) (*syntax.ListExpr, error) {
	var items []syntax.Expr

	for _, item := range list.GetValues() {
		value, err := _Value(item)
		if err != nil {
			return nil, err
		}

		items = append(items, value)
	}

	return &syntax.ListExpr{List: items}, nil
}

// _Struct is a JSON object as a Starlark dict.
//
// Keys are written in sorted order. A map has none of its own, and a
// generator whose output changes between runs is one whose result cannot be
// compared.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the dict node
func _Struct(fields *structpb.Struct) (*syntax.DictExpr, error) {
	var entries []syntax.Expr

	for _, key := range slices.Sorted(maps.Keys(fields.GetFields())) {
		value, err := _Value(fields.GetFields()[key])
		if err != nil {
			return nil, err
		}

		entries = append(entries, &syntax.DictEntry{
			Key:   _Text(strconv.Quote(key)),
			Value: value,
		})
	}

	return &syntax.DictExpr{List: entries}, nil
}

// _Whole is a number literal that range can take. A fraction is not a count.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Whole(value *structpb.Value) (string, error) {
	number, ok := value.GetKind().(*structpb.Value_NumberValue)
	if !ok {
		return "", fmt.Errorf("loop: %w", ERR_FORM)
	}

	text := _Number(number.NumberValue)

	if strings.Contains(text, ".") {
		return "", fmt.Errorf("loop: %w", ERR_FORM)
	}

	return text, nil
}
