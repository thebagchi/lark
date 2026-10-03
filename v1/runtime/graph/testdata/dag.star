def task_a(region):
    pass

def task_b():
    pass

def task_c():
    pass

def task_d(both):
    pass

def task_e():
    pass

def main():
    a = spawn(lambda: task_a("west"))
    b = spawn(task_b)
    join(a)
    c = spawn(task_c)
    got = join(a, b)
    d = spawn(lambda: task_d(got))
    join(b)
    e = spawn(task_e)
    join(c, d, e)
    pass
