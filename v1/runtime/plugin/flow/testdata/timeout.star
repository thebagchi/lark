def slow():
    sleep(30)
    return "never"

def main():
    return timeout(0.05, slow)
