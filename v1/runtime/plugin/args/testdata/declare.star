# Every shape an argument can take, and the two names disagreeing on purpose:
# what a caller supplies is "host", what the script calls it is "db".

db = arg("host", "db.internal")
port = arg("port", 5432)
ratio = arg("ratio", 0.25)
tls = arg("tls", True)
spare = arg("spare", None)
tags = arg("tags", ["a", "b"])
labels = arg("labels", {"tier": "gold"})
keyword = arg("keyword", default = "set either way round")

def main():
    return [db, port, ratio, tls, spare, tags, labels, keyword]
