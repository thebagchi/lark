def greet(who):
    return "hello " + who

def main():
    join(spawn(lambda: greet("alice")))
