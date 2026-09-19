def inner(current):
    return state.update("other", lambda n: 1)

def main():
    return state.update("count", inner)
