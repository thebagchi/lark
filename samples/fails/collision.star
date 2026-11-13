# Two loads that bind the same name in one file are refused: a name is bound
# once.
#
# Run it, from the repository root:
#
#   lark run samples/fails/collision.star
#
# Expected: exit code 1. Stderr is one line:
# script failed: samples/fails/collision.star:12:29: cannot reassign top-level helper

load("lib/collide_a.star", "helper")
load("lib/collide_b.star", "helper")

def main():
    print(helper())
