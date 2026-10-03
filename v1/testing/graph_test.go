// Regression probe for the flow: a script and the flow it describes still
// print the same thing.
package testing_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// STAMPED ends the time a log line was written, its first field.
const STAMPED = " "

// TestRoundTrip_NewModulesPrintTheSame checks a script and the flow it
// describes print the same lines, apart from when each was printed.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-10-02 13:12: compares the transcripts without when each line was
//     printed, which no two runs share
func TestRoundTrip_NewModulesPrintTheSame(t *testing.T) {
	root := _ModuleRoot(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "round.star")
	body := []byte(`
def main():
    random.seed(7)
    nums = []
    for _ in range(20):
        nums.append(random.int(0, 9))
    print(math.log2(1024))
    print(math.gcd(12, 18))
    print(regexp.sub(r"(\w+)@(\w+)", "$2/$1", "bob@corp"))
    print(hash.sha256("abc"))
    print(base64.encode("foobar"))
    print(base32.encode("foobar"))
    print(path.base(path.join("a", "b.txt")))
    print(nums)
`)
	if err := os.WriteFile(script, body, 0o644); err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(dir, "round.json")

	fromScript := _Lark(t, root, "-s", script)
	written := _Lark(t, root, "-s", script, "-t")
	if err := os.WriteFile(graph, written, 0o644); err != nil {
		t.Fatal(err)
	}
	fromGraph := _Lark(t, root, "-g", graph)
	if _Unstamped(fromScript) != _Unstamped(fromGraph) {
		t.Fatalf("script:\n%s\ngraph:\n%s", fromScript, fromGraph)
	}
}

// _Unstamped is a log with the time each line was written taken off.
//
// Revisions:
//   - 2026-10-02 13:12: initial creation
//   - 2026-10-02 16:21: takes off a text handler's time field
func _Unstamped(transcript []byte) string {
	var kept []string

	for line := range strings.Lines(string(transcript)) {
		_, rest, found := strings.Cut(line, STAMPED)
		if !found {
			rest = line
		}

		kept = append(kept, rest)
	}

	return strings.Join(kept, "")
}

func _ModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func _Lark(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/lark"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lark %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}
