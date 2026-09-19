def main():
    # Nothing is stored, so the update sees None and decides for itself.
    return state.update("fresh", lambda n: 0 if n == None else n + 1)
