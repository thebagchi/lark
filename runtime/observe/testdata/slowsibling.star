def patient():
    sleep(30)

def bad():
    assert(False, "this one breaks")

def main():
    join(spawn(bad), spawn(patient))
