# json comes from a plugin. The host enabled it by importing the package; this
# script did not have to ask.

def main():
    return json.encode({"runtime": "lark", "threads": [0, 1, 2]})
