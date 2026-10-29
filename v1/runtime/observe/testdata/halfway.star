# Prints one line and then raises, so a failed run still has a line to show.

def main():
    print("before")
    fail("after the print")
