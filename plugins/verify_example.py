#!/usr/bin/env python3
"""Example hydr0g3n verification plugin.

The plugin receives a JSON payload on stdin, re-issues a GET request against the
reported URL and checks the response body against a configurable regular
expression. The JSON response is written to stdout for hydr0g3n to consume.
"""

import json
import os
import re
import sys
import urllib.error
import urllib.request
from typing import Any, Dict

DEFAULT_PATTERN = r"(?i)success"


def load_event() -> Dict[str, Any]:
    try:
        return json.load(sys.stdin)
    except json.JSONDecodeError as exc:  # pragma: no cover - example script
        print(f"invalid JSON from hydr0g3n: {exc}", file=sys.stderr)
        sys.exit(1)


def verify(url: str, pattern: str) -> bool:
    request = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            body = response.read().decode("utf-8", errors="ignore")
    except urllib.error.URLError as exc:  # pragma: no cover - example script
        print(f"verification request failed: {exc}", file=sys.stderr)
        return False

    return bool(re.search(pattern, body))


def main() -> None:
    event = load_event()
    url = event.get("url")
    if not url:
        print("missing url in hydr0g3n payload", file=sys.stderr)
        sys.exit(1)

    pattern = os.environ.get("HYDRO_VERIFY_REGEX", DEFAULT_PATTERN)
    try:
        result = verify(url, pattern)
    except re.error as exc:  # pragma: no cover - example script
        print(f"invalid regular expression: {exc}", file=sys.stderr)
        sys.exit(1)

    json.dump({"verify": result}, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
