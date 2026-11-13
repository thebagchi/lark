# spawn refuses a nested function that reads a local variable of its caller.
# The error names the variable and the fix.
#
# Run it, from the repository root:
#
#   lark run samples/fails/captured_local.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/captured_local.star:18: spawn: work captures n, bound at samples/fails/captured_local.star:13:5: captures a local variable (write spawn(lambda: f(x)) to give the thread x as it is at the spawn)

def main():
    n = 5

    def work():
        print(n)

    h = spawn(work)
    join(h)
