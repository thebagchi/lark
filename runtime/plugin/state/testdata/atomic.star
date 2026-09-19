def bump():
    for i in range(200):
        state.update("count", lambda n: n + 1)

def main():
    state.set("count", 0)

    join(spawn(bump), spawn(bump), spawn(bump), spawn(bump))

    return state.get("count")
