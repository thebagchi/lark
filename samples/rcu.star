# The store works like read-copy-update.
#
# What is stored is frozen, so threads reading one name at once can never be
# handed something another can change underneath them. get therefore returns a
# copy: a script owns what it reads, changes it freely, and nothing it does is
# visible anywhere until it publishes the result.
#
# Read, copy, update.

def main():
    state.set("findings", ["one"])

    # A copy. Appending to it changes nothing that anyone else can see.
    mine = state.get("findings")
    mine.append("two")

    unpublished = state.get("findings")

    # Publishing is what makes it visible.
    state.set("findings", mine)

    print("my copy", mine)
    print("the store, before I published", unpublished)
    print("the store, after", state.get("findings"))
