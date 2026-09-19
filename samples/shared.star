# Module scope is frozen before anything concurrent runs, so two threads cannot
# share data through a global. state is how they pass anything to each other.
#
# A store belongs to one execution: run this twice and the count is 1 both
# times.

def collect(name):
    state.set(name, name.upper())

def main():
    join(spawn(gather_a), spawn(gather_b))

    return jsonpath_summary()

def gather_a():
    collect("alpha")

def gather_b():
    collect("beta")

def jsonpath_summary():
    doc = {"found": [state.get("alpha"), state.get("beta")]}

    return extract_json(doc, "/found/1")
