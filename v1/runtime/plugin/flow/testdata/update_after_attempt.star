def step():
    state.update("calls", lambda c: 1 if c == None else c + 1)
    return n()

def main():
    repeat(2, step)
    state.update("calls", lambda c: c + 10)
    return state.get("calls")
