# A failure stops the whole run. Threads that never ran stay in the graph as
# NOT_STARTED: later() is spawned after the join that fails, so it never
# starts. See it in the graph.
#
# Run it, from the repository root:
#
#   lark run samples/fails/failfast_graph.star
#
# To see the unstarted thread, write the changes of the graph to a file:
#
#   lark run -graph /tmp/graph.jsonl samples/fails/failfast_graph.star
#
# Check: grep -o '"later"[^}]*' /tmp/graph.jsonl prints STATUS_NOT_STARTED.
# slow is STATUS_CANCELLED, and the cause names boom.
#
# Expected: exit code 1. Stderr starts with this line (a cause line may follow):
#
#   script failed: boom: samples/fails/failfast_graph.star:27: fail: stop here

def slow():
    sleep(10000)

def later():
    print("never started")

def boom():
    fail("stop here")

def main():
    a = spawn(slow)
    b = spawn(boom)
    join(b, a)
    spawn(later)
