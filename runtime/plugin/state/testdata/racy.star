# The same work through get and set, which is two operations with a gap.
def bump():
    for i in range(200):
        state.set("count", state.get("count") + 1)

def main():
    state.set("count", 0)

    join(spawn(bump), spawn(bump), spawn(bump), spawn(bump))

    return state.get("count")
