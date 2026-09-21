def slow():
    sleep(3)
    return "done"

def waits():
    return join(spawn(slow))

def main():
    return timeout(0.2, waits)
