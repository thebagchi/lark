# Loading another script. The path is resolved beside this file, so
# "strings.star" means samples/strings.star.

load("strings.star", "shout", "join_with")

def main():
    print(join_with(" ", [shout("loaded"), shout("from"), shout("a module")]))
