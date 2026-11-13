package tests

import (
	"os"
	"path/filepath"
	"testing"
)

// ORDER_NAMES is a set of names that differ in case, digits and unicode.
//
// Revisions:
//   - 2026-11-02 08:31: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	ORDER_NAMES = []string{
		"z.txt",
		"a.txt",
		"B.txt",
		"A.txt",
		"10.txt",
		"2.txt",
		"1.txt",
		"é.txt",
		"e.txt",
		"中.txt",
		"_x",
		".hidden",
		"a b",
	}
)

// TestList_OrdersByTheBytesOfTheName pins the order of file.list for names
// that differ in case, digits and unicode.
//
// The order is the bytes of the name, as os.ReadDir gives it. The expected
// value was captured from the code that sorted the names a second time.
//
// Revisions:
//   - 2026-11-02 08:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestList_OrdersByTheBytesOfTheName(t *testing.T) {
	var (
		root = t.TempDir()
		want = `[".hidden", "1.txt", "10.txt", "2.txt", "A.txt", "B.txt", ` +
			`"_x", "a b", "a.txt", "e.txt", "z.txt", "é.txt", "中.txt"]`
	)

	for _, name := range ORDER_NAMES {
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
	}

	got, err := _Eval(
		t,
		root,
		`file.list(root)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Fatalf(
			"got %s, want %s",
			got,
			want,
		)
	}
}
