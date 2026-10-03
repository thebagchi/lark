def slow():
    sleep(30000)
    return "never"

def main():
    return timeout(50, slow)
