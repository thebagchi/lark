# A failed assertion ends the thread it ran on and no other. The sibling
# finishes untouched, and the failure surfaces where something joins it.

def breaks():
    assert(False, "this thread was always going to fail")

    return "unreachable"

def survives():
    return "this one finished"

def main():
    doomed = spawn(breaks)
    fine = spawn(survives)

    print("the survivor returned:", join(fine)[0])

    return join(doomed)
