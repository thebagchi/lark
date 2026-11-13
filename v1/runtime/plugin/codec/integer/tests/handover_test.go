package tests

import (
	"math/big"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"
)

const (
	// HANDOVER_ROUNDS is how many different inputs each site is called on.
	//
	// Revisions:
	//   - 2026-11-03 22:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HANDOVER_ROUNDS = 3

	// HANDOVER_SIZE is how many bytes of data each input holds.
	//
	// Revisions:
	//   - 2026-11-03 22:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HANDOVER_SIZE = 1024
)

// _Variant is HANDOVER_SIZE bytes that differ from every other variant.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Variant(round int) []byte {
	data := make(
		[]byte,
		HANDOVER_SIZE,
	)

	for idx := range data {
		data[idx] = byte(idx*7 + round*31 + 1)
	}

	return data
}

// _Chars is the text of a string or bytes value.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Chars(
	tb testing.TB,
	value starlark.Value,
) string {
	tb.Helper()

	switch held := value.(type) {
	case starlark.String:
		return string(held)

	case starlark.Bytes:
		return string(held)
	}

	tb.Fatalf(
		"got %s, want a string or bytes",
		value.Type(),
	)

	return ""
}

// _Disjoint fails the test when a result lost the text it was given, or shares
// bytes with an earlier result. Either is a write to a buffer that was handed
// over.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Disjoint(
	t *testing.T,
	results []starlark.Value,
	want func(round int) string,
) {
	t.Helper()

	for round, value := range results {
		got := _Chars(
			t,
			value,
		)

		if got != want(round) {
			t.Fatalf(
				"round %d: got %d bytes (%.16q), want %d",
				round,
				len(got),
				got,
				len(want(round)),
			)
		}

		for other := range round {
			if unsafe.StringData(got) == unsafe.StringData(_Chars(
				t,
				results[other],
			)) {
				t.Fatalf(
					"rounds %d and %d share one buffer",
					other,
					round,
				)
			}
		}
	}
}

// TestHandover_IntegerNeverWritesAHandedOverBuffer proves, for each builtin of
// the module that hands over a buffer, the rule that makes the handover safe:
// nothing writes the buffer after the call. Each builtin runs on three inputs,
// and then runs on all three again. A result must keep the text it was given
// and must not share bytes with another result. A builtin that reused one
// buffer, or wrote one after the call, would fail the second check, or the
// first.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHandover_IntegerNeverWritesAHandedOverBuffer(t *testing.T) {
	cases := []struct {
		name string
		fn   string
		args func(round int) starlark.Tuple
		want func(round int) string
	}{
		{
			"to_bytes 8",
			"to_bytes",
			func(round int) starlark.Tuple {
				return starlark.Tuple{
					starlark.MakeBigInt(
						new(big.Int).SetBytes(_Variant(round)[:8]),
					),
					starlark.MakeInt(8),
				}
			},
			func(round int) string {
				return string(_Variant(round)[:8])
			},
		},
		{
			"to_bytes 16",
			"to_bytes",
			func(round int) starlark.Tuple {
				return starlark.Tuple{
					starlark.MakeBigInt(
						new(big.Int).SetBytes(_Variant(round)[:16]),
					),
					starlark.MakeInt(16),
				}
			},
			func(round int) string {
				return string(_Variant(round)[:16])
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						tc.fn,
					)
					thread = &starlark.Thread{
						Name: "handover",
					}
					results [HANDOVER_ROUNDS]starlark.Value
				)

				call := func(round int) starlark.Value {
					value, err := fn.CallInternal(
						thread,
						tc.args(round),
						nil,
					)
					if err != nil {
						t.Fatal(err)
					}

					return value
				}

				for round := range HANDOVER_ROUNDS {
					results[round] = call(round)
				}

				// A second pass: a builtin that kept a buffer would write it now.
				for round := range HANDOVER_ROUNDS {
					call(round)
				}

				_Disjoint(
					t,
					results[:],
					tc.want,
				)
			},
		)
	}
}

// TestHandover_IntegerMakesOneBufferPerCall holds each builtin that hands a
// buffer over to the allocations of the handover: the call is boxed, the
// buffer is made once, and there is no second copy of it.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHandover_IntegerMakesOneBufferPerCall(t *testing.T) {
	cases := []struct {
		name string
		fn   string
		args func(round int) starlark.Tuple
		max  float64
	}{
		{
			"to_bytes 8",
			"to_bytes",
			func(round int) starlark.Tuple {
				return starlark.Tuple{
					starlark.MakeBigInt(
						new(big.Int).SetBytes(_Variant(round)[:8]),
					),
					starlark.MakeInt(8),
				}
			},
			8,
		},
		{
			"to_bytes 16",
			"to_bytes",
			func(round int) starlark.Tuple {
				return starlark.Tuple{
					starlark.MakeBigInt(
						new(big.Int).SetBytes(_Variant(round)[:16]),
					),
					starlark.MakeInt(16),
				}
			},
			8,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						tc.fn,
					)
					thread = &starlark.Thread{
						Name: "handover",
					}
					args = tc.args(0)
				)

				got := testing.AllocsPerRun(
					20,
					func() {
						_, err := fn.CallInternal(
							thread,
							args,
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}
					},
				)
				if got > tc.max {
					t.Fatalf(
						"%v allocations, want at most %v",
						got,
						tc.max,
					)
				}
			},
		)
	}
}
