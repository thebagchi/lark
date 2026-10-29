def boom():
    explode()
    return 1

def main():
    spawn(boom)
    sleep(30000)
    return "ok"
