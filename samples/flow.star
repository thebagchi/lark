# repeat, retry and timeout each take a count or a budget first and the function
# last, and call it straight away. They are not factories: there is no second
# call, and what they give back is what the wrapped call produced.
#
# Every attempt runs on a goroutine and an interpreter thread of its own, so an
# attempt is one evaluation whether or not anything is bounding it. n() is the
# 1-based attempt number, and it is per attempt rather than shared, because
# attempts run on different threads.

def counted():
    state.update("calls", lambda c: 1 if c == None else c + 1)

    return n()

def settles_on_the_third_try():
    # assert inside a retry ends the attempt. Outside one it would end the run.
    assert(n() >= 3, "not ready on attempt " + str(n()))

    return "ready on attempt " + str(n())

def pauses_briefly():
    sleep(0.01)

    return "finished inside its budget"

def main():
    last = repeat(3, counted)

    # Each wrapper calls straight away and gives back what the call produced.
    # Had pauses_briefly slept for thirty seconds, the timeout would fail in
    # about 5 seconds rather than waiting it out: the sleep inside it watches
    # the same cancel the timeout fires.
    print("repeat ran", state.get("calls"))
    print("last attempt", last)
    print("retry", retry(5, settles_on_the_third_try))
    print("timeout", timeout(5, pauses_briefly))
