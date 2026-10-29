def inner():
    state.update("y", lambda v: 1)

def outer(v):
    repeat(1, inner)
    return 1

def main():
    state.update("x", outer)
