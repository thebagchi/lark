def a():
    sleep(200)
    fail("from a")

def b():
    fail("from b")

def main():
    join(spawn(a), spawn(b))
