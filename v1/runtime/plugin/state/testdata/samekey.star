# An update inside an update, on the same name. Without the refusal this waits
# on a lock its own caller holds, and the run never ends.

def inner(current):
    return state.update("count", lambda n: 1)

def main():
    return state.update("count", inner)
