# A thread nobody joins, storing something that is not data.
#
# Its own failure would reach the report and never become the run's result, so
# without the whole run stopping this script would print "run survived" and
# leave nothing in the store, saying nothing about why.

def helper():
    return 1

def pick():
    return helper

def bad():
    state.set("k", pick())

def main():
    spawn(bad)

    sleep(0.1)

    print("run survived")
