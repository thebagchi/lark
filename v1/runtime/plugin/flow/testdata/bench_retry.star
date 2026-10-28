def empty():
    return "done"

def main():
    return retry(1, empty)
