# spawn starts a named function on a thread of its own and hands back a handle.
# join waits for handles and returns what each produced, in the order given.

load("strings.star", "shout")

def first():
    return shout("hello")

def second():
    return shout("world")

def main():
    parts = join(spawn(first), spawn(second))

    return " ".join(parts)
