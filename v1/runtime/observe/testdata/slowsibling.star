def patient():
    sleep(30000)

def bad():
    assert(False, "this one breaks")

def main():
    join(spawn(bad), spawn(patient))
