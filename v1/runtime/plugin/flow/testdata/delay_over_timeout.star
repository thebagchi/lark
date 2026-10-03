def step():
    return n()

def waits():
    return repeat(2, step, 30000)

def main():
    return timeout(200, waits)
