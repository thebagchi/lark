package observe

import (
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Callee is one function the run called: its node in the graph, the call
// the node shows, and the arrival number of each thread the node lists.
//
// newest is the ordinal of the function's newest call, which is the call its
// node shows. tickets is parallel to node.Threads: the thread at a position
// arrived with the number at that position. A node lists its threads in the
// order they arrived, so the numbers are sorted, and a thread that leaves is
// found by a binary search of ints, with no lookup of each thread it passes.
type _Callee struct {
	node    *workflowpb.Node
	newest  int
	tickets []int
}
