def writer():
    state.set("findings", ["one", "two"])

def reader():
    # A copy, so this is the reader's own list.
    found = state.get("findings")

    found.append("three")

    return found

def main():
    join(spawn(writer))

    return join(spawn(reader))[0]
