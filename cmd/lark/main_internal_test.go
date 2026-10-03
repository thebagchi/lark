package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// LONG is a directory name long enough that a socket beneath it is past
	// what a unix socket path may hold, and PRIVATE the mode it is made with.
	LONG    = 120
	PRIVATE = 0o700

	// UNPATTERNED is a plugin directory whose name the search cannot read as a
	// pattern, so loading from it fails.
	UNPATTERNED = "[x"
)

// TestHosting_LeavesNothingWhenItFails fails plugin hosting in the two ways
// that come after the socket's directory is made - the listener refusing its
// socket, and the plugins failing to load - and expects that directory gone
// either way.
//
// Internal, because _Hosting is main's: the package has no surface a test
// outside it could reach, and the directory left behind is the invariant.
//
// Revisions:
//   - 2026-10-03 20:28: initial creation
func TestHosting_LeavesNothingWhenItFails(t *testing.T) {
	short := t.TempDir()
	long := filepath.Join(t.TempDir(), strings.Repeat("d", LONG))

	err := os.Mkdir(long, PRIVATE)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, temp, plugins string }{
		{"a socket path too long to listen on", long, short},
		{
			"a plugin directory the search cannot read",
			short,
			filepath.Join(short, UNPATTERNED),
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv("TMPDIR", item.temp)

			_, _, err := _Hosting(item.plugins)
			if err == nil {
				t.Fatal("hosting started")
			}

			left, err := os.ReadDir(item.temp)
			if err != nil {
				t.Fatal(err)
			}

			for _, entry := range left {
				t.Errorf("left %s behind", filepath.Join(item.temp, entry.Name()))
			}
		})
	}
}
