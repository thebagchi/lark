load("cycle_a.star", "a")

def b():
    return a()
