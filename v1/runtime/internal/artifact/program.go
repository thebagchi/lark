package artifact

import (
	"encoding/binary"
	"fmt"
)

// The encoded program is the byte string go.starlark.net writes with
// Program.Write. Its layout is the comment at the top of
// internal/compile/serial.go in the pinned module version. This file walks
// that layout without building anything, so a count the decoder would
// allocate from is checked first.
//
// Clauses, with the serial.go line of the decoder step each one guards:
//
//	P1 magic "!sky" opens the data, and eight bytes of header follow (217-231).
//	P2 the string offset lies inside the data and not before byte 8 (231-235).
//	P3 the version varint equals PROGRAM_VERSION (237-239).
//	P4 every count is between 0 and the bytes still unread in the program
//	   section, since each element takes at least one byte (246, 252, 272,
//	   343, 351, 364).
//	P5 every string length is between 0 and the string bytes unread (329-332).
//	P6 a constant type is 0 to 4 (255-266).
//	P7 no byte of either section is left over (292).
//
// Every element takes at least one byte, so a walk that reaches its end has
// read at least as many bytes as it counted elements. Memory the decoder
// then allocates is linear in the input. It is measured, not derived: up to
// about 32 times the input (TotalAlloc, bigint constants), 28 times peak RSS,
// funcs 19.8 times, names 18 times, under `ulimit -v 8000000` with the probe
// poc/impl/probe/C94/probe_c94_test.go (C94 test round 1).
//
// Revisions:
//   - 2026-11-04 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	// PROGRAM_VERSION is compile.Version of the pinned go.starlark.net. A
	// newer module writes another number, Load refuses its bundles, and the
	// tests that Save and Load the fixtures fail. That is the way to find the
	// pin out of date.
	//
	// Revisions:
	//   - 2026-11-04 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	PROGRAM_VERSION = 14

	PROGRAM_MAGIC  = "!sky"
	PROGRAM_HEADER = 8
	CONSTANT_KINDS = 5

	// The constant types of serial.go, 255-266.
	//
	// Revisions:
	//   - 2026-11-04 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	KIND_STRING  = 0
	KIND_BYTES   = 1
	KIND_INT     = 2
	KIND_FLOAT   = 3
	KIND_BIGINT  = 4
	FUNCODE_INTS = 5
)

// PROGRAM_REFUSED opens every refusal _Bounded gives. It is text and not a
// sentinel error, as the interpreter's own decode errors are. The bytes of the
// program never enter a message.
//
// Revisions:
//   - 2026-11-04 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	PROGRAM_REFUSED = "the compiled program is malformed"
)

// _Walker reads the two sections of an encoded program, program first.
// Nothing in it grows with the input.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
type _Walker struct {
	prog    []byte // the program section not yet read
	strs    int    // the string bytes not yet claimed
	problem error
}

// _Bounded refuses code the pinned decoder could not read within memory
// proportional to its length. It allocates nothing that grows with the input.
// Returns nil for an encoded program of this format version whose counts and
// lengths agree with the bytes.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132, B126)
func _Bounded(code []byte) error {
	if len(code) < PROGRAM_HEADER ||
		string(code[:len(PROGRAM_MAGIC)]) != PROGRAM_MAGIC {
		return fmt.Errorf(
			"%s: no magic number (P1)",
			PROGRAM_REFUSED,
		)
	}

	offset := int64(binary.LittleEndian.Uint32(code[4:PROGRAM_HEADER]))
	if offset < PROGRAM_HEADER || offset > int64(len(code)) {
		return fmt.Errorf(
			"%s: string offset outside the data (P2)",
			PROGRAM_REFUSED,
		)
	}

	walker := &_Walker{
		prog: code[PROGRAM_HEADER:offset],
		strs: len(code) - int(offset),
	}

	walker._Program()

	if walker.problem != nil {
		return walker.problem
	}

	if len(walker.prog) != 0 || walker.strs != 0 {
		return fmt.Errorf(
			"%s: data left over (P7)",
			PROGRAM_REFUSED,
		)
	}

	return nil
}

// _Program walks the whole program, in the order serial.go writes it.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Program() {
	version, _ := w._Varint("version")
	if w.problem == nil && version != PROGRAM_VERSION {
		w._Refuse("version differs from the pinned one (P3)")
	}

	w._String()
	w._Bindings()
	w._Names()
	w._Constants()
	w._Bindings()
	w._Function()

	for range w._Count() {
		w._Function()
	}

	w._Varint("recursion")
}

// _Names walks a count of strings.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Names() {
	for range w._Count() {
		w._String()
	}
}

// _Constants walks a count of typed constants.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Constants() {
	for range w._Count() {
		kind, _ := w._Varint("constant type")
		if w.problem == nil && (kind < 0 || kind >= CONSTANT_KINDS) {
			w._Refuse("constant type unknown (P6)")
		}

		switch kind {
		case KIND_STRING, KIND_BYTES, KIND_BIGINT:
			w._String()
		case KIND_INT:
			w._Varint("constant")
		case KIND_FLOAT:
			w._Uvarint()
		}
	}
}

// _Function walks one Funcode.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Function() {
	w._Binding()
	w._String() // doc
	w._String() // code

	for range w._Count() { // pclinetab
		w._Varint("line table")
	}

	w._Bindings() // locals

	for range w._Count() { // cells
		w._Varint("cell")
	}

	w._Bindings() // free variables

	for range FUNCODE_INTS {
		w._Varint("function field")
	}
}

// _Bindings walks a count of bindings.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Bindings() {
	for range w._Count() {
		w._Binding()
	}
}

// _Binding walks a name, a line and a column.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Binding() {
	w._String()
	w._Varint("line")
	w._Varint("column")
}

// _Count reads a count, and refuses one above the program bytes unread. It
// returns zero after a refusal, so the loops that call it end at once.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132, B126)
func (w *_Walker) _Count() int {
	value, ok := w._Varint("count")
	if !ok {
		return 0
	}

	if value < 0 || value > int64(len(w.prog)) {
		w._Refuse("count above the bytes left (P4)")

		return 0
	}

	return int(value)
}

// _String claims one string's length from the string section.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _String() {
	value, ok := w._Varint("string length")
	if !ok {
		return
	}

	if value < 0 || value > int64(w.strs) {
		w._Refuse("string length above the bytes left (P5)")

		return
	}

	w.strs -= int(value)
}

// _Varint reads a signed varint, and refuses an empty or overlong one. After
// a refusal it reads nothing and returns false.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Varint(what string) (int64, bool) {
	if w.problem != nil {
		return 0, false
	}

	value, size := binary.Varint(w.prog)
	if size <= 0 {
		w._Refuse(what + " is cut short or too long")

		return 0, false
	}

	w.prog = w.prog[size:]

	return value, true
}

// _Uvarint reads an unsigned varint, as a float constant is written.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Uvarint() {
	_, size := binary.Uvarint(w.prog)
	if size <= 0 {
		w._Refuse("float is cut short or too long")

		return
	}

	w.prog = w.prog[size:]
}

// _Refuse records the first problem only.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
func (w *_Walker) _Refuse(why string) {
	if w.problem == nil {
		w.problem = fmt.Errorf(
			"%s: %s",
			PROGRAM_REFUSED,
			why,
		)
	}
}
