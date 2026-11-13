# lark takes exactly one input. Two scripts on one command line are a
# command-line error: nothing runs, and the exit code is 2, not 1.
#
# Run it, from the repository root, with a second script after this one:
#
#   lark run samples/fails/two_scripts.star samples/hello.star
#
# Expected: exit code 2. Stderr is the two lines below.
#
#   lark: wrong number of arguments (give exactly one FILE)
#   try lark help

def main():
    print("this never runs")
