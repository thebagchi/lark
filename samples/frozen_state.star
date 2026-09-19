# A store exists so threads can reach it at once, and handing a mutable value
# to two of them is the race it is meant to avoid.
#
# So values are frozen on the way in, and a thread that tries to change what it
# read fails loudly instead of corrupting what another thread is reading.
#
# This script fails on purpose.

def writer():
    state.set("findings", ["one", "two"])

def spoiler():
    found = state.get("findings")

    found.append("three")

    return found

def main():
    join(spawn(writer))

    return join(spawn(spoiler))[0]
