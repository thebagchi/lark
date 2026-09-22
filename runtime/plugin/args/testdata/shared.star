# The module the entry point loads.

prefix = arg("prefix", "svc")

def label(host):
    return "%s://%s" % (prefix, host)
