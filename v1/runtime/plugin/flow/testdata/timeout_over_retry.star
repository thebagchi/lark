def slow():
    sleep(3000)
    return "done"

def waits():
    return retry(3, slow)

def main():
    return timeout(200, waits)
