# assert and fail differ in how far a failure reaches.
#
# The slow thread is joined FIRST, so join waits for it before it ever looks at
# the failing one. That ordering is what makes the difference visible:
#
#   fail()    ends only its own thread. Nothing cancels the slow one, so join
#             waits the whole count out before reporting the failure.
#   assert()  ends the run. The slow thread is cancelled where it stands and
#             the script finishes immediately.
#
# Swap the two lines in doomed() and time it both ways.

def counts_a_long_way():
    total = 0

    for i in range(15000000):
        total += i

    return total

def doomed():
    fail("this ends only this thread")
    # assert(False, "this would end the whole run")

def main():
    slow = spawn(counts_a_long_way)
    dies = spawn(doomed)

    return join(slow, dies)
