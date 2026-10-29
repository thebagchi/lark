def broken():
    sleep(100)
    assert(False, "a sibling broke")

def slow():
    sleep(3000)
    return "done"

def main():
    spawn(broken)
    return retry(3, slow)
