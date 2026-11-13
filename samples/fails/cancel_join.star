# join of a cancelled handle fails: the result of a cancelled thread never
# exists. Read it with outcomes() instead.
#
# Run it, from the repository root:
#
#   lark run samples/fails/cancel_join.star
#
# Expected: exit code 1. Stderr starts with this line (more lines may follow):
#
#   script failed: samples/fails/cancel_join.star:18: waits: cancelled: context canceled (read a cancelled thread with outcomes())

def waits():
    sleep(10000)

def main():
    h = spawn(waits)
    cancel(h)
    join(h)
