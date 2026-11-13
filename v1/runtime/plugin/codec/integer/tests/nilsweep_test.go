package tests

import (
	"maps"
	"math"
	"slices"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestNilSweep_integer calls the plugin's methods on a nil and a zero receiver, and
// every builtin of its module with each nil or odd argument shape a host or a
// plugin author could pass (C87, Q2, Q85). A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-05 19:07: the too-wide field is math.MaxInt, for 32 bits (C58, B119)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestNilSweep_integer(t *testing.T) {
	t.Run(
		"nil and zero receivers",
		func(t *testing.T) {
			nilsweep.Methods(
				t,
				"integer",
			)
		},
	)

	members := nilsweep.Members(
		t,
		"integer",
	)

	for _, name := range slices.Sorted(maps.Keys(members)) {
		t.Run(
			name,
			func(t *testing.T) {
				nilsweep.Run(
					t,
					members[name],
				)
			},
		)
	}

	var (
		one      = starlark.MakeInt(1)
		nilField *common.Field
		nilFn    *starlark.Builtin
		zeroInt  = new(common.Field)
		unset    = &common.Field{
			Width: 8,
		}
		wide = &common.Field{
			Number: starlark.MakeInt(1),
			Width:  math.MaxInt,
		}
		negative = &common.Field{
			Number: starlark.MakeInt(1),
			Width:  -1,
		}
	)

	nilsweep.Rows(
		t,
		[]nilsweep.Row{
			{
				Name: "Unpack nil builtin",
				Call: func() error {
					_, err := integer.Unpack(
						nilFn,
						starlark.Tuple{one, one},
						nil,
					)

					return err
				},
			},
			{
				Name: "Unpack nil builtin, no arguments",
				Call: func() error {
					_, err := integer.Unpack(
						nilFn,
						nil,
						nil,
					)

					return err
				},
			},
			{
				Name: "Unpack no arguments",
				Call: func() error {
					_, err := integer.Unpack(
						starlark.NewBuiltin(
							"x",
							nil,
						),
						nil,
						nil,
					)

					return err
				},
			},
			{
				Name: "Encode nil field",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						nilField,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "EncodeSigned nil field",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"who",
						nilField,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "Encode zero field",
				Call: func() error {
					_, err := integer.Encode(
						"",
						zeroInt,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "EncodeSigned zero field",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"",
						zeroInt,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "Encode zero number, width 8",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						unset,
					)

					return err
				},
			},
			{
				Name: "EncodeSigned zero number, width 8",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"who",
						unset,
					)

					return err
				},
			},
			{
				Name: "Encode zero number, width 9",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						&common.Field{
							Width: 9,
						},
					)

					return err
				},
			},
			{
				Name: "EncodeSigned zero number, width 9",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"who",
						&common.Field{
							Width: 9,
						},
					)

					return err
				},
			},
			{
				Name: "Encode width past memory",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						wide,
					)

					return err
				},
				Want: memwatch.ErrRoom,
			},
			{
				Name: "EncodeSigned width past memory",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"who",
						wide,
					)

					return err
				},
			},
			{
				Name: "Encode negative width",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						negative,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "EncodeSigned negative width",
				Call: func() error {
					_, err := integer.EncodeSigned(
						"who",
						negative,
					)

					return err
				},
				Want: unpack.ErrRange,
			},
			{
				Name: "Encode empty order",
				Call: func() error {
					_, err := integer.Encode(
						"who",
						&common.Field{
							Number: starlark.MakeInt(1),
							Width:  12,
						},
					)

					return err
				},
				Want: integer.ErrOrder,
			},
			{
				Name: "Decode nil data",
				Call: func() error {
					_, err := integer.Decode(
						"who",
						nil,
						"",
					)

					return err
				},
				Want: integer.ErrOrder,
			},
			{
				Name: "DecodeSigned nil data",
				Call: func() error {
					_, err := integer.DecodeSigned(
						"who",
						nil,
						"",
					)

					return err
				},
				Want: integer.ErrOrder,
			},
			{
				Name: "Decode nil data, big order",
				Call: func() error {
					_, err := integer.Decode(
						"who",
						nil,
						common.BIG,
					)

					return err
				},
			},
			{
				Name: "DecodeSigned nil data, little order",
				Call: func() error {
					_, err := integer.DecodeSigned(
						"who",
						nil,
						common.LITTLE,
					)

					return err
				},
			},
			{
				Name: "Decode nine bytes, empty order",
				Call: func() error {
					_, err := integer.Decode(
						"who",
						make(
							[]byte,
							9,
						),
						"",
					)

					return err
				},
				Want: integer.ErrOrder,
			},
			{
				Name: "DecodeSigned nine bytes, empty order",
				Call: func() error {
					_, err := integer.DecodeSigned(
						"who",
						make(
							[]byte,
							9,
						),
						"",
					)

					return err
				},
				Want: integer.ErrOrder,
			},
		},
	)
}
