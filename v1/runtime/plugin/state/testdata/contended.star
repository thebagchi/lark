def holds():
    state.update("k", lambda v: sleep(30000))

def waits():
    state.update("k", lambda v: 1)

def main():
    spawn(holds)
    sleep(50)
    spawn(waits)
    assert(False, "end the run while waits is blocked on the lock")
