# A loaded module declaring an argument of its own, so that a run binds every
# unit it initialises and not only the entry.

load("shared.star", "label")

host = arg("host", "localhost")

def main():
    return label(host)
