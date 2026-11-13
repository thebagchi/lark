# -args names a key this script never declares with arg(). The key is not an
# error that stops the run: lark warns about it, and the run goes on.
#
# Run it, from the repository root:
#
#   lark run -args '{"nope": 1}' samples/fails/unknown_args.star
#
# Expected: exit code 0. The script prints its host. The warning about the
# unknown key "nope" goes to stderr, as this line:
# lark: argument "nope" is not declared by the script
# Check: the same run without -args prints no warning.

host = arg("host", "localhost")

def main():
    print("host", host)
