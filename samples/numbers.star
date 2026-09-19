# math is go.starlark.net's own module.
#
# What it returns is worth knowing before relying on it: floor and ceil hand
# back integers, everything else hands back floats - so round(2.5) is 3.0, not
# 3. Starlark keeps ints and floats apart, and that difference shows up the
# first time a result is compared or used as an index.

def main():
    return {
        "sqrt(2)": math.sqrt(2),
        "floor(-1.5) is an int": math.floor(-1.5),
        "ceil(-1.5) is an int": math.ceil(-1.5),
        "round(2.5) is a float": math.round(2.5),
        "pow(2, 10)": math.pow(2, 10),
        "hypot(3, 4)": math.hypot(3, 4),
        "degrees(pi)": math.degrees(math.pi),
        "log(e)": math.log(math.e),
    }
