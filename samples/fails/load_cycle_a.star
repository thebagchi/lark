# Two files that load each other are a cycle in the load graph, and the run
# refuses them.
#
# Run it, from the repository root:
#
#   lark run samples/fails/load_cycle_a.star
#
# Expected: exit code 1. Stderr is one line:
# script failed: cycle in the load graph: samples/fails/load_cycle_a.star ->
# samples/fails/lib/load_cycle_b.star -> samples/fails/load_cycle_a.star

load("lib/load_cycle_b.star", "b")

a = 1

def main():
    print(a, b)
