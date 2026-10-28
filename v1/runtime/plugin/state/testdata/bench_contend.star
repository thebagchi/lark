def holder():
    state.update("k", lambda v: sleep(50))

def setter():
    sleep(10)
    state.set("k", 1)

def main():
    spawn(holder)
    spawn(setter)
    sleep(100)
    return state.get("k")
