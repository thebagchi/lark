# state is how threads pass data to each other.
#
# Module scope is frozen before anything concurrent runs, so a script cannot
# share anything by assigning to a global. A store is the only way.
#
# The store belongs to one execution: run this twice and both runs start empty.

def gather_alpha():
    state.set("alpha", "first thread was here")

def gather_beta():
    state.set("beta", "second thread was here")

def main():
    # Nothing is written until these have finished, so join before reading.
    join(spawn(gather_alpha), spawn(gather_beta))

    # A name nothing has written reads as None rather than failing - the
    # ordinary case in a store several threads write to.
    missing = state.get("gamma")

    return [state.get("alpha"), state.get("beta"), missing]
