load("cycle_b.star", "b")

def a():
    return b()

def main():
    return a()
