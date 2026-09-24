# Nesting reached through a spawn, with the join moved outside the update so
# that the refusal under test is the child's own.
#
# The child is spawned inside the update and carries the mark for life, so its
# state.update is refused whenever it runs. The join happens after the parent
# has returned, which is legal.

def bump():
    state.update("k", lambda v: 1)

def spawner(v):
    return spawn(bump)

def main():
    held = state.update("k", spawner)

    join(held)
