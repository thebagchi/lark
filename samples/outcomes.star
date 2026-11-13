# outcomes waits for every handle given and returns one record for each, in the
# order they were passed, whether the handle succeeded or not.
#
# join stops at the first failure. outcomes does not: it stops no thread, and a
# script that wants to carry on after a thread it chose to ignore reads each
# record and decides. Use join when one failure should end the work, and
# outcomes when you want the whole picture.
#
# Run it:
#
#   lark run samples/outcomes.star
#
# A record is a dict with four keys:
#
#   ok      True when the thread returned, False when it failed
#   value   what the thread returned, or None
#   error   the error text, or None
#   kind    "fail" for an ordinary failure, "cancelled" for a thread someone
#           cancelled, and None for success
#
# Expected output: one line per thread, then the failure count. The error text
# is the whole message, and it starts with the file and line where fail() was
# called. This script prints a check on the text rather than the text, because
# the line number changes when the script is regenerated from a flow.

def fine():
    return "all good"

def broken():
    fail("this thread failed")

def late():
    sleep(10)

    return "finished late"

def main():
    records = outcomes(spawn(fine), spawn(broken), spawn(late))

    for record in records:
        print("ok", record["ok"], "| kind", record["kind"], "| value", record["value"])

    # The error text is None for a thread that succeeded, so test ok first.
    failed = [r for r in records if not r["ok"]]
    print("failures", len(failed))
    print("error names the cause", "this thread failed" in failed[0]["error"])
