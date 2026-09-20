# math is go.starlark.net's own module.
#
# What it returns is worth knowing before relying on it: floor and ceil hand
# back integers, everything else hands back floats - so round(2.5) is 3.0, not
# 3. Starlark keeps ints and floats apart, and that difference shows up the
# first time a result is compared or used as an index.

def main():
    print("sqrt(2)", math.sqrt(2))
    print("floor(-1.5) is an int", math.floor(-1.5))
    print("ceil(-1.5) is an int", math.ceil(-1.5))
    print("round(2.5) is a float", math.round(2.5))
    print("pow(2, 10)", math.pow(2, 10))
    print("hypot(3, 4)", math.hypot(3, 4))
    print("degrees(pi)", math.degrees(math.pi))
    print("log(e)", math.log(math.e))
