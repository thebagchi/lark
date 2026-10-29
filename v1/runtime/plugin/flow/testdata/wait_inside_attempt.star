def waits():
    event.wait("never", 100)

def outer(v):
    retry(1, waits)
    return 1

def main():
    state.update("x", outer)
