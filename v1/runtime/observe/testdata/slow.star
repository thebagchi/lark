def slow():
    sleep(30000)

def main():
    h = spawn(slow)
    join(h)
