def flaky():
    assert(n() >= 3, "not ready on attempt " + str(n()))
    return n()

def main():
    return retry(5, flaky)
