def main():
    state.set("findings", ["one"])

    mine = state.get("findings")
    mine.append("two")

    return [state.get("findings"), mine]
