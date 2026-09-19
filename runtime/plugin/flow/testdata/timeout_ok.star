def quick():
    return "finished in time"

def main():
    return timeout(quick, 5)()
