package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
)

// _Namer is an interface a test value is held in.
//
// Revisions:
//   - 2026-11-11 00:37: initial creation (C122, B268, B267)
type _Namer interface {
	Name() string
}

// _Func is a function type with a method, as a value of a host may be.
//
// Revisions:
//   - 2026-11-11 00:37: initial creation (C122, B268, B267)
type _Func func() string

// Name calls the function, which panics when it is nil.
//
// Revisions:
//   - 2026-11-11 00:37: initial creation (C122, B268, B267)
func (f _Func) Name() string {
	return f()
}

// _Saying is a function that is there.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B268, B267)
func _Saying() string {
	return "x"
}

// TestAbsent_IsTrueForNothingOfAnyKind proves Absent finds a nil of every kind
// that can be nil, whether it is given as an interface of its own or as any,
// and finds nothing in a value that is there. The four copies of this check
// that it replaces each knew some of the kinds (B267, B268).
//
// Revisions:
//   - 2026-11-11 00:37: initial creation (C122, B268, B267)
func TestAbsent_IsTrueForNothingOfAnyKind(t *testing.T) {
	var (
		pointer *int
		slice   []string
		table   map[string]int
		fn      func()
		pipe    chan int
		wrapped _Namer = _Func(nil)
		naked   _Namer
		zero    int
		empty   = []string{}
		made    = map[string]int{}
		some    = 1
		named   = _Func(_Saying)
	)

	var tests = []struct {
		name  string
		value any
		want  bool
	}{
		{
			"nil",
			nil,
			true,
		},
		{
			"nil pointer",
			pointer,
			true,
		},
		{
			"nil slice",
			slice,
			true,
		},
		{
			"nil map",
			table,
			true,
		},
		{
			"nil func",
			fn,
			true,
		},
		{
			"nil channel",
			pipe,
			true,
		},
		{
			"nil func with a method, in an interface",
			wrapped,
			true,
		},
		{
			"nil interface",
			naked,
			true,
		},
		{
			"pointer to a value",
			&some,
			false,
		},
		{
			"empty slice that is made",
			empty,
			false,
		},
		{
			"empty map that is made",
			made,
			false,
		},
		{
			"zero int",
			zero,
			false,
		},
		{
			"empty string",
			"",
			false,
		},
		{
			"func with a method",
			named,
			false,
		},
		{
			"struct",
			struct{}{},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := nilcheck.Absent(tc.value)
				if got != tc.want {
					t.Fatalf(
						"Absent(%#v) got %v, want %v",
						tc.value,
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestAbsent_ReadsAnInterfaceByItsDynamicValue proves Absent called with a
// type argument that is an interface looks at what the interface holds, so a
// typed nil in it is found (B268).
//
// Revisions:
//   - 2026-11-11 00:37: initial creation (C122, B268, B267)
func TestAbsent_ReadsAnInterfaceByItsDynamicValue(t *testing.T) {
	var (
		held _Namer = _Func(nil)
		none _Namer
		some _Namer = _Func(_Saying)
	)

	if !nilcheck.Absent(held) || !nilcheck.Absent(none) || nilcheck.Absent(some) {
		t.Fatalf(
			"got %v %v %v, want true true false",
			nilcheck.Absent(held),
			nilcheck.Absent(none),
			nilcheck.Absent(some),
		)
	}
}
