# Loads the same module the entry script does, so one compile reaches lib.star
# by two routes.
load("lib.star", "double")

def triple(x):
    return double(x) + x
