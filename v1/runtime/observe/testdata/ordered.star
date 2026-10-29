# Prints from the spine, then from a thread the spine joins, then from the
# spine again, so the order of the three lines is fixed.

def alpha():
    print("two")

def main():
    print("one")
    join(spawn(alpha))
    print("three")
