def left():
    print("from left")

def right():
    print("from right")

def main():
    first = spawn(left)
    second = spawn(right)
    join(first, second)
