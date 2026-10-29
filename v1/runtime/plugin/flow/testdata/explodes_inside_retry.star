def boom():
    explode()

def main():
    return retry(3, boom)
