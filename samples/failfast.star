# A failed assertion stops the whole run, not just the thread it ran on.
#
# The sibling counts to a hundred million. It never finishes: the assertion
# cancels it, and this script ends in milliseconds rather than seconds.
#
# join is fail-fast too - when one handle fails it cancels the ones it has not
# reached, waits for them so nothing is abandoned, and raises the first failure
# in argument order.

def counts_a_long_way():
    total = 0

    for i in range(100000000):
        total += i

    return total

def gives_up():
    assert(False, "this ends the whole run")

def main():
    slow = spawn(counts_a_long_way)
    doomed = spawn(gives_up)

    return join(doomed, slow)
