def holds():
    state.update("k", lambda v: sleep(30))

def waits():
    state.update("k", lambda v: 1)

def main():
    spawn(holds)
    sleep(0.05)
    spawn(waits)
    assert(False, "end the run while waits is blocked on the lock")
