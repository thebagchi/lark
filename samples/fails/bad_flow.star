# run reads a .json file as a flow. A file that is not valid JSON, or a flow that
# fails its check, stops the run before any step runs, with exit code 1.
#
#   1. echo '{bad' > /tmp/bad.json
#   2. lark run /tmp/bad.json
#      Check: exit code 1. Stderr is one line:
#      flow failed: /tmp/bad.json: proto: syntax error (line 1:2): invalid value bad
#   3. echo '{}' > /tmp/empty.json
#   4. lark run /tmp/empty.json
#      Check: exit code 1. Stderr is one line:
#      flow failed: main: function has no body: no form

def main():
    print("this never runs")
