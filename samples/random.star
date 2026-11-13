# random draws numbers, picks, shuffles and makes bytes. Importing the plugin
# is what enables it, and the lark command does.
#
# Seed first, or the run is different every time. random.seed(n) makes this
# run's source hand out one fixed sequence, so the output below is the same on
# every machine. Without the call the source starts from crypto/rand.
#
# A seed repeats exactly only in one thread. Threads share one source, so which
# thread gets which number depends on the order they ask. This script draws on
# the main thread only. A script that needs a thread to repeat gives that
# thread its own seed and draws only there.
#
# Run it:
#
#   lark run samples/random.star
#
# Expected output: the same lines on every run. The check is to run it twice
# and compare; comment out random.seed(7) and the numbers change each time.

def main():
    random.seed(7)

    print("die", random.int(1, 6))
    print("float", random.float())
    print("choice", random.choice(["red", "green", "blue"]))
    print("bytes", hex.from_bytes(random.bytes(4)))

    deck = [1, 2, 3, 4, 5]
    # shuffle gives back a new list; it does not reorder the one it is given.
    print("shuffled", random.shuffle(deck))
    print("original", deck)

    # Seeding again restarts the sequence.
    random.seed(7)
    print("die again", random.int(1, 6))
