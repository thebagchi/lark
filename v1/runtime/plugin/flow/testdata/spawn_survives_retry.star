def worker():
    sleep(200)
    state.set("done", "yes")
    return "yes"

def attempt():
    spawn(worker)
    return "started"

def main():
    retry(1, attempt)
    sleep(500)
    return state.get("done")
