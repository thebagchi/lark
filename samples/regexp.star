# regexp gives a script regular expressions. The engine is RE2, which matches in
# time linear in the length of the text, so no pattern can make a run hang. The
# price: no lookahead, lookbehind or backreferences. A pattern that asks for one
# is refused when it compiles.
#
# Each function takes the pattern first and the text second:
#
#   match    a match at the start of the text only, or None
#   search   the first match anywhere, or None
#   findall  every match, in order
#   sub      replace matches; $1 and ${name} name a group, and $$ is a dollar
#   split    cut the text at each match
#   quote    escape text so it matches itself
#
# A match is a struct with text, start, end, groups and named. Named groups use
# (?P<name>x).
#
# Run it:
#
#   lark run samples/regexp.star
#
# Expected output: one line per call. Each result is fixed by the input.

LINE = "2026-11-05 build 42 passed, build 43 failed"

def main():
    m = regexp.match("(\\d+)-(\\d+)-(\\d+)", LINE)
    print("match", m.text, m.groups, m.start, m.end)

    # match is anchored: "build" is in the text, but not at its start.
    print("no match at start", regexp.match("build", LINE))

    s = regexp.search("build (?P<n>\\d+) (?P<state>\\w+)", LINE)
    print("search", s.text, s.named)

    print("findall", [f.text for f in regexp.findall("build \\d+", LINE)])

    print("sub", regexp.sub("build (\\d+)", "job-$1", LINE))
    print("sub, first only", regexp.sub("build (\\d+)", "job-$1", LINE, count = 1))
    print("split", regexp.split(",\\s*", LINE))
    print("quote", regexp.quote("1+1=2?"))
