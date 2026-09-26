def main():
    looping = [1]
    looping.append(looping)

    state.set("looping", looping)

    got = state.get("looping")
    got.append(2)

    shared = [9]
    state.set("twice", [shared, shared])

    pair = state.get("twice")
    pair[0].append(8)

    return [len(got), pair[1]]
