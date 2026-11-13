package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLines_StringCutsALongPath proves the text of the value lines returns
// holds the path cut to 100 bytes, as an error does (Q4, B273). A file name is
// at most 255 bytes long, so a name of 250 bytes is the longest a test can make.
//
// Revisions:
//   - 2026-11-11 06:44: initial creation (C122, B273)
func TestLines_StringCutsALongPath(t *testing.T) {
	var (
		root = t.TempDir()
		name = strings.Repeat(
			"a",
			250,
		)
	)

	err := os.WriteFile(
		filepath.Join(
			root,
			name,
		),
		nil,
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := _Eval(
		t,
		root,
		`str(file.lines(path.join(root, "`+name+`")))`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) > 250 || !strings.Contains(
		got,
		"... (",
	) {
		t.Fatalf(
			"got %d bytes, want the path cut: %.200q",
			len(got),
			got,
		)
	}
}
