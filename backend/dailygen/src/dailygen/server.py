"""HTTP front for the card renderer: the bot POSTs /render and gets back where
the png landed under the shared data dir.

POST /render {"guild", "title", "day", "images", "mean"?, "dist"?, "percentile"?,
              "users"?: [{"score"}], "user_size"?, "max_push"?, "icon"?, "sample"?,
              "population"?}
  -> 200 {"path": "<guild>/<day>/daily_<day>.png"}   (relative to the data dir)
  -> 400/500 {"error": "..."}
Avatars are implicit: users[i] uses <guild>/<day>/<images>/<i>.png when it
exists. "icon" names the guild's icon in that same folder (e.g. "guild.png").
"images" is a required bare folder name: the bot makes it as a scratch dir for
this render's avatars/icon and removes it afterward, so we only ever read from
it, never create or delete it ourselves.
"mean" is the guild's average score; null or omitted when nobody in the guild
played. "dist" is krillion's 20 bucket counts; null, [], or all-zero when
there's no distribution that day. "population" overrides the "OF ..." footer
wording (default "today's players").
GET /health -> 200
"""

import argparse
import json
import os
import re
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from .card import CardData, render_png
from .players import Player

DEFAULT_PORT = 40404
# data/dailygen/ on the bind mount shared with the bot
DEFAULT_DATA_DIR = "/app/data/dailygen"
MAX_BODY = 1 << 20  # requests are small; refuse anything silly

GUILD_ID = re.compile(r"^\d{1,20}$")
FILE_NAME = re.compile(r"^\w[\w.-]{0,63}$")  # a bare name, so it can't leave its folder


class BadRequest(ValueError):
    pass


def build(body: dict, data_dir: Path) -> tuple[CardData, Path]:
    """request -> card and where to write it. raises BadRequest on bad input"""
    guild = str(body.get("guild", ""))
    if not GUILD_ID.match(guild):  # also keeps every path inside data_dir
        raise BadRequest("guild must be a discord id")
    images_name = str(body.get("images", ""))
    if not FILE_NAME.match(images_name):  # required: the bot's scratch dir for this render
        raise BadRequest("images must be a folder name")
    try:
        day = int(body["day"])
        mean = None if body.get("mean") is None else float(body["mean"])
        dist = [] if body.get("dist") is None else [float(c) for c in body["dist"]]
        percentile = None if body.get("percentile") is None else float(body["percentile"])
        users = [float(u["score"]) for u in body.get("users") or []]
        user_size = float(body.get("user_size", 28))
        max_push = int(body.get("max_push", 4))
        sample = None if body.get("sample") is None else int(body["sample"])
    except (KeyError, TypeError, ValueError) as err:
        raise BadRequest(f"bad field: {err}") from None
    icon = str(body.get("icon") or "")
    if icon and not FILE_NAME.match(icon):
        raise BadRequest("icon must be a file name")
    if day < 0 or user_size <= 0:
        raise BadRequest("day and user_size must be positive")
    population = body.get("population")
    if population is None:
        population = "today's players"
    elif not isinstance(population, str):
        raise BadRequest("population must be a string")
    else:
        population = population[:40]

    card_dir = data_dir / guild / str(day)
    images = card_dir / images_name  # the bot's scratch dir; we only ever read from it
    players = [Player(score=s, avatar=str(images / f"{i}.png")) for i, s in enumerate(users)]
    data = CardData(
        title=str(body.get("title", "")),
        day=day,
        mean=mean,
        dist=dist,
        percentile=percentile,
        players=players,
        user_size=user_size,
        max_push=max_push,
        icon=str(images / icon) if icon else None,
        sample=sample,
        population=population,
    )
    return data, card_dir / f"daily_{day}.png"


def write_atomic(path: Path, content: bytes) -> None:
    """readers never see a half-written png"""
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=".daily_", suffix=".png")
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(content)
        os.chmod(tmp, 0o644)  # mkstemp makes it owner-only; the bot reads it too
        os.replace(tmp, path)
    except BaseException:
        Path(tmp).unlink(missing_ok=True)
        raise


def make_handler(data_dir: Path):
    class Handler(BaseHTTPRequestHandler):
        def _reply(self, status: int, payload: dict) -> None:
            raw = json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self):
            if self.path == "/health":
                self._reply(200, {"ok": True})
            else:
                self._reply(404, {"error": "not found"})

        def do_POST(self):
            if self.path != "/render":
                self._reply(404, {"error": "not found"})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= MAX_BODY:
                    raise BadRequest("missing or oversized body")
                body = json.loads(self.rfile.read(length))
                if not isinstance(body, dict):
                    raise BadRequest("body must be a json object")
                data, out = build(body, data_dir)
            except (BadRequest, json.JSONDecodeError, ValueError) as err:
                self._reply(400, {"error": str(err)})
                return
            try:
                write_atomic(out, render_png(data))
            except Exception as err:  # the bot shows this, so keep it short
                self._reply(500, {"error": f"render failed: {err}"})
                return
            self._reply(200, {"path": str(out.relative_to(data_dir))})

        def log_message(self, fmt, *args):
            print(f"{self.address_string()} {fmt % args}", file=sys.stderr)

    return Handler


def serve(port: int, data_dir: Path) -> ThreadingHTTPServer:
    return ThreadingHTTPServer(("0.0.0.0", port), make_handler(data_dir))


def main() -> int:
    parser = argparse.ArgumentParser(prog="dailygen.server", description="serve the card renderer over http")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT)
    parser.add_argument("--data-dir", default=DEFAULT_DATA_DIR)
    args = parser.parse_args()
    data_dir = Path(args.data_dir).resolve()
    server = serve(args.port, data_dir)
    print(f"dailygen listening on :{args.port}, data in {data_dir}", file=sys.stderr)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
