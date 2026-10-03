def main():
    seen = state.get("runs")

    if seen == None:
        seen = 0

    state.set("runs", seen + 1)

    return state.get("runs")
