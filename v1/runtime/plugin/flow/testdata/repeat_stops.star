def step():
    state.update("calls", lambda c: 1 if c == None else c + 1)
    fail("stopping on attempt " + str(n()))

def main():
    return repeat(5, step)
