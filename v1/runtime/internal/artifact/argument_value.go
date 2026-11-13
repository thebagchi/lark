package artifact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// BASE is the base an int argument is written in, and WIDTH the size in bits of the
// int and of the float it is read to.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437, Q438)
const (
	BASE  = 10
	WIDTH = 64
)

// _Valued is the arguments of a run as the Map of lark Values it is handed, from the
// plain values _Converted gives, by the strict rules of protobuf JSON (Q437, Q438):
//
//   - an integer type, and a json.Number written as an integer, is an exact int,
//     and one outside int64 is out of range (unpack.ErrRange);
//   - a float type, and a json.Number with a fraction or an exponent, is a float; NaN
//     and the infinities were refused by _Converted, and a number past float64 is
//     refused here;
//   - a string is text and must be valid UTF-8, a map key too; a string is never
//     read as base64, and a []byte is bytes, the one way a host gives them;
//   - nil is null, a []any is a list, a map[string]any is a map.
//
// A failure names the argument, quoted and cut (Q4), and wraps unpack.ErrRange or
// unpack.ErrConvert, or trail.ErrDeep for a value nested past common.DEPTH. The
// arguments are walked in the order of their names, so the first failure is the
// same on every run.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437, Q438)
func _Valued(plain map[string]any) (*workflowpb.Map, error) {
	made := &workflowpb.Map{
		Entries: make(
			map[string]*workflowpb.Value,
			len(plain),
		),
	}

	for _, key := range slices.Sorted(maps.Keys(plain)) {
		value, err := _Plain(
			plain[key],
			0,
		)
		if err == nil && !utf8.ValidString(key) {
			err = fmt.Errorf(
				"name is not valid UTF-8: %w",
				unpack.ErrConvert,
			)
		}

		if err != nil {
			return nil, fmt.Errorf(
				"argument %s: %w",
				unpack.QuotePrefix(key),
				err,
			)
		}

		made.Entries[key] = value
	}

	return made, nil
}

// _Plain is one plain value as the lark Value of its kind, depth levels of list and
// map inside the argument. The depth is no more than common.DEPTH, because _Converted
// has refused a value nested past it (trail.ErrDeep).
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437, Q438)
func _Plain(
	given any,
	depth int,
) (*workflowpb.Value, error) {
	value, found, err := _Simple(given)
	if found {
		return value, err
	}

	switch held := given.(type) {
	case json.Number:
		return _Numbered(held)

	case []any:
		return _Listed(
			held,
			depth,
		)

	case map[string]any:
		return _Mapped(
			held,
			depth,
		)
	}

	return _Whole(given)
}

// _Simple is a plain value that holds no other value as its lark Value, and whether
// it is one: nil, a bool, text, bytes and a float.
//
// Revisions:
//   - 2026-11-13 13:05: initial creation, out of _Plain (C132, Q437)
func _Simple(given any) (*workflowpb.Value, bool, error) {
	switch held := given.(type) {
	case nil:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Null{
				Null: structpb.NullValue_NULL_VALUE,
			},
		}, true, nil

	case bool:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Bool{
				Bool: held,
			},
		}, true, nil

	case string:
		if !utf8.ValidString(held) {
			return nil, true, fmt.Errorf(
				"text is not valid UTF-8: %w",
				unpack.ErrConvert,
			)
		}

		return &workflowpb.Value{
			Kind: &workflowpb.Value_String_{
				String_: held,
			},
		}, true, nil

	case []byte:
		return &workflowpb.Value{
			Kind: &workflowpb.Value_Bytes{
				Bytes: bytes.Clone(held),
			},
		}, true, nil

	case float32:
		return _Real(float64(held)), true, nil

	case float64:
		return _Real(held), true, nil
	}

	return nil, false, nil
}

// _Whole is a Go integer as an int: a signed one as it is, an unsigned one when it
// fits int64. Any other Go type is not a value an argument holds.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437, Q438)
func _Whole(given any) (*workflowpb.Value, error) {
	var number int64

	switch held := given.(type) {
	case int:
		number = int64(held)

	case int8:
		number = int64(held)

	case int16:
		number = int64(held)

	case int32:
		number = int64(held)

	case int64:
		number = held

	case uint:
		return _Unsigned(uint64(held))

	case uint8:
		number = int64(held)

	case uint16:
		number = int64(held)

	case uint32:
		number = int64(held)

	case uint64:
		return _Unsigned(held)

	default:
		return nil, fmt.Errorf(
			"a Go type %s is not a value an argument holds: %w",
			unpack.QuotePrefix(fmt.Sprintf(
				"%T",
				given,
			)),
			unpack.ErrConvert,
		)
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_Int{
			Int: number,
		},
	}, nil
}

// _Unsigned is an unsigned integer as an int, when it fits int64.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q438)
func _Unsigned(number uint64) (*workflowpb.Value, error) {
	if number > math.MaxInt64 {
		return nil, fmt.Errorf(
			"%d is out of range of an int64: %w",
			number,
			unpack.ErrRange,
		)
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_Int{
			Int: int64(number),
		},
	}, nil
}

// _Real is a float value.
//
// Revisions:
//   - 2026-11-13 13:00: initial creation (C132, Q437)
func _Real(number float64) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Float{
			Float: number,
		},
	}
}

// _Numbered is a json.Number as the kind its text says: an int when the text has
// no fraction and no exponent, and a float when it has one. The text must be a JSON
// number, so "+1", "0x10" and "1_0" are refused. An int outside int64 is
// unpack.ErrRange, a float outside float64 too.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437, Q438)
func _Numbered(given json.Number) (*workflowpb.Value, error) {
	text := string(given)

	if text == "" || !json.Valid([]byte(text)) ||
		(text[0] != '-' && (text[0] < '0' || text[0] > '9')) {
		return nil, fmt.Errorf(
			"%s is not a JSON number: %w",
			unpack.QuotePrefix(text),
			unpack.ErrConvert,
		)
	}

	if strings.ContainsAny(
		text,
		".eE",
	) {
		number, err := strconv.ParseFloat(
			text,
			WIDTH,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s is out of range of a float: %w",
				unpack.ShortText(text),
				unpack.ErrRange,
			)
		}

		return _Real(number), nil
	}

	number, err := strconv.ParseInt(
		text,
		BASE,
		WIDTH,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s is out of range of an int64: %w",
			unpack.ShortText(text),
			unpack.ErrRange,
		)
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_Int{
			Int: number,
		},
	}, nil
}

// _Listed is a list of plain values as a list value.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437)
func _Listed(
	items []any,
	depth int,
) (*workflowpb.Value, error) {
	list := &workflowpb.List{
		Items: make(
			[]*workflowpb.Value,
			0,
			len(items),
		),
	}

	for _, item := range items {
		value, err := _Plain(
			item,
			depth+1,
		)
		if err != nil {
			return nil, err
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
	}, nil
}

// _Mapped is a map of plain values as a map value.
//
// Revisions:
//   - 2026-11-13 12:49: initial creation (C132, Q437)
func _Mapped(
	entries map[string]any,
	depth int,
) (*workflowpb.Value, error) {
	made := &workflowpb.Map{
		Entries: make(
			map[string]*workflowpb.Value,
			len(entries),
		),
	}

	for _, key := range slices.Sorted(maps.Keys(entries)) {
		if !utf8.ValidString(key) {
			return nil, fmt.Errorf(
				"a key is not valid UTF-8: %w",
				unpack.ErrConvert,
			)
		}

		value, err := _Plain(
			entries[key],
			depth+1,
		)
		if err != nil {
			return nil, err
		}

		made.Entries[key] = value
	}

	return &workflowpb.Value{
		Kind: &workflowpb.Value_Map{
			Map: made,
		},
	}, nil
}
