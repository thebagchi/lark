# A set from inside an update is a nested update, and must be refused rather
# than wait for a lock this evaluation already holds.

def inner(current):
    state.set("k", "never stored")

    return current

def main():
    return state.update("k", inner)
