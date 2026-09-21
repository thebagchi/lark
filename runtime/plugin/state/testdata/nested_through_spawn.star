def bump():
    state.update("k", lambda v: 1)

def main():
    state.update("k", lambda v: join(spawn(bump)))
