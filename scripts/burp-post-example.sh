#!/usr/bin/env bash
# shellcheck disable=SC2312
set -euo pipefail

PORT="${1:-8899}"

python3 - <<'PY' "${PORT}"
import base64
import http.server
import json
import sys
from datetime import datetime

PORT = int(sys.argv[1])

class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('content-length', '0'))
        body = self.rfile.read(length)
        try:
            payload = json.loads(body.decode('utf-8'))
        except json.JSONDecodeError as exc:
            print(f"[{datetime.utcnow().isoformat()}] invalid JSON payload: {exc}")
            self.send_response(400)
            self.end_headers()
            return

        print(f"\n[{datetime.utcnow().isoformat()}] received finding for {payload.get('url')}")
        print(json.dumps(payload, indent=2))

        for label, section in (("Request", payload.get("request")), ("Response", payload.get("response"))):
            if not section:
                continue
            if section.get("base64"):
                try:
                    decoded = base64.b64decode(section.get("value", "")).decode('utf-8', errors='replace')
                except Exception as exc:  # pragma: no cover - diagnostic only
                    print(f"failed to decode {label.lower()} body: {exc}")
                else:
                    print(f"--- {label} ---\n{decoded}")

        self.send_response(204)
        self.end_headers()

    def log_message(self, format, *args):  # pragma: no cover - silence default logging
        return

with http.server.ThreadingHTTPServer(('0.0.0.0', PORT), Handler) as server:
    print(f"Listening on http://0.0.0.0:{PORT} for --burp-host payloads...")
    server.serve_forever()
PY
