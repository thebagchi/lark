def writer():
    state.set("message", "written by one thread, read by another")

def reader():
    return state.get("message")

def main():
    join(spawn(writer))

    return join(spawn(reader))[0]
