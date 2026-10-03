def slow():
    sleep(3000)
    return "done"

def waits():
    return join(spawn(slow))

def main():
    return timeout(200, waits)
