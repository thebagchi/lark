# Draws from several threads at once, which is what the lock is for.

def draw():
    return [random.int(1, 1000000) for i in range(50)]

def main():
    return join(spawn(draw), spawn(draw), spawn(draw), spawn(draw))
