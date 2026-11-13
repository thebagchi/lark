package clone

import (
	"errors"

	"go.starlark.net/starlark"
)

// Missing stands for a nil starlark.Value that Walk found where a value
// belongs: an element of a list, a tuple or a dict, or the value itself.
//
// A script cannot build one; a Go host can, and Freeze of the container that
// holds it dereferences it. Walk hands it back as what is not data, so a caller
// can say what was wrong with Type, as it does for any other value.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
type Missing struct{}

// String is the text of a value that is not there.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
func (m *Missing) String() string {
	return "nil"
}

// Type names the type of a missing value: nil.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
func (m *Missing) Type() string {
	return "nil"
}

// Freeze is empty: there is nothing to freeze.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
func (m *Missing) Freeze() {
	// Empty
}

// Truth reports a missing value as false.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
func (m *Missing) Truth() starlark.Bool {
	return starlark.False
}

// Hash refuses: a missing value is not a key.
//
// Revisions:
//   - 2026-11-13 09:37: initial creation (C131, B293, Q429)
func (m *Missing) Hash() (uint32, error) {
	return 0, errors.New("nil is unhashable")
}
