def never():
    assert(False, "never ready")

def main():
    return retry(3, never)
