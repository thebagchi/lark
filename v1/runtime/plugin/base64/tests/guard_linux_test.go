//go:build linux

package tests

import (
	"bytes"
	"syscall"
	"testing"
)

const (
	// PAGE is the size of a page that the guard test maps and protects. 4096 is
	// the page size of every Linux target that this repository builds for.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PAGE = 4096
)

// _Region maps a page that nothing may touch, two pages of data, and another
// page that nothing may touch, and returns the data. A read or a write of one
// byte past either end of a slice placed against the data stops the test run
// with a fault, so a kernel's over-read or over-write is found at once, not
// only when it changes a guard byte.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Region(t *testing.T) []byte {
	t.Helper()

	memory, err := syscall.Mmap(
		-1,
		0,
		4*PAGE,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Skipf(
			"mmap: %v",
			err,
		)
	}

	t.Cleanup(func() {
		_ = syscall.Munmap(memory)
	})

	for _, guard := range [][]byte{memory[:PAGE], memory[3*PAGE:]} {
		err = syscall.Mprotect(
			guard,
			syscall.PROT_NONE,
		)
		if err != nil {
			t.Fatalf(
				"mprotect: %v",
				err,
			)
		}
	}

	return memory[PAGE : 3*PAGE]
}

// _AtEnd is a slice of size bytes that ends at the last byte of the data, and
// _AtStart one that begins at the first. The capacity is the length.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _AtEnd(
	region []byte,
	size int,
) []byte {
	return region[len(region)-size : len(region) : len(region)]
}

// _AtStart is a slice of size bytes that begins at the first byte of the data.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _AtStart(
	region []byte,
	size int,
) []byte {
	return region[:size:size]
}

// _Guard is two regions with an unmapped page at each end, and the way a slice
// is placed in them: against the first byte or against the last.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Guard struct {
	Source []byte
	Target []byte
	Place  func(
		[]byte,
		int,
	) []byte
}

// TestKernels_TouchNothingPastASlice proves no kernel reads or writes a byte
// outside its source and its destination. The source and the destination are
// each placed so that they end at an unmapped page, and again so that they
// begin at one, for every size 0 to LONG, at every level and in both
// alphabets. A byte outside either slice is a fault. The output is also
// compared with the reference, so the test is no weaker than the sweeps.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_TouchNothingPastASlice(t *testing.T) {
	var (
		source = _Region(t)
		target = _Region(t)
		places = map[string]func(
			[]byte,
			int,
		) []byte{
			"end":   _AtEnd,
			"start": _AtStart,
		}
	)

	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			for name, place := range places {
				t.Run(
					_Name(
						level,
						alphabet,
					)+"/"+name,
					func(t *testing.T) {
						var (
							guard = &_Guard{
								source,
								target,
								place,
							}
							mode = &_Mode{
								level,
								alphabet,
							}
						)

						for size := 0; size <= LIMIT; size++ {
							guard.Encode(
								t,
								mode,
								size,
							)
							guard.Decode(
								t,
								mode,
								size,
							)
						}
					},
				)
			}
		}
	}
}

// Encode encodes size bytes, placed in the source, to a destination of the
// exact size, and compares with the reference.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (g *_Guard) Encode(
	t *testing.T,
	mode *_Mode,
	size int,
) {
	t.Helper()

	src := g.Place(
		g.Source,
		size,
	)

	copy(
		src,
		_Random(
			uint64(size),
			size,
		),
	)

	var (
		want = _Reference(
			mode.Alphabet,
			src,
		)
		dst = g.Place(
			g.Target,
			len(want),
		)
	)

	EncodeAt(
		mode.Level,
		mode.Alphabet,
		dst,
		src,
	)

	if !bytes.Equal(
		dst,
		want,
	) {
		t.Fatalf(
			"encode size %d: wrong text",
			size,
		)
	}
}

// Decode decodes the bare text of size bytes of data, with the text and the
// bytes placed against a guard, and compares with the data.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (g *_Guard) Decode(
	t *testing.T,
	mode *_Mode,
	size int,
) {
	t.Helper()

	var (
		data = _Random(
			uint64(size),
			size,
		)
		bare = _Bare(
			mode.Alphabet,
			data,
		)
		text = g.Place(
			g.Source,
			len(bare),
		)
		dst = g.Place(
			g.Target,
			len(data),
		)
	)

	copy(
		text,
		bare,
	)

	count, ok := DecodeAt(
		mode.Level,
		mode.Alphabet,
		dst,
		text,
	)

	if !ok || !bytes.Equal(
		dst[:count],
		data,
	) {
		t.Fatalf(
			"decode size %d: wrong bytes or refused",
			size,
		)
	}
}
