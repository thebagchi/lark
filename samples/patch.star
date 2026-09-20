# RFC 6902 patch edits a document and returns a new one. The input is never
# changed - which is what makes it safe to patch something another thread can
# see, or something state has already frozen.

BEFORE = {"name": "run-1", "tags": ["slow"], "retries": 0}

def main():
    after = patch_json(BEFORE, [
        {"op": "replace", "path": "/retries", "value": 3},
        {"op": "add", "path": "/tags/-", "value": "flaky"},
        {"op": "add", "path": "/owner", "value": "qa"},
        {"op": "remove", "path": "/name"},
        {"op": "test", "path": "/retries", "value": 3},
    ])

    print("before", BEFORE)
    print("after", after)
