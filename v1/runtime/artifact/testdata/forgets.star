def forever():
    total = 0
    for i in range(100000000):
        total += i
    return total

def main():
    spawn(forever)
    return "done"
