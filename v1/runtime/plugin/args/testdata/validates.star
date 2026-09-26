# A script checking what its run supplied, at module level.
#
# The check belongs here rather than inside main: it is about the run's
# arguments, which are bound before anything else executes, so a run given a
# port nobody could listen on stops before it does any work.

port = arg("port", 8080)

assert(port > 0, "port must be positive")

def main():
    return port
