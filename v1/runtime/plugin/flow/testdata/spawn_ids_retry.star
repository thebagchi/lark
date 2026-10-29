def checks():
    assert(n() >= 2, "inner not ready")
    return n()

def attempt():
    return join(spawn(checks))[0]

def worker():
    return n()

def twice():
    return join(spawn(worker))[0]

def main():
    first = retry(3, attempt)
    second = repeat(2, twice)
    return [first, second]
