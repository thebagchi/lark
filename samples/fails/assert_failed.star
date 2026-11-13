# A failed assert() stops the run. The text is assertion failed: then the
# message, quoted.
#
# Run it, from the repository root:
#
#   lark run samples/fails/assert_failed.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/assert_failed.star:13: assertion failed: "arithmetic broke"

def main():
    assert(1 + 1 == 3, "arithmetic broke")
