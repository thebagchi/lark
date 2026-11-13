# A script with no def main() is a module, to load. Run on its own, it has no
# entry point.
#
# Run it, from the repository root:
#
#   lark run samples/fails/no_main.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/no_main.star: no entry point (define def main() with no required parameters; a file without it is a module, for load)

def helper():
    return 1
