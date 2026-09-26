# A thread spawned inside an update, reaching the store after the update has
# returned.
#
# The child copied the mark at birth and nothing clears a copy, so its set is
# refused for the rest of its life. It cannot be joined - an update's function
# returns data now, so the handle cannot leave - and an unjoined failure does
# not reach the run, so what it did is read from what it printed.

def child():
    sleep(0.05)

    print("child reached the store")

    state.set("b", 1)

    print("child stored")

def spawner(v):
    spawn(child)

    return "spawned"

def main():
    state.update("a", spawner)

    sleep(0.3)
