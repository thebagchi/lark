# A run wide enough that it reports from several goroutines at once, and with
# a retry in it so that one node reaches RUNNING more than once.

def flaky():
    assert(n() == 3, "not ready yet")
    return "ready"

def lane():
    return retry(3, flaky)

def main():
    return join(spawn(lane), spawn(lane), spawn(lane))
