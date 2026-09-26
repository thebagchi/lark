# A spawned function that spawns, which is what makes a thread id hierarchical:
# deep is started by alpha, not by the entry point.

def deep():
    return 3

def alpha():
    return join(spawn(deep))[0]

def beta():
    return 2

def main():
    join(spawn(alpha), spawn(beta))
