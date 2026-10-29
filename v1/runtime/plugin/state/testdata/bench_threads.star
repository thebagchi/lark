# Sixteen threads write the same name, each in a loop.
def writer():
    for i in range(100):
        state.set("k", i)

def main():
    handles = []

    for _ in range(16):
        handles.append(spawn(writer))

    join(*handles)

    return state.get("k")
