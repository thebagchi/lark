def again():
    return state.update("k", lambda v: 2)

def main():
    return state.update("k", lambda v: timeout(1, again))
