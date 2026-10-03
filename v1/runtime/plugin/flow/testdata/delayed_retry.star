def flaky():
    assert(n() >= 2, "not ready on attempt " + str(n()))
    return n()

def main():
    return retry(3, flaky, 300)
