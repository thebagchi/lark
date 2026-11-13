# A queue passes many values from one thread to another, in order.
#
# q = queue.new() makes a queue that holds 64 values, and queue.new(n) one that
# holds n. The queue is an object: pass it to the threads you spawn, as below.
#
# q.push(value) adds a value at the back and never waits. It answers None, or
# queue.FULL when the queue already holds as many values as it can. The value
# is frozen and the queue keeps that same value, so nothing is copied.
#
# q.pop() takes the value at the front and waits until there is one. Like
# event.wait it answers the pair (value, err): err is None when a value came,
# and queue.CLOSED when q.close() was called and every value pushed before it
# was already taken. pop takes no time limit. A timeout() around it, or a
# cancel of the run, ends the wait.
#
# Each value goes to exactly one popper. Several workers on one queue share the
# work; none sees the values of another. This is not a broadcast.
#
# Run it:
#
#   lark run samples/queue.star
#
# Expected output: the ids in order, the shares of the two workers adding up to
# 20, the number of values a queue of 3 took before it answered FULL, and the
# record of a pop that a timeout cut. A push after q.close() stops the whole
# run; uncomment the last line of main to see it.

def produce(q, count):
    for i in range(1, count + 1):
        # A full queue is an answer, not a fault. This producer waits a
        # moment and tries again; another might drop the value.
        while q.push({"id": i, "square": i * i}) == queue.FULL:
            sleep(1)

    # Close once, after the last push. A popper then drains what is left and
    # sees CLOSED, so it knows when to stop without a marker value.
    q.close()

def ids(q):
    seen = []

    while True:
        job, err = q.pop()

        if err == queue.CLOSED:
            return seen

        seen.append(job["id"])

def worker(q):
    total = 0

    while True:
        job, err = q.pop()

        if err == queue.CLOSED:
            return total

        total += 1

def main():
    # One producer and one popper: the order of the pushes is the order of the pops.
    ordered = queue.new(2)
    sending = spawn(lambda: produce(ordered, 5))
    reading = spawn(lambda: ids(ordered))

    join(sending)
    print("ids in order", join(reading)[0])

    # One producer and two poppers: every value goes to one of them.
    shared = queue.new()
    left = spawn(lambda: worker(shared))
    right = spawn(lambda: worker(shared))
    feeding = spawn(lambda: produce(shared, 20))

    join(feeding)
    shares = join(left, right)
    print("shares add up to", shares[0] + shares[1])

    # A queue holds a fixed number of values. push does not wait, so a full
    # queue answers at once.
    small = queue.new(3)
    taken = 0

    for i in range(10):
        if small.push(i) == queue.FULL:
            break

        taken += 1

    print("values taken before full", taken)

    # Nothing pushes to idle, so its pop would wait for ever. A timeout() around
    # it ends the wait after 20 milliseconds, and outcomes reads what became of it.
    idle = queue.new()
    record = outcomes(spawn(lambda: timeout(20, lambda: idle.pop())))[0]
    print("pop cut by a timeout", record["ok"], record["kind"])

    # q.push(1) after q.close() stops the run:
    # ordered.push(1)
