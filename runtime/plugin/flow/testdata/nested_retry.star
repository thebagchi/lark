def always():
    assert(False, "never")

def inner():
    state.update("outer", lambda v: 1 if v == None else v + 1)
    return retry(2, always)

def main():
    retry(3, inner)
