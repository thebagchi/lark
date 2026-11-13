# A name that is not defined is refused before the script runs. When a defined
# name is close, the error ends with a did-you-mean hint.
#
# Run it, from the repository root:
#
#   lark run samples/fails/undefined_name.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/undefined_name.star:14:11: undefined: totl (did you mean total?)

def main():
    total = 10
    print(totl)
