# A script that uses a plugin name, clock.add, needs the plugin. Run with no
# plugin, the name is undefined. A bundle made with the plugin, then run
# without it, is refused as well: see the round trip below.
#
# Run it, from the repository root:
#
#   lark run samples/fails/needs_plugin.star
#
# Expected: exit code 1. Stderr is this one line:
#
#   script failed: samples/fails/needs_plugin.star:24:11: undefined: clock
#
# Round trip with the plugin (run from the repository root, after make build):
#
#   1. lark compile -plugins bin -o /tmp/n.bin samples/fails/needs_plugin.star
#      Check: it prints a wrote line, exit code 0.
#   2. lark run /tmp/n.bin
#      Check: exit code 1. The error says the bundle needs a plugin that was
#      not given, and names clock.
#   3. lark run -plugins bin /tmp/n.bin
#      Check: it prints 3.

def main():
    print(clock.add(1, 2))
