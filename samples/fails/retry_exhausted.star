# retry(3, fn) runs fn up to three times, and only an assertion makes it try
# again. fail() is not retried, so the first attempt ends the run.
#
# Run it, from the repository root:
#
#   lark run samples/fails/retry_exhausted.star
#
# Expected: exit code 1. Stderr starts with this line (a cause line may follow):
#
#   script failed: retry(3, attempt) attempt 1: samples/fails/retry_exhausted.star:13: fail: still broken

def attempt():
    fail("still broken")

def main():
    retry(3, attempt)
