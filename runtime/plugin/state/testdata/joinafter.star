# Joining once the update has returned. The refusal is about a name being
# held, not about the handle, so this is legal and must stay so.

def worker():
    return "joined"

def main():
    held = spawn(worker)

    state.update("k", lambda v: 1)

    return join(held)[0]
