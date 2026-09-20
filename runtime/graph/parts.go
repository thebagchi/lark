package graph

import (
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"
)

// _Parts is the sub-expressions of a composite expression, left to right,
// which is the order Starlark evaluates them in.
//
// Listed rather than walked, because syntax.Walk is pre-order: it would hand
// back a call before that call's arguments, and the order steps come out in is
// the whole of what a derived graph promises.
//
// An expression kind missing from here yields no parts, so a call buried in
// one produces no step. That is a graph saying less, which the body it also
// carries makes good.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Parts(expr syntax.Expr) []syntax.Expr {
	switch actual := expr.(type) {
	case *syntax.BinaryExpr:
		return []syntax.Expr{actual.X, actual.Y}

	case *syntax.UnaryExpr:
		return []syntax.Expr{actual.X}

	case *syntax.ParenExpr:
		return []syntax.Expr{actual.X}

	case *syntax.ListExpr:
		return actual.List

	case *syntax.TupleExpr:
		return actual.List

	case *syntax.DictExpr:
		return actual.List

	case *syntax.DictEntry:
		return []syntax.Expr{actual.Key, actual.Value}

	case *syntax.IndexExpr:
		return []syntax.Expr{actual.X, actual.Y}

	case *syntax.DotExpr:
		return []syntax.Expr{actual.X}

	case *syntax.CondExpr:
		return []syntax.Expr{actual.Cond, actual.True, actual.False}
	}

	return nil
}

// _Arg is the value an expression states, and whether it states one at all.
//
// A literal, or a list or dict of them. A name, an arithmetic expression or a
// call states no value until it runs, which is a classification rather than a
// fault: the call it belongs to is simply not a step.
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
// without this a graph would drop every boolean argument a script passes.
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
// rather than taken quietly: a reordered dict is a graph that lies about what
// its script does, where a refusal is a graph that says less.
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
