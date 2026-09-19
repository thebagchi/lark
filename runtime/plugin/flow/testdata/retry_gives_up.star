def never():
    assert(False, "never ready")

def main():
    return retry(never, 3)()
