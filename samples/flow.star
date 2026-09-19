# repeat, retry and timeout each take a function and give back a callable. The
# wrapping happens when that callable is invoked, not when it is built.
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
    three = repeat(counted, 3)
    last = three()

    eventually = retry(settles_on_the_third_try, 5)

    # A bounded call. Note the parentheses: the factory gives back a callable,
    # and nothing happens until it is called.
    bounded = timeout(pauses_briefly, 5)

    # Had pauses_briefly slept for thirty seconds instead, this would fail in
    # about 5 seconds rather than waiting it out: the sleep inside it watches
    # the same cancel the timeout fires.
    return {
        "repeat ran": state.get("calls"),
        "last attempt": last,
        "retry": eventually(),
        "timeout": bounded(),
    }
