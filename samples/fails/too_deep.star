# A value nested deeper than 10,000 levels is refused by str, print and fail,
# not allowed to overflow the stack.
#
# Run it, from the repository root:
#
#   lark run samples/fails/too_deep.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/too_deep.star:16: str: value nested too deep

def main():
    v = []
    for i in range(20000):
        v = [v]
    print(str(v))
