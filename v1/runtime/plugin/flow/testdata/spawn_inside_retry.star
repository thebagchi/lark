def checks():
    assert(n() >= 2, "inner not ready")
    return n()

def attempt():
    return join(spawn(checks))[0]

def main():
    return retry(3, attempt)
