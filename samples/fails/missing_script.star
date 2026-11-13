# A script file that does not exist is a file error, not a script error: the
# exit code is 3, as the help text lists for a file that cannot be read.
# This file is only the home of the header; the run names a file that is not
# there.
#
# Run it, from the repository root:
#
#   lark run samples/fails/does_not_exist.star
#
# Expected: exit code 3. Stderr starts with: lark: open
# samples/fails/does_not_exist.star: no such file or directory

def main():
    print("this never runs")
