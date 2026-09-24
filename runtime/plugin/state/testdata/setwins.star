# A set that lands while an update's function is running must not be lost.
#
# The update reads the value, sleeps inside its function, then stores what it
# read. The set writes while that sleep is going. Without the name's lock the
# update's store lands second and the set is silently overwritten.

def slow(current):
    sleep(0.05)

    return "from-update"

def updater():
    state.update("k", slow)

def writer():
    sleep(0.01)
    state.set("k", "from-set")

def main():
    join(spawn(updater), spawn(writer))

    return state.get("k")
