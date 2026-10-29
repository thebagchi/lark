# A declaration inside an attempt. The attempt runs on the module's own
# thread, but it is a body all the same, as it was when it had a thread of its
# own.

def declare():
    return arg("host", "localhost")

host = repeat(1, declare)

def main():
    return host
