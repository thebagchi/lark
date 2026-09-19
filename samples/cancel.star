# cancel stops a thread without waiting for it. Joining a cancelled handle
# raises, because asking for its result is asking for something that will never
# exist.

def counts_forever():
    total = 0

    for i in range(100000000):
        total += i

    return total

def main():
    h = spawn(counts_forever)

    cancel(h)

    return join(h)
