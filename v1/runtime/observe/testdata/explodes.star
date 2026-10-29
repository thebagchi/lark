# Calls a builtin that panics, on a thread the spine joins.

def boom():
    explode()
    return 1

def main():
    return join(spawn(boom))
