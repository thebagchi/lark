def writer():
    state.set("list", [1, 2])

def mutator():
    got = state.get("list")

    # The store froze this on the way in, so appending fails loudly rather
    # than racing the thread that wrote it.
    got.append(3)

    return got

def main():
    join(spawn(writer))

    return join(spawn(mutator))[0]
