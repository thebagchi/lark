# An event is a latch: one thread says something happened, and any thread can
# wait until it has.
#
# event.post(name, value) says it, once. event.wait(name, milliseconds) waits
# for it and answers with the pair (value, err). err is None when the event was
# posted and a message when the wait ran out, so a timeout is an answer the
# script reads rather than a fault.
#
# It is a latch, not a wakeup: it stays posted. A waiter that arrives after the
# post does not wait at all, and every waiter sees the same value. So the order
# of the threads cannot lose the signal.
#
# Run it:
#
#   lark run samples/event.star
#
# Expected output: the waiter, the late waiter and the timeout, each on a line.
# A second event.post of one name stops the whole run; uncomment the last line
# of main to see it.

def producer():
    sleep(10)
    event.post("loaded", {"rows": 12})

def consumer():
    value, err = event.wait("loaded", 5000)

    return value["rows"] if err == None else err

def main():
    producing = spawn(producer)
    waiting = spawn(consumer)

    print("consumer saw rows", join(waiting)[0])

    join(producing)

    # The event is still posted, so this wait returns at once.
    value, err = event.wait("loaded", 5000)
    print("late waiter", value, err)

    # Nothing posts "never", so this one runs out after 20 milliseconds.
    value, err = event.wait("never", 20)
    print("nothing posted", value, err)

    # event.post("loaded", {"rows": 13})
