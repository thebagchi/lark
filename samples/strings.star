# A library. It defines no main, because nothing starts a run here - a module
# is loaded, not run.

def shout(word):
    return word.upper()

def join_with(sep, parts):
    return sep.join(parts)
