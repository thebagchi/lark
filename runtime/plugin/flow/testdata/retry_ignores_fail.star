def broken():
    state.update("calls", lambda c: 1 if c == None else c + 1)
    fail("not an assertion")

def main():
    return retry(broken, 5)()
