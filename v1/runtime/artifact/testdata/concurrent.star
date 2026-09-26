load("lib.star", "double")

def left():
    return double(2)

def right():
    return double(3)

def main():
    got = join(spawn(left), spawn(right))
    return got[0] + got[1]
