# Reads what nothing has written, which is the ordinary case in a store threads
# share rather than an error.

def main():
    return state.get("never written")
