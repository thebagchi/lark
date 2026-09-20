def good():
    return 1

def bad():
    assert(False, "this one breaks")

def main():
    join(spawn(good), spawn(bad))
