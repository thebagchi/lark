def swift():
    return "done"

def main():
    return timeout(swift, 5)()
