# Joining while a name is held. The update takes "k" before it calls its
# function, so a join inside that function waits for a thread that may need
# "k" - and nothing in a script can end that wait.
#
# Refused at the join. The child never runs.

def bump():
    state.update("k", lambda v: 1)

def main():
    state.update("k", lambda v: join(spawn(bump)))
