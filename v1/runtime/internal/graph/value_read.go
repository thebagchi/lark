package graph

import (
	"math"
	"unicode/utf8"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/internal/declared"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// One direction of one subject lives here: reading the value a literal states
// out of a script. The other direction is in value_write.go.

// _Arg is the value an expression states, and whether it states one at all.
//
// A literal, or a list or dict of them. A name, an arithmetic expression or a
// call states no value until it runs, which is a classification rather than a
// fault: the call it belongs to passes a name, or is not a statement.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-10 01:00: now uses Literal, were _Literal (C120a, Q314)
//   - 2026-11-13 10:15: the value is a workflow.Value (C132, Q407, Q425)
func _Arg(expr syntax.Expr) (*workflowpb.Value, bool) {
	switch actual := expr.(type) {
	case *syntax.Literal:
		return Literal(actual)

	case *syntax.Ident:
		return _Word(actual.Name)

	case *syntax.ListExpr:
		return _Every(actual.List)

	case *syntax.DictExpr:
		return _Pairs(actual.List)
	}

	return nil, false
}

// Literal is the value a literal states.
//
// go.starlark.net keeps the value of a bytes literal as a Go string, as it does
// a string's, so the token tells the two apart: b"0" is bytes and "0" is a string
// (B290). An int is an int64 (Q440): one past that range, which go.starlark.net
// holds as a *big.Int, states no value, so a constant that holds one is refused
// in convert. A float stays a float, so 1 and 1.0 are two values (Q421), and one
// that is NaN or an infinity states no value. A string that is not valid UTF-8
// states none either, as a flow value holds text (Q436, B294).
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-08 10:35: a nil literal states no value (C108, B192)
//   - 2026-11-10 00:25: renamed from _Literal, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-12 23:33: a bytes literal states no value, was read as a string (C130, B290, Q403)
//   - 2026-11-13 10:15: a bytes literal is bytes, an int is an int64 and a float is a
//     float, were JSON numbers; an int past int64 and a string that is no UTF-8 state none
//     (C132, Q407, Q421, Q436, Q440)
func Literal(literal *syntax.Literal) (*workflowpb.Value, bool) {
	if literal == nil {
		return nil, false
	}

	switch actual := literal.Value.(type) {
	case string:
		if literal.Token != syntax.BYTES && !utf8.ValidString(actual) {
			return nil, false
		}

		if literal.Token == syntax.BYTES {
			return &workflowpb.Value{
				Kind: &workflowpb.Value_Bytes{
					Bytes: []byte(actual),
				},
			}, true
		}

		return &workflowpb.Value{
			Kind: &workflowpb.Value_String_{
				String_: actual,
			},
		}, true

	case int64:
		return _Integral(actual), true

	case float64:
		if math.IsNaN(actual) || math.IsInf(
			actual,
			0,
		) {
			return nil, false
		}

		return &workflowpb.Value{
			Kind: &workflowpb.Value_Float{
				Float: actual,
			},
		}, true
	}

	return nil, false
}

// _Integral is an int value of the number it is given.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q440)
func _Integral(number int64) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Int{
			Int: number,
		},
	}
}

// _Word is the value one of Starlark's three bare words states.
//
// True, False and None are parsed as identifiers rather than literals, so
// without this a flow would drop every boolean argument a script passes.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-13 10:15: the value is a workflow.Value (C132, Q407, Q425)
func _Word(name string) (*workflowpb.Value, bool) {
	switch name {
	case TRUE:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Bool{
				Bool: true,
			},
		}, true

	case FALSE:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Bool{
				Bool: false,
			},
		}, true

	case NONE:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Null{
				Null: structpb.NullValue_NULL_VALUE,
			},
		}, true
	}

	return nil, false
}

// _Every is the value a list states, or none if any element states none.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-13 10:15: the value is a workflow.Value (C132, Q407, Q425)
func _Every(items []syntax.Expr) (*workflowpb.Value, bool) {
	list := new(workflowpb.List)

	for _, item := range items {
		value, ok := _Arg(item)
		if !ok {
			return nil, false
		}

		list.Items = append(
			list.Items,
			value,
		)
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_List{
			List: list,
		},
	}, true
}

// _Pairs is the value a dict states, or none if any key or value states none.
//
// Only a string key, and not an empty one, because a map of the flow has no other
// kind (Q423, Q435). A dict whose key is another value is carried as text by the
// reading, and reaches neither this function nor the flow's values.
//
// And at most one pair. A map has no order, while Starlark keeps a dict in the
// order it was written and prints it that way - so a dict of two pairs carried
// through the schema comes back possibly reordered, and a script that printed
// one would print something else. One pair has no order to lose.
//
// This is the schema's limit rather than this function's, and it is refused
// rather than taken quietly: a reordered dict is a flow that lies about what
// its script does, where a refusal is a flow that says less.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 01:32: refuses a dict of more than one pair, whose order the
//     schema cannot carry
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-13 10:15: the value is a workflow.Map, and the key is a string literal that is
//     not empty (C132, Q407, Q423, Q435)
func _Pairs(items []syntax.Expr) (*workflowpb.Value, bool) {
	if len(items) > 1 {
		return nil, false
	}

	entries := make(map[string]*workflowpb.Value)

	for _, item := range items {
		entry, ok := item.(*syntax.DictEntry)
		if !ok {
			return nil, false
		}

		key, ok := entry.Key.(*syntax.Literal)
		if !ok {
			return nil, false
		}

		var named, _ = Literal(key)

		text, ok := named.GetKind().(*workflowpb.Value_String_)
		if !ok || text.String_ == "" {
			return nil, false
		}

		value, ok := _Arg(entry.Value)
		if !ok {
			return nil, false
		}

		entries[text.String_] = value
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_Map{
			Map: &workflowpb.Map{
				Entries: entries,
			},
		},
	}, true
}

// _Quantity is the number an expression states, and whether it states one.
//
// A sleep or a wrapper wants a number, and _Arg answers for any literal: read
// as a number a string or a bool is a zero, so sleep("1") became a sleep of
// nothing rather than a statement the flow does not model. An int and a float
// are numbers, as the builtin takes both, so sleep(1000.0) is a second too.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-13 10:15: an int and a float are the two numbers (C132, Q407, Q421, Q440)
func _Quantity(expr syntax.Expr) (float64, bool) {
	value, ok := _Arg(expr)
	if !ok {
		return 0, false
	}

	switch number := value.GetKind().(type) {
	case *workflowpb.Value_Int:
		return float64(number.Int), true

	case *workflowpb.Value_Float:
		return number.Float, true
	}

	return 0, false
}

// _Natural is the whole, non-negative number an expression states, when it
// fits an int32, and whether it states one.
//
// What a duration in milliseconds is, which a script and the schema both count,
// and what a loop's count is. A duration that is anything else is one the
// builtin refuses with scheduler.ErrDuration, so it is no statement and the function
// holding it keeps its text.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
//   - 2026-11-02 10:25: moved to value_read.go
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
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

// _Keyed reports whether an expression states a value except for a dict key that
// is no string: a list or a dict of values, or a call argument of one, in which a
// dict is keyed by a bytes, a number or a bare word. Such a dict is no map, which
// has string keys only (Q423), so the statement that holds it travels as text
// (Q406).
//
// Revisions:
//   - 2026-11-13 00:47: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: renamed from _Holds, and asks for a key that is no string, where it asked
//     for a bytes literal, which is a value now (C132, Q406, Q407, Q423)
func _Keyed(expr syntax.Expr) bool {
	stringed, changed := _Stringed(expr)
	if !changed {
		return false
	}

	_, ok := _Arg(stringed)

	return ok
}

// _Stringed is a copy of an expression with each dict key that is no string, of
// its lists, dicts and the arguments of a call, written as a string literal, and
// whether any was. A node of any other kind is
// returned as it is, so _Arg refuses it as it would the original.
//
// A key is rewritten only when it states a value: a literal, or True, False or
// None. A key that is a name, a tuple or a sum is left alone, so the dict stays
// one _Arg refuses.
//
// Revisions:
//   - 2026-11-13 01:24: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: rewrites a dict key that is no string, where it rewrote a bytes
//     literal, and no keyword value, which a constant call never takes (C132, Q406, Q407, Q423)
func _Stringed(expr syntax.Expr) (syntax.Expr, bool) {
	switch actual := expr.(type) {
	case *syntax.ListExpr:
		copied := *actual

		var changed bool

		copied.List, changed = _Everything(actual.List)

		return &copied, changed

	case *syntax.DictExpr:
		copied := *actual

		var changed bool

		copied.List, changed = _Everything(actual.List)

		return &copied, changed

	case *syntax.DictEntry:
		var (
			copied       = *actual
			keyed, first = _Quoted(actual.Key)
			valued, next = _Stringed(actual.Value)
		)

		copied.Key = keyed
		copied.Value = valued

		return &copied, first || next

	case *syntax.CallExpr:
		copied := *actual

		var changed bool

		copied.Args, changed = _Everything(actual.Args)

		return &copied, changed
	}

	return expr, false
}

// _Quoted is a dict key as a string literal, "k", when it states a value that is
// no string, and whether it did. The literal is not empty, which _Pairs refuses.
// A string key, and a key that states no value, are returned as they are.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation, out of _Stringed (C132, Q406, Q423, Q435)
func _Quoted(key syntax.Expr) (syntax.Expr, bool) {
	value, ok := _Arg(key)
	if !ok {
		return key, false
	}

	switch value.GetKind().(type) {
	case *workflowpb.Value_String_, *workflowpb.Value_List, *workflowpb.Value_Map:
		return key, false
	}

	return &syntax.Literal{
		Token: syntax.STRING,
		Value: "k",
		Raw:   `"k"`,
	}, true
}

// _Everything is a copy of a list of expressions, each through _Stringed, and
// whether any changed.
//
// Revisions:
//   - 2026-11-13 01:24: initial creation, out of _Stringed (C130, B290, Q406)
func _Everything(items []syntax.Expr) ([]syntax.Expr, bool) {
	var (
		changed bool
		copied  = make(
			[]syntax.Expr,
			len(items),
		)
	)

	for idx, item := range items {
		var swapped bool

		copied[idx], swapped = _Stringed(item)
		changed = changed || swapped
	}

	return copied, changed
}

// _Defaults reports whether an expression is a call of arg whose default is keyed
// as _Keyed says: a dict with a key that is no string. The name is read as
// _Declared reads it, a non-empty string in the first place, so a form _Declared
// refuses, such as arg(name = "a", default = {1: 2}), is refused whatever its
// default is (Q406).
//
// Revisions:
//   - 2026-11-13 01:18: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: asks for a default with a key that is no string, where it asked for
//     a bytes literal (C132, Q406, Q407, Q423)
func _Defaults(expr syntax.Expr) bool {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return false
	}

	callee := declared.Callee(call)
	if callee == nil || callee.Name != ARG || len(call.Args) != TAKES {
		return false
	}

	name, spelled := declared.Spelled(call.Args[0])
	if !spelled || name == "" {
		return false
	}

	return _Keyed(Passed(call.Args[1]))
}
