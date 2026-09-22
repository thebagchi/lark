# arg declares what a run supplies, and what the declaration takes when
# nothing does.
#
# Run it as it is to see the defaults, then supply some:
#
#   lark -s samples/args.star
#   lark -s samples/args.star -a '{"host": "db.internal", "port": 5432}'
#
# A declaration is a module-level statement, and the name it binds need not be
# the name a caller supplies it under - "host" arrives, "server" is what this
# script calls it. An arg() call inside a function body is refused, because the
# thread running a body is not the thread a run binds its arguments on, so such
# a call could only ever take its default in silence.
#
# Declare one with no default and the run stops at the declaration unless
# something supplies it. Uncomment the last one to see that.

server = arg("host", "localhost")
port = arg("port", 8080)
retries = arg("retries", 3)
verbose = arg("verbose", False)
tags = arg("tags", ["staging"])

# token = arg("token")

# A run initialises the script itself, so a check about the run's arguments
# belongs out here, where it stops a bad run before it does any work:
#
#   lark -s samples/args.star -a '{"retries": 0}'

assert(retries > 0, "retries must be at least one")

def main():
    print("%s:%d" % (server, port))
    print("retries", retries)
    print("verbose", verbose)
    print("tags", tags)

    # An argument is an ordinary value once bound, so a count counts.
    repeat(retries, attempt)

def attempt():
    print("attempt", n())
