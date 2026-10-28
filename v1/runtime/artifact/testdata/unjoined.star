def boom():
    explode()
    return 1

def main():
    spawn(boom)
    return "ok"
