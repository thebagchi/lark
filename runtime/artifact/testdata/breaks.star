# A module-level statement that fails. Initialising happens once per run, so
# this is a run's failure and not a compile's.

broken = fail("module level")

def main():
    return broken
