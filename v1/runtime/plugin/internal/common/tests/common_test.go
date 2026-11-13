package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	DECLARED = map[string]bool{"helper": true}

	INNER = starlark.NewBuiltin(
		"inner",
		func(
			_ *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			_ []starlark.Tuple,
		) (starlark.Value, error) {
			return starlark.MakeInt(len(args)), nil
		},
	)
)

// _Call calls the from_int builtin of base.
//
// Revisions:
//   - 2026-11-04 21:51: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Call(
	base int,
	args starlark.Tuple,
) (starlark.Value, error) {
	return starlark.Call(
		&starlark.Thread{},
		starlark.NewBuiltin(
			"from_int",
			shared.FromInt(base),
		),
		args,
		nil,
	)
}

// TestRefused_QuotesAndBoundsTheText proves the message shape, the sentinel in
// the chain, and the bound on a long text.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRefused_QuotesAndBoundsTheText(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{
			"short",
			"zz",
			`who got "zz": wants bytes or a string`,
		},
		{
			"control byte",
			"a\nb",
			`who got "a\nb": wants bytes or a string`,
		},
		{
			"long",
			strings.Repeat(
				"x",
				10*common.QUOTE_LIMIT,
			),
			"",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				err := shared.Refused(
					"who",
					item.text,
					unpack.ErrData,
				)
				if !errors.Is(
					err,
					unpack.ErrData,
				) {
					t.Fatalf(
						"sentinel lost: %v",
						err,
					)
				}

				if item.want != "" && err.Error() != item.want {
					t.Fatalf(
						"got %q, want %q",
						err.Error(),
						item.want,
					)
				}

				if len(err.Error()) > 4*common.QUOTE_LIMIT+100 {
					t.Fatalf(
						"message of %d bytes is not bounded",
						len(err.Error()),
					)
				}
			},
		)
	}
}

// TestWidth_RefusesBelowOne proves the sign check, its text and its sentinel.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWidth_RefusesBelowOne(t *testing.T) {
	cases := []struct {
		name  string
		width int
		want  string
	}{
		{
			"one",
			1,
			"",
		},
		{
			"many",
			1 << 20,
			"",
		},
		{
			"zero",
			0,
			"who got a width of 0: out of range",
		},
		{
			"negative",
			-3,
			"who got a width of -3: out of range",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				err := shared.Width(
					"who",
					item.width,
				)
				if item.want == "" {
					if err != nil {
						t.Fatalf(
							"got %v",
							err,
						)
					}

					return
				}

				if err == nil || err.Error() != item.want || !errors.Is(
					err,
					unpack.ErrRange,
				) {
					t.Fatalf(
						"got %v, want %q",
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestGuarded_ChecksThenCalls proves that a failed check is named and stops the
// call, and that a passed check leaves the inner result untouched.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuarded_ChecksThenCalls(t *testing.T) {
	guarded := shared.Guarded(
		INNER,
		func(
			_ *starlark.Thread,
			args starlark.Tuple,
		) error {
			if len(args) > 1 {
				return unpack.ErrData
			}

			return nil
		},
	)

	if guarded.Name() != "inner" {
		t.Fatalf(
			"name %q",
			guarded.Name(),
		)
	}

	got, err := starlark.Call(
		&starlark.Thread{},
		guarded,
		starlark.Tuple{starlark.None},
		nil,
	)
	if err != nil || got.String() != "1" {
		t.Fatalf(
			"passed call gave %v, %v",
			got,
			err,
		)
	}

	_, err = starlark.Call(
		&starlark.Thread{},
		guarded,
		starlark.Tuple{starlark.None, starlark.None},
		nil,
	)
	if !errors.Is(
		err,
		unpack.ErrData,
	) || !strings.Contains(
		err.Error(),
		"inner: wants bytes or a string",
	) {
		t.Fatalf(
			"failed call gave %v",
			err,
		)
	}
}

// TestFromInt_PadsInTheBase proves the digits, the padding and that a number
// wider than the width is not cut.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFromInt_PadsInTheBase(t *testing.T) {
	cases := []struct {
		name string
		base int
		args starlark.Tuple
		want string
	}{
		{
			"binary pad",
			2,
			starlark.Tuple{starlark.MakeInt(5), starlark.MakeInt(8)},
			"00000101",
		},
		{
			"hex pad",
			16,
			starlark.Tuple{starlark.MakeInt(255), starlark.MakeInt(4)},
			"00FF",
		},
		{
			"wider than width",
			16,
			starlark.Tuple{starlark.MakeInt(0xabcd), starlark.MakeInt(1)},
			"ABCD",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Call(
					item.base,
					item.args,
				)
				if err != nil || got != starlark.String(item.want) {
					t.Fatalf(
						"got %v, %v, want %q",
						got,
						err,
						item.want,
					)
				}
			},
		)
	}

	_, err := _Call(
		16,
		starlark.Tuple{starlark.MakeInt(-1), starlark.MakeInt(2)},
	)
	if !errors.Is(
		err,
		unpack.ErrRange,
	) {
		t.Fatalf(
			"a negative number gave %v",
			err,
		)
	}
}

// TestNames_SaysWhatAnArgumentPlainlyIs proves that parentheses do not change
// the answer, and that an unknown name answers empty.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation (C59)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNames_SaysWhatAnArgumentPlainlyIs(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{
			"lambda",
			"lambda: 1",
			"a lambda",
		},
		{
			"wrapped lambda",
			"((lambda: 1))",
			"a lambda",
		},
		{
			"declared",
			"helper",
			"the function helper",
		},
		{
			"wrapped declared",
			"(helper)",
			"the function helper",
		},
		{
			"undeclared",
			"other",
			"",
		},
		{
			"call",
			"helper()",
			"",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				expr, err := new(syntax.FileOptions).ParseExpr(
					"x",
					item.source,
					0,
				)
				if err != nil {
					t.Fatal(err)
				}

				got := shared.Names(
					expr,
					DECLARED,
				)
				if got != item.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						item.want,
					)
				}

				_, paren := shared.Bare(expr).(*syntax.ParenExpr)
				if paren {
					t.Fatal("Bare left a parenthesis")
				}
			},
		)
	}
}
