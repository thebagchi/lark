def flaky():
    assert(n() == 3, "not ready yet")
    return "ready"

def main():
    return retry(flaky, 3)()
