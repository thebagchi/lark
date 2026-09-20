def step():
    sleep(0.05)
    return n()

def main():
    return repeat(step, 3)()
