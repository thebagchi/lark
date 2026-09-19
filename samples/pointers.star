# RFC 6901 pointers read a value out of a document without the script walking
# it step by step.
#
# extract_json answers None when a path is missing, and match_json answers
# False - a document not having something is an answer, not a failure. A
# pointer that is not a pointer is still an error, because that is the script
# being wrong rather than the document.

REPORT = {
    "session": {"id": "abc-123", "retries": 0},
    "steps": [
        {"name": "connect", "ok": True},
        {"name": "verify", "ok": False},
    ],
    "a/b": "a key with a slash in it",
}

def main():
    return {
        "id": extract_json(REPORT, "/session/id"),
        "second step": extract_json(REPORT, "/steps/1/name"),
        "escaped slash": extract_json(REPORT, "/a~1b"),
        "how many steps": len_json(REPORT, "/steps"),
        "absent": extract_json(REPORT, "/session/nothing"),
        "retries are zero": match_json(REPORT, "/session/retries", 0),
        "found by name": find_key(REPORT, "id"),
    }
