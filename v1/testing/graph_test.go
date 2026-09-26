// Regression probes for the graph: what Check accepts as a thread id, and
// that a script and the graph it describes still print the same thing.
package testing_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

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
	if !bytes.Equal(fromScript, fromGraph) {
		t.Fatalf("script:\n%s\ngraph:\n%s", fromScript, fromGraph)
	}
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

func TestCheck_ASpineForkingItself(t *testing.T) {
	call := &workflowpb.Step{
		Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: graph.ENTRY}},
	}
	fork := &workflowpb.Step{
		Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: graph.SPINE}},
	}
	built := &workflowpb.Graph{
		Threads: []*workflowpb.Thread{{
			Id: graph.SPINE,
			State: &workflowpb.Thread_Static{Static: &workflowpb.Static{
				Steps: []*workflowpb.Step{call, fork},
			}},
		}},
	}
	fork.GetFork().Thread = "thread_1"
	if err := graph.Check(built); err != nil {
		t.Fatalf("spine forking thread_1: %v", err)
	}

	fork.GetFork().Thread = "thread_1_1"
	if !errors.Is(graph.Check(built), graph.ErrParentage) {
		t.Fatal("a grandchild id under the spine was not refused")
	}

	fork.GetFork().Thread = graph.SPINE
	if !errors.Is(graph.Check(built), graph.ErrParentage) {
		t.Fatalf("spine forking itself: %v", graph.Check(built))
	}
}

func TestCheck_ANestedOrdinalIsADecimal(t *testing.T) {
	if err := graph.Check(_Forking("thread_1", "thread_1_2")); err != nil {
		t.Fatalf("thread_1 forking thread_1_2: %v", err)
	}
	if !errors.Is(graph.Check(_Forking("thread_1", "thread_1_02")), graph.ErrParentage) {
		t.Fatalf(
			"thread_1 forking thread_1_02: %v",
			graph.Check(_Forking("thread_1", "thread_1_02")),
		)
	}
}

func TestCheck_AnOrdinalIsADecimal(t *testing.T) {
	for _, id := range []string{"thread_1", "thread_10"} {
		if err := graph.Check(_SpineForking(id)); err != nil {
			t.Fatalf("spine forking %s: %v", id, err)
		}
	}

	for _, id := range []string{"thread_01", "thread_00", "thread_1a"} {
		if !errors.Is(graph.Check(_SpineForking(id)), graph.ErrParentage) {
			t.Fatalf("spine forking %s: %v", id, graph.Check(_SpineForking(id)))
		}
	}
}

func TestCheck_AnEmptyOrdinalIsNotAChild(t *testing.T) {
	for _, id := range []string{"thread_", "thread_01", "thread_00", "thread_1a"} {
		t.Logf("spine forking %q -> %v", id, graph.Check(_SpineForking(id)))
	}

	if !errors.Is(graph.Check(_SpineForking("thread_")), graph.ErrParentage) {
		t.Fatalf("spine forking thread_: %v", graph.Check(_SpineForking("thread_")))
	}
}

func _SpineForking(id string) *workflowpb.Graph {
	return _Forking(graph.SPINE, id)
}

func _Forking(parent string, id string) *workflowpb.Graph {
	return &workflowpb.Graph{
		Threads: []*workflowpb.Thread{{
			Id: parent,
			State: &workflowpb.Thread_Static{Static: &workflowpb.Static{
				Steps: []*workflowpb.Step{
					{Action: &workflowpb.Step_Call{
						Call: &workflowpb.Call{Function: graph.ENTRY},
					}},
					{Action: &workflowpb.Step_Fork{
						Fork: &workflowpb.Fork{Thread: id},
					}},
				},
			}},
		}},
	}
}
