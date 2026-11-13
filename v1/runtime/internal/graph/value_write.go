package graph

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// One direction of one subject lives here: writing a value back as the literal
// a script would have. The other direction is in value_read.go.

// FRACTION is the precision FormatFloat takes to write the fewest digits that read
// back as the same float.
//
// Revisions:
//   - 2026-09-20 21:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-13 10:15: WHOLE is dropped, a float is no longer written as an int when it is whole
//     (C132, Q421)
const (
	FRACTION = -1

	// BITS is the size FormatFloat assumes the number came from.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	BITS = 64

	// DECIMAL is the base an int value is written in.
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q407, Q440)
	DECIMAL = 10

	// HEX holds the digit for each value of a half byte, in the case a bytes literal
	// is written in.
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421)
	HEX = "0123456789abcdef"
)

// _Value is one flow value as a Starlark expression.
//
// An int is written as the integer it holds and a float as a float, so 3 and 3.0
// come back as the two values they were: a flow keeps what the script wrote
// (Q421). A value that is not one is refused with ErrForm: a Value with no kind,
// a null that holds anything but null, a string that is not valid UTF-8, a float
// that is NaN or an infinity. One nested deeper than common.DEPTH is refused with
// trail.ErrDeep (Q2).
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-11-02 10:25: moved to value_write.go
//   - 2026-11-13 10:15: takes a workflow.Value, refuses one that is none, and bounds its depth, in
//     place of a google.protobuf.Value (C132, Q2, Q407, Q421, Q425, Q439)
func _Value(value *workflowpb.Value) (syntax.Expr, error) {
	return _Deeper(
		value,
		0,
	)
}

// _Deeper is one flow value as a Starlark expression, depth levels of list and
// map inside the value the caller started from. A null that holds anything but
// null, and a string that is not valid UTF-8, are ErrForm (Q439).
//
// Revisions:
//   - 2026-11-13 10:15: initial creation, out of _Value (C132, Q2, Q407, Q439)
func _Deeper(
	value *workflowpb.Value,
	depth int,
) (syntax.Expr, error) {
	if depth > common.DEPTH {
		return nil, fmt.Errorf(
			"value: %w",
			trail.ErrDeep,
		)
	}

	switch kind := value.GetKind().(type) {
	case *workflowpb.Value_Null:
		return _Nothing(kind.Null)

	case *workflowpb.Value_String_:
		return _Worded(kind.String_)

	case *workflowpb.Value_Int:
		return _Integer(kind.Int), nil

	case *workflowpb.Value_Float:
		return _Real(kind.Float)

	case *workflowpb.Value_Bytes:
		return _Binary(kind.Bytes), nil

	case *workflowpb.Value_Bool:
		return _Name(_Spelled(kind)), nil

	case *workflowpb.Value_List:
		return _List(
			kind.List,
			depth,
		)

	case *workflowpb.Value_Map:
		return _Dict(
			kind.Map,
			depth,
		)

	default:
		return nil, fmt.Errorf(
			"value has no kind (%T): %w",
			value.GetKind(),
			ErrForm,
		)
	}
}

// _Nothing is None, when the null kind holds null and nothing else.
//
// Revisions:
//   - 2026-11-13 13:05: initial creation, out of _Deeper (C132, Q439)
func _Nothing(held structpb.NullValue) (syntax.Expr, error) {
	if held != structpb.NullValue_NULL_VALUE {
		return nil, fmt.Errorf(
			"null holds %d, which is out of range: %w",
			held,
			ErrForm,
		)
	}

	return _Name(NONE), nil
}

// _Worded is a string value as the string literal it is. The text must be valid
// UTF-8, as the text of a flow is.
//
// Revisions:
//   - 2026-11-13 13:05: initial creation, out of _Deeper (C132, Q436)
func _Worded(text string) (syntax.Expr, error) {
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf(
			"string is not valid UTF-8: %w",
			ErrForm,
		)
	}

	return _Text(strconv.Quote(text)), nil
}

// _Spelled is the word a bool is written as.
//
// Revisions:
//   - 2026-11-13 13:05: initial creation, out of _Deeper (C132, Q407)
func _Spelled(kind *workflowpb.Value_Bool) string {
	if kind.Bool {
		return TRUE
	}

	return FALSE
}

// _Integer is an int value as the integer literal it is. The number is an int64
// and the literal is its decimal digits, so no text of a flow reaches the script
// here: the range and the digits were checked by the decoder, which reads an int
// from text or from a number and refuses one outside the range (Q438, Q440).
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q440)
func _Integer(number int64) syntax.Expr {
	return _Num(strconv.FormatInt(
		number,
		DECIMAL,
	))
}

// _Real is a float value as the float literal it is. A float with no point and no
// exponent gains ".0", so a whole float is not read back as an int. NaN and the
// infinities have no literal in Starlark, so they are refused.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421)
func _Real(number float64) (syntax.Expr, error) {
	if math.IsNaN(number) || math.IsInf(
		number,
		0,
	) {
		return nil, fmt.Errorf(
			"float is not finite: %w",
			ErrForm,
		)
	}

	raw := strconv.FormatFloat(
		number,
		'g',
		FRACTION,
		BITS,
	)

	if !strings.ContainsAny(
		raw,
		".e",
	) {
		raw += ".0"
	}

	return &syntax.Literal{
		Token: syntax.FLOAT,
		Raw:   raw,
	}, nil
}

// _Binary is a bytes value as a bytes literal, written in ASCII: a byte that is a
// printable ASCII character stands for itself, and every other byte, a quote and a
// backslash among them, is \x and two hex digits. Any bytes survive, whether or not
// they are valid UTF-8.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421)
func _Binary(data []byte) *syntax.Literal {
	var raw strings.Builder

	raw.WriteString(`b"`)

	for _, one := range data {
		if one >= ' ' && one <= '~' && one != '"' && one != '\\' {
			raw.WriteByte(one)

			continue
		}

		raw.WriteString(`\x`)
		raw.WriteByte(HEX[one>>4])
		raw.WriteByte(HEX[one&0xf])
	}

	raw.WriteByte('"')

	return &syntax.Literal{
		Token: syntax.BYTES,
		Raw:   raw.String(),
	}
}

// _List is a list value as a Starlark list.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the list node
//   - 2026-11-02 10:25: moved to value_write.go
//   - 2026-11-13 10:15: takes a workflow.List and the depth it starts at (C132, Q2, Q407)
func _List(
	list *workflowpb.List,
	depth int,
) (*syntax.ListExpr, error) {
	var items []syntax.Expr

	for _, item := range list.GetItems() {
		value, err := _Deeper(
			item,
			depth+1,
		)
		if err != nil {
			return nil, err
		}

		items = append(
			items,
			value,
		)
	}

	return &syntax.ListExpr{
		List: items,
	}, nil
}

// _Dict is a map value as a Starlark dict.
//
// Keys are written in sorted order. A map has none of its own, and a
// generator whose output changes between runs is one whose result cannot be
// compared.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the dict node
//   - 2026-11-02 10:25: moved to value_write.go
//   - 2026-11-13 10:15: renamed from _Struct, and takes a workflow.Map and the depth it starts at
//     (C132, Q2, Q407, Q423)
func _Dict(
	entries *workflowpb.Map,
	depth int,
) (*syntax.DictExpr, error) {
	var written []syntax.Expr

	for _, key := range slices.Sorted(maps.Keys(entries.GetEntries())) {
		if !utf8.ValidString(key) {
			return nil, fmt.Errorf(
				"map key is not valid UTF-8: %w",
				ErrForm,
			)
		}

		value, err := _Deeper(
			entries.GetEntries()[key],
			depth+1,
		)
		if err != nil {
			return nil, err
		}

		written = append(
			written,
			&syntax.DictEntry{
				Key:   _Text(strconv.Quote(key)),
				Value: value,
			},
		)
	}

	return &syntax.DictExpr{
		List: written,
	}, nil
}

// Whole is an int literal that range can take. A float is not a count, and nor is
// anything that is no int.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-11-02 10:25: moved to value_write.go
//   - 2026-11-10 01:00: comment now names Whole, were _Whole (C120a, Q314)
//   - 2026-11-13 10:15: takes an int value (C132, Q407, Q421, Q440)
func _Whole(value *workflowpb.Value) (string, error) {
	number, ok := value.GetKind().(*workflowpb.Value_Int)
	if !ok {
		return "", fmt.Errorf(
			"loop: %w",
			ErrForm,
		)
	}

	return strconv.FormatInt(
		number.Int,
		DECIMAL,
	), nil
}
