def slow():
    sleep(30)
    return "never"

def main():
    return timeout(slow, 0.05)()
