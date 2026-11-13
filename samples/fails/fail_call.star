# fail() stops the run with the message you give it.
#
# Run it, from the repository root:
#
#   lark run samples/fails/fail_call.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/fail_call.star:12: fail: the disk is full

def main():
    fail("the disk is full")
