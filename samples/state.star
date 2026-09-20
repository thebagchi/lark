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

def counted():
    # Several threads change one name, so this must be an update. A get
    # followed by a set is two operations, and another thread writing between
    # them loses one of the increments.
    for i in range(50):
        state.update("writes", lambda n: 1 if n == None else n + 1)

def report():
    print("alpha", state.get("alpha"))
    print("beta", state.get("beta"))
    print("writes", state.get("writes"))

    # A name nothing has written reads as None rather than failing - the
    # ordinary case in a store several threads write to.
    print("gamma, which nothing wrote", state.get("gamma"))

def main():
    # These two write their own names, so a plain set is enough.
    join(spawn(gather_alpha), spawn(gather_beta))

    # These four all write "writes", so they update instead.
    join(spawn(counted), spawn(counted), spawn(counted), spawn(counted))

    report()
