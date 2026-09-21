# Prints from the spine and from two spawned threads, so a transcript has
# three lanes in it.

def alpha():
    print("from alpha")

def beta():
    print("from beta")

def main():
    print("from the spine")

    join(spawn(alpha), spawn(beta))
