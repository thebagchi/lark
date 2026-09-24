# A thread spawned inside an update, locking after the update has returned.
#
# The child copied the mark at birth and nothing clears a copy, so it is
# refused for the rest of its life - deliberately, because clearing the mark
# on release would make this script succeed or fail on where the set happened
# to land relative to a release it cannot see.

def child():
    sleep(0.05)

    state.set("b", 1)

def spawner(v):
    return spawn(child)

def main():
    held = state.update("a", spawner)

    join(held)
