def slow():
    sleep(3000)
    return "done"

def waits():
    return retry(3, slow)

def bounded():
    return timeout(200, waits)

def main():
    spawn(bounded)
    sleep(400)
    return "main lives"
