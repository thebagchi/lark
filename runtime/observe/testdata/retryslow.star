def flaky():
    sleep(0.05)
    assert(n() == 3, "not ready yet")
    return "ready"

def main():
    return retry(3, flaky)
