# load of a file that does not exist stops the run before main starts.
#
# Run it, from the repository root:
#
#   lark run samples/fails/load_missing.star
#
# Expected: exit code 1. Stderr is one line:
# script failed: cannot load "lib/nothing_here.star" from samples/fails/load_missing.star:
# open samples/fails/lib/nothing_here.star: no such file or directory

load("lib/nothing_here.star", "x")

def main():
    print(x)
