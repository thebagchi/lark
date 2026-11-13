package unpack

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/proto"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

var (
	// ErrConvert is proto.ErrConvert.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotProtoValue (C121, Q349)
	ErrConvert = proto.ErrConvert
)

// FromProto returns a protobuf value as a script sees it.
//
// The remote plugin uses it for remote results: a whole number inside the int64
// range is an int and any other number is a float; a nil, unset or null value
// is None; and an error from a dict names the key. A failure wraps ErrConvert.
// The args plugin does not: an argument is a lark Value, read by FromValue.
//
// The walk has a trail, as every walk over a value has (Q2). A list or a
// struct that holds itself returns trail.ErrCyclic, and nesting deeper
// than common.DEPTH returns trail.ErrDeep, which neither wraps
// ErrConvert. Before the trail a cyclic value ended the process with a
// stack overflow.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation, merged from args and remote
//   - 2026-11-02 11:24: renamed to FromProto and ErrNotProtoValue
//   - 2026-11-10 11:53: walks with a trail, so a cycle or a value nested past
//     common.DEPTH is an error, not a stack overflow (C117, B247, Q2, Q65, Q206)
//   - 2026-11-10 17:15: uses ErrConvert and ErrCyclic and ErrDeep, formerly ErrNotProtoValue and
//     ErrSelfContaining and ErrTooDeep (C121, Q349)
//   - 2026-11-13 12:48: the args plugin reads FromValue, so the remote plugin alone uses it
//     (C132, Q437)
func FromProto(value *structpb.Value) (starlark.Value, error) {
	return _Converted(
		trail.New(context.Background()),
		value,
	)
}

// _Converted is value as a script sees it, with every list and struct entered
// on path.
//
// Revisions:
//   - 2026-11-10 11:53: initial creation, from the body of FromProto (C117, B247)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Converted(
	path *common.Trail,
	value *structpb.Value,
) (starlark.Value, error) {
	switch held := value.GetKind().(type) {
	case *structpb.Value_NullValue, nil:
		return starlark.None, nil

	case *structpb.Value_BoolValue:
		return starlark.Bool(held.BoolValue), nil

	case *structpb.Value_NumberValue:
		return _Number(held.NumberValue), nil

	case *structpb.Value_StringValue:
		return starlark.String(held.StringValue), nil

	case *structpb.Value_ListValue:
		return _List(
			path,
			held.ListValue,
		)

	case *structpb.Value_StructValue:
		return _Dict(
			path,
			held.StructValue,
		)
	}

	return nil, fmt.Errorf(
		"%T: %w",
		value.GetKind(),
		ErrConvert,
	)
}

// _Number is a number as a script sees it.
//
// A whole number is an int, because JSON has one number type and Starlark has
// two. The range is checked before the conversion, because the Go spec leaves
// an out-of-range float-to-int conversion to the implementation: 2^63 gives
// MaxInt64 on arm64. NaN fails both comparisons and stays a float.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation, with the range check before the conversion
//   - 2026-11-02 11:34: is the shared number rule, from args _Number; BOUND is local
func _Number(number float64) starlark.Value {
	// BOUND is 2^63. A float64 holds it exactly and MaxInt64 it does not, so
	// a whole n fits an int64 exactly when -BOUND <= n < BOUND.
	const BOUND = 1 << 63

	whole := number == math.Trunc(number)
	if whole && number >= -BOUND && number < BOUND {
		return starlark.MakeInt64(int64(number))
	}

	return starlark.Float(number)
}

// _List converts a list, one element at a time.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation, merged from args and remote
//   - 2026-11-02 11:34: is the shared list helper, from args and remote
//   - 2026-11-10 11:53: enters values on the trail (C117, B247)
func _List(
	path *common.Trail,
	values *structpb.ListValue,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		values,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	var (
		parts = values.GetValues()
		into  = make(
			[]starlark.Value,
			0,
			len(parts),
		)
	)

	for _, part := range parts {
		one, err := _Converted(
			path,
			part,
		)
		if err != nil {
			return nil, err
		}

		into = append(
			into,
			one,
		)
	}

	return starlark.NewList(into), nil
}

// _Dict converts a struct, one field at a time.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation, merged from args and remote
//   - 2026-11-02 11:34: is the shared dict helper, from args and remote
//   - 2026-11-10 11:32: uses proto.Set, which moved to internal/proto (C120b, Q315)
//   - 2026-11-10 11:53: enters fields on the trail (C117, B247)
func _Dict(
	path *common.Trail,
	fields *structpb.Struct,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		fields,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	into := starlark.NewDict(len(fields.GetFields()))

	for key, part := range fields.GetFields() {
		one, err := _Converted(
			path,
			part,
		)
		if err != nil {
			return nil, err
		}

		err = proto.Set(
			into,
			key,
			one,
		)
		if err != nil {
			return nil, err
		}
	}

	return into, nil
}

// FromValue returns a value of a flow as a script sees it.
//
// The args plugin uses it for script arguments. Each kind is its own value, as the
// strict rules of protobuf JSON have it: an int is an int and a float is a float,
// so 1 and 1.0 stay two values; bytes are bytes; a map is a dict whose keys are
// inserted in sorted order. A Value with no kind is not a value, and a null kind
// that holds anything but null is out of range: both wrap ErrConvert.
//
// The walk has a trail, as every walk over a value has (Q2): a list or a map that
// holds itself returns trail.ErrCyclic, and nesting deeper than common.DEPTH
// returns trail.ErrDeep. A converted value is new on each call, so two runs never
// share a list.
//
// Revisions:
//   - 2026-11-13 12:48: initial creation (C132, Q2, Q437)
func FromValue(value *workflowpb.Value) (starlark.Value, error) {
	return _Lowered(
		trail.New(context.Background()),
		value,
	)
}

// _Lowered is value as a script sees it, with every list and map entered on path.
//
// Revisions:
//   - 2026-11-13 12:48: initial creation (C132, Q2, Q437)
func _Lowered(
	path *common.Trail,
	value *workflowpb.Value,
) (starlark.Value, error) {
	switch held := value.GetKind().(type) {
	case *workflowpb.Value_Null:
		if held.Null != structpb.NullValue_NULL_VALUE {
			return nil, fmt.Errorf(
				"null holds %d: %w",
				held.Null,
				ErrConvert,
			)
		}

		return starlark.None, nil

	case *workflowpb.Value_Bool:
		return starlark.Bool(held.Bool), nil

	case *workflowpb.Value_Int:
		return starlark.MakeInt64(held.Int), nil

	case *workflowpb.Value_Float:
		return starlark.Float(held.Float), nil

	case *workflowpb.Value_String_:
		return starlark.String(held.String_), nil

	case *workflowpb.Value_Bytes:
		return starlark.Bytes(held.Bytes), nil

	case *workflowpb.Value_List:
		return _Listing(
			path,
			held.List,
		)

	case *workflowpb.Value_Map:
		return _Mapping(
			path,
			held.Map,
		)
	}

	return nil, fmt.Errorf(
		"value has no kind (%T): %w",
		value.GetKind(),
		ErrConvert,
	)
}

// _Listing converts a list of a flow, one element at a time.
//
// Revisions:
//   - 2026-11-13 12:48: initial creation (C132, Q2, Q437)
func _Listing(
	path *common.Trail,
	values *workflowpb.List,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		values,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	into := make(
		[]starlark.Value,
		0,
		len(values.GetItems()),
	)

	for _, part := range values.GetItems() {
		one, err := _Lowered(
			path,
			part,
		)
		if err != nil {
			return nil, err
		}

		into = append(
			into,
			one,
		)
	}

	return starlark.NewList(into), nil
}

// _Mapping converts a map of a flow, one entry at a time, in the order of its keys.
//
// Revisions:
//   - 2026-11-13 12:48: initial creation (C132, Q2, Q437)
func _Mapping(
	path *common.Trail,
	fields *workflowpb.Map,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		fields,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	into := starlark.NewDict(len(fields.GetEntries()))

	for _, key := range slices.Sorted(maps.Keys(fields.GetEntries())) {
		one, err := _Lowered(
			path,
			fields.GetEntries()[key],
		)
		if err != nil {
			return nil, err
		}

		err = proto.Set(
			into,
			key,
			one,
		)
		if err != nil {
			return nil, err
		}
	}

	return into, nil
}
