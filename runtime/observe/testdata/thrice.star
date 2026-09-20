def first():
    return 1

def second():
    return 2

def main():
    join(spawn(first), spawn(second), spawn(first))
