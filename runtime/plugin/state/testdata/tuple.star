def main():
    state.set("pair", (1, 2))
    state.set("nested", [("a", 1), {"k": ("b", 2)}])

    return [state.get("pair"), state.get("nested")]
