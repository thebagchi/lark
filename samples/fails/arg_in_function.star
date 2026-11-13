# arg() declares a module-level name. Inside a function body it is refused,
# because it could only ever take its default.
#
# Run it, from the repository root:
#
#   lark run samples/fails/arg_in_function.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/arg_in_function.star:13: host: arg declares a module-level name

def main():
    host = arg("host", "localhost")
    print(host)
