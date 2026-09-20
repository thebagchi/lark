def slow():
    sleep(30)

def main():
    h = spawn(slow)
    join(h)
