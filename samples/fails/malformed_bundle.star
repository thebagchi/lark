# A bundle that is not a whole, valid bundle is refused when it is read, and
# the script inside never starts. Make a good bundle, cut it short, and run
# the cut file.
#
#   1. lark compile -o /tmp/b.bin samples/fails/malformed_bundle.star
#      Check: it prints a wrote line, exit code 0.
#   2. head -c 40 /tmp/b.bin > /tmp/cut.bin
#   3. lark run /tmp/cut.bin
#      Check: exit code 1. Stderr is one line:
#      bundle failed: /tmp/cut.bin: unmarshal bundle: proto: cannot parse invalid wire-format data
#
# A file that is not a bundle at all (any text) gives the same error.

def main():
    print("this never runs")
