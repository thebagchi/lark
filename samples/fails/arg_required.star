# An arg() with no default needs a value. A run that supplies none stops at
# the declaration, and the error says how to supply it.
#
# Run it, from the repository root:
#
#   lark run samples/fails/arg_required.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: initialise samples/fails/arg_required.star: samples/fails/arg_required.star:12: token: argument not supplied and has no default (supply it as -args '{"token": ...}', or give arg() a default)

token = arg("token")

def main():
    print(token)
