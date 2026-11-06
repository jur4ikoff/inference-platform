#!/usr/bin/env python3
"""Print human-readable output from a stream of go test -json events."""
import json
import sys

for line in sys.stdin:
    event = json.loads(line)
    if event.get("Action") == "output":
        sys.stdout.write(event.get("Output", ""))
        sys.stdout.flush()
