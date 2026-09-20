def step():
    state.update("calls", lambda c: 1 if c == None else c + 1)
    return n()

def main():
    last = repeat(3, step)
    return [last, state.get("calls")]
