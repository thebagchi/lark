# timeout(ms, fn) fails the run when fn does not end in ms milliseconds. The
# call sleeps for ten seconds but the run ends in about 50 milliseconds,
# because the time limit cancels it.
#
# Run it, from the repository root:
#
#   lark run samples/fails/timeout.star
#
# Expected: exit code 1. Stderr starts with this line (more lines may follow):
#
#   script failed: samples/fails/timeout.star:17: timeout(50, slow): timed out

def slow():
    sleep(10000)

def main():
    timeout(50, slow)
