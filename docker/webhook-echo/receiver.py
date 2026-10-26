#!/usr/bin/env python3
"""
A webhook receiver, for testing this project's outbound webhooks.

Deliberately dumb and deliberately honest: it accepts a POST, keeps the headers
and body of the last request it saw, and hands them back on GET /last. It does
NOT verify the signature - the end-to-end test does that itself, with an
independent HMAC, because a receiver that used our own signer would only prove
the two agree with each other.

`POST /fail` answers 500, so the retry and dead-letter paths have somewhere real
to point at.

Runs only under the `e2e` compose profile. It is test infrastructure, not part of
the stack.
"""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

last = {"headers": {}, "body": ""}


class Receiver(BaseHTTPRequestHandler):
    def do_POST(self):  # noqa: N802 - the base class names it
        length = int(self.headers.get("content-length", 0))
        body = self.rfile.read(length).decode("utf-8") if length else ""

        if self.path == "/fail":
            self._reply(500, {"error": "deliberately refusing"})
            return

        # Header names arrive case-insensitively; lower them so the test does
        # not have to guess which casing the client used.
        last["headers"] = {k.lower(): v for k, v in self.headers.items()}
        last["body"] = body
        self._reply(200, {"received": True})

    def do_GET(self):  # noqa: N802
        self._reply(200, last if self.path == "/last" else {"ok": True})

    def _reply(self, status, payload):
        encoded = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, *args):
        # Quiet: the container log is read when something is wrong, and one line
        # per delivery attempt buries it.
        pass


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 9000), Receiver).serve_forever()
