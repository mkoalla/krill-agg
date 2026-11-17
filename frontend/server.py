"""Serves the static day page (index.html and anything beside it) and the one
endpoint it calls, POST /api/day.

TEMPORARY: /api/day is meant to come from the bot's own data. Until that
exists it's stood in for here by krillion.io itself, which only serves
today's puzzle, so any other day is refused. Krillion's api sends no CORS
headers, so the page can't call it directly; this server fetches it instead.

POST /api/day {"day": null | int}   null means today
  -> 200 {"day", "date", "asOf", "questions": [{"id", "text",
          "answers": [{"answer", "aliases", "tier", "score", "quip", "popularity"}]}]}
     quip and popularity are null when krillion has none
  -> 404 {"error"} for a day that isn't available (anything but today, for now)

python3 server.py [--port 9090]
"""

import argparse
import gzip
import json
import sys
import threading
import time
import urllib.request
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

HERE = Path(__file__).resolve().parent
KRILLION = "https://krillion.io/api"  # TEMPORARY: see the module docstring
POPULARITY_TTL = 45  # seconds; krillion's own page re-polls about this often
MAX_BODY = 1 << 10


def fetch_json(path: str):
    req = urllib.request.Request(KRILLION + path, headers={"User-Agent": "krill-agg-site"})
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.load(resp)


class Today:
    """today's merged day, rebuilt when krillion's day turns over; the
    answer key is fixed for a day, the pick counts are refreshed every so often"""

    def __init__(self):
        self.lock = threading.Lock()
        self.date = None
        self.reveal = None
        self.popularity = None
        self.popularity_at = 0.0
        self.body = None  # gzipped json, rebuilt with the counts

    def get(self) -> bytes:
        with self.lock:
            today = fetch_json("/today")
            if today["date"] != self.date:
                self.date, self.day = today["date"], today["dayNumber"]
                self.reveal = fetch_json(f"/reveal?date={self.date}")
                self.popularity, self.body = None, None
            if self.popularity is None or time.time() - self.popularity_at > POPULARITY_TTL:
                try:
                    self.popularity = fetch_json(f"/answer-popularity?date={self.date}")
                except Exception:  # counts are optional; keep the last ones if any
                    self.popularity = self.popularity or {"asOf": None, "prompts": {}}
                self.popularity_at = time.time()
                self.body = gzip.compress(json.dumps(self.merge()).encode(), compresslevel=6)
            return self.body

    def merge(self) -> dict:
        counts = self.popularity.get("prompts") or {}
        questions = []
        for p in self.reveal["prompts"]:
            picks = counts.get(p["id"]) or {}
            lower = {k.lower(): v for k, v in picks.items()}
            answers = []
            for a in p["answers"]:
                n = picks.get(a["answer"])
                if n is None:  # counted under another spelling
                    n = next((lower[x.lower()] for x in [a["answer"], *a.get("aliases", [])] if x.lower() in lower), None)
                quip = a.get("quip")
                answers.append({
                    "answer": a["answer"],
                    "aliases": a.get("aliases", []),
                    "tier": a["tier"],
                    "score": a["score"],
                    "quip": quip.strip() if isinstance(quip, str) and quip.strip() else None,
                    "popularity": n,
                })
            questions.append({"id": p["id"], "text": p["text"], "answers": answers})
        return {"day": self.day, "date": self.date, "asOf": self.popularity.get("asOf"), "questions": questions}


TODAY = Today()


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(HERE), **kwargs)

    def _json(self, status: int, payload: dict | None = None, gz: bytes | None = None):
        if gz is None:
            gz = gzip.compress(json.dumps(payload).encode())
        raw = gzip.decompress(gz)
        if "gzip" in self.headers.get("Accept-Encoding", ""):
            body, encoding = gz, "gzip"
        else:
            body, encoding = raw, None
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        # browsers unzip as it arrives, so Content-Length (the zipped size) can't
        # measure progress; this is the size the page will actually count
        self.send_header("X-Uncompressed-Length", str(len(raw)))
        self.send_header("Access-Control-Expose-Headers", "X-Uncompressed-Length")
        if encoding:
            self.send_header("Content-Encoding", encoding)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path != "/api/day":
            self._json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(min(length, MAX_BODY)) or b"{}")
            day = body.get("day") if isinstance(body, dict) else None
        except ValueError:
            self._json(400, {"error": "body must be json"})
            return
        try:
            gz = TODAY.get()
        except Exception as err:
            self._json(502, {"error": f"krillion: {err}"})
            return
        if day is not None and day != TODAY.day:
            self._json(404, {"error": f"only today's day (#{TODAY.day}) is available for now"})
            return
        self._json(200, gz=gz)

    def end_headers(self):
        # the page is tiny and changes as it's worked on; never serve it stale
        if self.command == "GET":
            self.send_header("Cache-Control", "no-cache")
        super().end_headers()

    def log_message(self, fmt, *args):
        print(f"{self.address_string()} {fmt % args}", file=sys.stderr)


def main() -> int:
    parser = argparse.ArgumentParser(description="serve the krillion day page")
    parser.add_argument("--port", type=int, default=9090)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("0.0.0.0", args.port), Handler)
    print(f"serving {HERE} on :{args.port}", file=sys.stderr)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
