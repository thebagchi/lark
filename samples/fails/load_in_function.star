# load is a top-level statement. Inside a function it is refused.
#
# Run it, from the repository root:
#
#   lark run samples/fails/load_in_function.star
#
# Expected: exit code 1. Stderr is one line:
# script failed: samples/fails/load_in_function.star:11:5: load statement within a function

def main():
    load("../strings.star", "shout")
    print(shout("x"))
