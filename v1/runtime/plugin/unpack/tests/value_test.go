package tests

import (
	"errors"
	"math"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestFromProto_Kinds checks every kind of value, and the number rule: a whole
// number inside the int64 range is an int, anything else is a float.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation
//   - 2026-11-02 11:24: renamed to match FromProto
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromProto_Kinds(t *testing.T) {
	cases := []struct {
		name  string
		given *structpb.Value
		want  string
		kind  string
	}{
		{
			"null",
			structpb.NewNullValue(),
			"None",
			"NoneType",
		},
		{
			"unset value",
			new(structpb.Value),
			"None",
			"NoneType",
		},
		{
			"nil value",
			nil,
			"None",
			"NoneType",
		},
		{
			"true",
			structpb.NewBoolValue(true),
			"True",
			"bool",
		},
		{
			"string",
			structpb.NewStringValue("a"),
			`"a"`,
			"string",
		},
		{
			"whole",
			structpb.NewNumberValue(3),
			"3",
			"int",
		},
		{
			"fraction",
			structpb.NewNumberValue(0.25),
			"0.25",
			"float",
		},
		{
			"negative whole",
			structpb.NewNumberValue(-7),
			"-7",
			"int",
		},
		{
			"negative zero",
			structpb.NewNumberValue(math.Copysign(
				0,
				-1,
			)),
			"0",
			"int",
		},
		{
			"2^53",
			structpb.NewNumberValue(1 << 53),
			"9007199254740992",
			"int",
		},
		{
			"2^53+2",
			structpb.NewNumberValue(1<<53 + 2),
			"9007199254740994",
			"int",
		},
		{
			"largest below 2^63",
			structpb.NewNumberValue(math.Nextafter(
				1<<63,
				0,
			)),
			"9223372036854774784",
			"int",
		},
		{
			"2^63",
			structpb.NewNumberValue(1 << 63),
			"9.223372036854776e+18",
			"float",
		},
		{
			"-2^63",
			structpb.NewNumberValue(-(1 << 63)),
			"-9223372036854775808",
			"int",
		},
		{
			"below -2^63",
			structpb.NewNumberValue(math.Nextafter(
				-(1 << 63),
				math.Inf(-1),
			)),
			"-9.223372036854778e+18",
			"float",
		},
		{
			"1e300",
			structpb.NewNumberValue(1e300),
			"1e+300",
			"float",
		},
		{
			"+Inf",
			structpb.NewNumberValue(math.Inf(1)),
			"+inf",
			"float",
		},
		{
			"-Inf",
			structpb.NewNumberValue(math.Inf(-1)),
			"-inf",
			"float",
		},
		{
			"NaN",
			structpb.NewNumberValue(math.NaN()),
			"nan",
			"float",
		},
		{
			"nested",
			structpb.NewListValue(&structpb.ListValue{
				Values: []*structpb.Value{
					structpb.NewNumberValue(1),
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"u": new(structpb.Value),
						},
					}),
				},
			}),
			`[1, {"u": None}]`,
			"list",
		},
		{
			"empty list",
			structpb.NewListValue(new(structpb.ListValue)),
			"[]",
			"list",
		},
		{
			"empty dict",
			structpb.NewStructValue(new(structpb.Struct)),
			"{}",
			"dict",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := unpack.FromProto(item.given)
				if err != nil {
					t.Fatalf(
						"unexpected error: %v",
						err,
					)
				}

				if got.String() != item.want {
					t.Fatalf(
						"got %s, want %s",
						got.String(),
						item.want,
					)
				}

				if got.Type() != item.kind {
					t.Fatalf(
						"type %s, want %s",
						got.Type(),
						item.kind,
					)
				}
			},
		)
	}
}

// TestFromProto_UnsetInsideAContainer checks that an unset element is None and
// not an error, at any depth.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation
//   - 2026-11-02 11:24: renamed to match FromProto
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromProto_UnsetInsideAContainer(t *testing.T) {
	var list = structpb.NewListValue(&structpb.ListValue{
		Values: []*structpb.Value{new(structpb.Value)},
	})

	got, err := unpack.FromProto(list)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "[None]" {
		t.Fatalf(
			"got %s",
			got.String(),
		)
	}
}

// TestFromProto_CycleAndDepth proves a list or a struct that holds itself
// returns trail.ErrCyclic, and nesting past common.DEPTH returns
// trail.ErrDeep, where the process ended with a stack overflow (B247, Q2,
// Q65, Q206).
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B247)
//   - 2026-11-10 17:15: uses ErrCyclic and ErrDeep, formerly ErrSelfContaining and ErrTooDeep
//     (C121, Q349)
func TestFromProto_CycleAndDepth(t *testing.T) {
	var (
		list = new(structpb.ListValue)
		self = &structpb.Value{
			Kind: &structpb.Value_ListValue{
				ListValue: list,
			},
		}
		fields = new(structpb.Struct)
		holder = &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: fields,
			},
		}
		deep = structpb.NewNullValue()
	)

	list.Values = []*structpb.Value{self}
	fields.Fields = map[string]*structpb.Value{"k": holder}

	for range common.DEPTH + 1 {
		deep = structpb.NewListValue(&structpb.ListValue{
			Values: []*structpb.Value{deep},
		})
	}

	cases := []struct {
		name  string
		given *structpb.Value
		want  error
	}{
		{
			"list holds itself",
			self,
			trail.ErrCyclic,
		},
		{
			"struct holds itself",
			holder,
			trail.ErrCyclic,
		},
		{
			"nested past the depth limit",
			deep,
			trail.ErrDeep,
		},
	}

	for _, test := range cases {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := unpack.FromProto(test.given)
				if !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}
			},
		)
	}

	var (
		shared = structpb.NewListValue(&structpb.ListValue{})
		_, err = unpack.FromProto(structpb.NewListValue(&structpb.ListValue{
			Values: []*structpb.Value{shared, shared},
		}))
	)
	if err != nil {
		t.Fatalf(
			"a value shared twice is no cycle: %v",
			err,
		)
	}
}
