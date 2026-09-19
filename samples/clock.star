# time is go.starlark.net's own module: instants, durations, and arithmetic
# over both.
#
# Only the duration parts are printed here. An instant formats in the machine's
# own timezone, so printing one would make this sample's output differ between
# machines rather than between runs - which is worse, because it looks stable
# until somebody else tries it.

def main():
    started = time.from_timestamp(1600000000)
    budget = time.parse_duration("1h30m")

    deadline = started + budget

    return {
        "budget in seconds": budget.seconds,
        "how long until the deadline": str(deadline - started),
        "a minute is": str(time.minute),
        "an hour holds": (time.hour).seconds / (time.minute).seconds,
        "now is an instant": type(time.now()) == "time.time",
    }
