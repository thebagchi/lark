# Draws a fixed number of values from a seeded source, on one thread, which is
# where a seed repeats exactly.

def main():
    random.seed(42)

    return [random.int(1, 1000000) for i in range(10)]
