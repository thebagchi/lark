# One thread writes a name and reads it back, as pairs in a loop.
def main():
    for i in range(1000):
        state.set("k", i)
        seen = state.get("k")

    return seen
