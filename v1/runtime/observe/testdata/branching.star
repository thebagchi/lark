def alpha():
    return 1

def beta():
    return 2

def unreached():
    return 0

def main():
    a = spawn(alpha)
    b = spawn(beta)
    join(a, b)
