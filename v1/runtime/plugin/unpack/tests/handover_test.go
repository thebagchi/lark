package tests

import (
	"testing"
	"unsafe"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestHandover_SharesTheBuffer proves that Handover makes no copy: the text
// starts at the first byte of the buffer, and has its length. A string
// conversion would start elsewhere.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHandover_SharesTheBuffer(t *testing.T) {
	cases := map[string][]byte{
		"empty":  {},
		"nil":    nil,
		"one":    []byte("a"),
		"text":   []byte("hello, world"),
		"binary": {0x00, 0xff, 0x80, 0x7f},
		"cap > len": append(
			make(
				[]byte,
				0,
				64,
			),
			"abc"...,
		),
	}

	for name, buffer := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					want = string(buffer)
					got  = unpack.Handover(buffer)
				)

				if got != want {
					t.Fatalf(
						"got %q, want %q",
						got,
						want,
					)
				}

				copied := unsafe.StringData(got) != unsafe.SliceData(buffer)
				if len(buffer) > 0 && copied {
					t.Fatal("the text is a copy, not the buffer")
				}
			},
		)
	}
}

// TestHandover_SeesALaterWrite shows the rule by breaking it: a write to the
// buffer after the handover changes the text. A caller that passes a buffer it
// writes again corrupts a value that a script holds. Each site's own test
// proves that its buffer is made for the call and is not written again.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHandover_SeesALaterWrite(t *testing.T) {
	var (
		buffer = []byte("abc")
		got    = unpack.Handover(buffer)
	)

	buffer[0] = 'X'

	if got != "Xbc" {
		t.Fatalf(
			"got %q: the text did not follow the buffer",
			got,
		)
	}
}

// TestHandover_AllocatesNothing holds Handover to no allocation.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHandover_AllocatesNothing(t *testing.T) {
	var (
		buffer = make(
			[]byte,
			1024,
		)
		got = testing.AllocsPerRun(
			100,
			func() {
				_ = unpack.Handover(buffer)
			},
		)
	)

	if got != 0 {
		t.Fatalf(
			"got %v allocations, want 0",
			got,
		)
	}
}
