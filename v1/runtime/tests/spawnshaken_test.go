package tests

import (
	"errors"
	"path"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// ASSIGNED_MODULE is a module whose top level assigns the handle of a
	// spawn to r.
	//
	// Revisions:
	//   - 2026-11-12 19:00: initial creation (C130, B287)
	ASSIGNED_MODULE = `
def f():
    return 1

r = spawn(f)
`

	// ASSIGNED_UNREAD loads the module and never reads r.
	//
	// Revisions:
	//   - 2026-11-12 19:00: initial creation (C130, B287)
	ASSIGNED_UNREAD = `
load("assigned.star", "f")

def main():
    return f()
`

	// ASSIGNED_READ loads the module and joins r in main.
	//
	// Revisions:
	//   - 2026-11-12 19:00: initial creation (C130, B287)
	ASSIGNED_READ = `
load("assigned.star", "r")

def main():
    return join(r)
`

	// ASSIGNED_LINE is the module with a second statement on the line of the
	// assignment, a call that reads r, so the assignment stays.
	//
	// Revisions:
	//   - 2026-11-12 19:00: initial creation (C130, B287)
	ASSIGNED_LINE = `
def f():
    return 1

r = spawn(f); print(r)
`

	// ASSIGNED_ELSEWHERE is the module with a second statement on the line of the
	// assignment, a call that does not read r, so the assignment is shaken.
	//
	// Revisions:
	//   - 2026-11-12 19:00: initial creation (C130, B287)
	ASSIGNED_ELSEWHERE = `
def f():
    return 1

r = spawn(f); print(1)
`
)

// _Assigned is a Loader that serves one module, assigned.star, with the text
// it was made with.
//
// Revisions:
//   - 2026-11-12 19:00: initial creation (C130, B287)
type _Assigned struct {
	text string
}

// Resolve reads target as a name beside from.
//
// Revisions:
//   - 2026-11-12 19:00: initial creation (C130, B287)
func (m *_Assigned) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text of the module.
//
// Revisions:
//   - 2026-11-12 19:00: initial creation (C130, B287)
func (m *_Assigned) Load(string) ([]byte, error) {
	return []byte(strings.TrimPrefix(
		m.text,
		"\n",
	)), nil
}

// TestRun_AnAssignedSpawnOfALoadedModuleIsShakenOrRefused proves
// `r = spawn(f)` at the top level of a loaded module never races (B287, Q383,
// Q389). When nothing reads r, the shake takes the assignment out, so the
// merged script holds no spawn and no thread can start. When main reads r, or
// a statement that reads r shares the line so the assignment stays, the spawn runs
// while the module loads and is refused with runtime.ErrInit.
//
// The check: go test -race -count=1 -run TestRun_AnAssignedSpawnOfALoadedModuleIsShakenOrRefused
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 19:00: initial creation (C130, B287)
func TestRun_AnAssignedSpawnOfALoadedModuleIsShakenOrRefused(t *testing.T) {
	var tests = []struct {
		name   string
		module string
		entry  string
		want   error
		spawns bool
	}{
		{
			"r is never read",
			ASSIGNED_MODULE,
			ASSIGNED_UNREAD,
			nil,
			false,
		},
		{
			"main joins r",
			ASSIGNED_MODULE,
			ASSIGNED_READ,
			runtime.ErrInit,
			true,
		},
		{
			"a statement that reads r shares its line",
			ASSIGNED_LINE,
			ASSIGNED_UNREAD,
			runtime.ErrInit,
			true,
		},
		{
			"r is never read, a statement that does not read it shares its line",
			ASSIGNED_ELSEWHERE,
			ASSIGNED_UNREAD,
			nil,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var source = &common.Source{
					Entry: "assigned_main.star",
					Text: []byte(strings.TrimPrefix(
						tc.entry,
						"\n",
					)),
					Loader: &_Assigned{
						text: tc.module,
					},
				}

				merged, err := runtime.Merge(source)
				if err != nil {
					t.Fatal(err)
				}

				if got := strings.Contains(
					string(merged.Text),
					"spawn(",
				); got != tc.spawns {
					t.Fatalf(
						"merged script holds a spawn: got %v, want %v\n%s",
						got,
						tc.spawns,
						merged.Text,
					)
				}

				built, err := runtime.Compile(source)
				if err != nil {
					t.Fatal(err)
				}

				for range 20 {
					_, err = _Waited(
						t,
						built,
					)
					if !errors.Is(
						err,
						tc.want,
					) {
						t.Fatalf(
							"err = %v, want %v",
							err,
							tc.want,
						)
					}
				}
			},
		)
	}
}
