# Compiles, and no graph can carry it: a module-level dict of several pairs has
# an order a google.protobuf.Struct does not keep, so derivation refuses rather
# than hand back a graph that prints something else.

BEFORE = {"name": "run-1", "tags": ["slow"], "retries": 0}

def main():
    print(BEFORE)
