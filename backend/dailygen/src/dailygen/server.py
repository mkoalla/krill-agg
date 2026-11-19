"""HTTP front for the card renderer: the bot POSTs /render and gets back where
the card (lossless webp) landed under the shared data dir.

POST /render {"guild", "title", "day", "images", "mean"?, "dist"?, "percentile"?,
              "users"?: [{"score"}], "user_size"?, "max_push"?, "icon"?, "sample"?,
              "population"?, "deadline"?}
  -> 200 {"path": "<guild>/<day>/daily_<day>.webp"}   (relative to the data dir)
  -> 400/500 {"error": "..."}
  -> 504 {"error": ...} when "deadline" (unix ms) passed before the card was
     written: the bot gave up waiting, so nothing is drawn or saved
Avatars are implicit: users[i] uses <guild>/<day>/<images>/<i>.png when it
exists. "icon" names the guild's icon in that same folder (e.g. "guild.png").
"images" is a required bare folder name: the bot makes it as a scratch dir for
this render's avatars/icon and removes it afterward, so we only ever read from
it, never create or delete it ourselves.
"mean" is the guild's average score; null or omitted when nobody in the guild
played. "dist" is krillion's 20 bucket counts; null, [], or all-zero when
there's no distribution that day. "population" overrides the "OF ..." footer
wording (default "today's players").
POST /trend {"guild", "user", "name", "title", "guild_name", "images", "points",
             "ma"?, "window"?, "rank"?, "deadline"?}
  -> 200 {"path": "<guild>/users/<user>/<name>"}, overwriting
"name" is the card's file name (a bare *.webp): the bot names cards by what
they show and only asks for one it doesn't already have.
One player's games over a period (/topu): "points" is [{"day", "score"}] per
game, "ma" [{"day", "value"}] the moving average the bot worked out over a
"window" (e.g. "1-WEEK"), "rank" their place on the guild's best-score board.
The avatar is <guild>/users/<user>/<images>/0.png; errors are as /render's.
POST /median {"guild", "name", "title", "images", "first", "last",
              "guild_series", "global_series"?, "window"?, "percentile"?,
              "icon"?, "deadline"?}
  -> 200 {"path": "<guild>/server/<name>"}, overwriting
/topg's card over days first..last: the guild's daily medians (yellow) against
every krillion player's (cyan), faint under their moving averages. Each series
is {"points": [{"day", "value"}], "ma"?: [{"day", "value"}], "median"?,
"dives"?}; "median" is over every game in the span, not of the daily ones.
guild_series needs points. "percentile" is the share of everyone's games under
the guild's median ("BETTER THAN" line). "icon" names the guild's icon in
<guild>/server/<images>/, riding the end of the yellow line.
GET /health -> 200
"""

import argparse
import json
import os
import re
import sys
import tempfile
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from . import median, trend
from .card import CardData, render_webp
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
    return data, card_dir / f"daily_{day}.webp"


def build_trend(body: dict, data_dir: Path) -> tuple[trend.TrendData, Path]:
    """/trend request -> its card and where to write it. raises BadRequest on bad input"""
    guild = str(body.get("guild", ""))
    user = str(body.get("user", ""))
    if not GUILD_ID.match(guild) or not GUILD_ID.match(user):  # keeps paths inside data_dir
        raise BadRequest("guild and user must be discord ids")
    images_name = str(body.get("images", ""))
    if not FILE_NAME.match(images_name):
        raise BadRequest("images must be a folder name")
    name = str(body.get("name", ""))
    if not FILE_NAME.match(name) or not name.endswith(".webp"):
        raise BadRequest("name must be a .webp file name")
    try:
        points = sorted((int(p["day"]), float(p["score"])) for p in body["points"])
        ma = sorted((int(p["day"]), float(p["value"])) for p in body.get("ma") or [])
        rank = None if body.get("rank") is None else int(body["rank"])
    except (KeyError, TypeError, ValueError) as err:
        raise BadRequest(f"bad field: {err}") from None
    if not points:
        raise BadRequest("points must not be empty")
    user_dir = data_dir / guild / "users" / user
    data = trend.TrendData(
        title=str(body.get("title", "")),
        guild=str(body.get("guild_name", "")),
        points=points,
        ma=ma,
        window=str(body.get("window", ""))[:20],
        rank=rank,
        avatar=str(user_dir / images_name / "0.png"),
    )
    return data, user_dir / name


def _series(raw) -> median.Series:
    """one /median series: {"points", "ma"?, "median"?, "dives"?}"""
    if raw is None:
        return median.Series()
    if not isinstance(raw, dict):
        raise BadRequest("a series must be an object")
    return median.Series(
        points=sorted((int(p["day"]), float(p["value"])) for p in raw.get("points") or []),
        ma=sorted((int(p["day"]), float(p["value"])) for p in raw.get("ma") or []),
        median=None if raw.get("median") is None else float(raw["median"]),
        dives=int(raw.get("dives") or 0),
    )


def build_median(body: dict, data_dir: Path) -> tuple[median.MedianData, Path]:
    """/median request -> its card and where to write it. raises BadRequest on bad input"""
    guild = str(body.get("guild", ""))
    if not GUILD_ID.match(guild):  # keeps paths inside data_dir
        raise BadRequest("guild must be a discord id")
    images_name = str(body.get("images", ""))
    if not FILE_NAME.match(images_name):
        raise BadRequest("images must be a folder name")
    name = str(body.get("name", ""))
    if not FILE_NAME.match(name) or not name.endswith(".webp"):
        raise BadRequest("name must be a .webp file name")
    icon = str(body.get("icon") or "")
    if icon and not FILE_NAME.match(icon):
        raise BadRequest("icon must be a file name")
    try:
        first, last = int(body["first"]), int(body["last"])
        guild_series = _series(body.get("guild_series"))
        everyone = _series(body.get("global_series"))
        percentile = None if body.get("percentile") is None else float(body["percentile"])
    except (KeyError, TypeError, ValueError) as err:
        raise BadRequest(f"bad field: {err}") from None
    if first < 1 or last < first:
        raise BadRequest("first and last must be days, first..last")
    if not guild_series.points:
        raise BadRequest("no guild medians to draw")
    server_dir = data_dir / guild / "server"
    data = median.MedianData(
        title=str(body.get("title", "")),
        first=first,
        last=last,
        window=str(body.get("window", ""))[:20],
        guild=guild_series,
        everyone=everyone,
        percentile=percentile,
        icon=str(server_dir / images_name / icon) if icon else None,
    )
    return data, server_dir / name


# each card's endpoint: how to read its request and draw it
ROUTES = {
    "/render": (build, render_webp),
    "/trend": (build_trend, trend.render_webp),
    "/median": (build_median, median.render_webp),
}


def parse_deadline(body: dict) -> float | None:
    """the bot's optional "deadline": unix milliseconds it stops waiting at"""
    raw = body.get("deadline")
    if raw is None:
        return None
    if isinstance(raw, bool) or not isinstance(raw, (int, float)):
        raise BadRequest("deadline must be unix milliseconds")
    return raw / 1000


def past(deadline: float | None) -> bool:
    return deadline is not None and time.time() > deadline


def write_atomic(path: Path, content: bytes) -> None:
    """readers never see a half-written card"""
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=".daily_", suffix=path.suffix)
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
            route = ROUTES.get(self.path)
            if route is None:
                self._reply(404, {"error": "not found"})
                return
            parse, render = route
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= MAX_BODY:
                    raise BadRequest("missing or oversized body")
                body = json.loads(self.rfile.read(length))
                if not isinstance(body, dict):
                    raise BadRequest("body must be a json object")
                data, out = parse(body, data_dir)
                deadline = parse_deadline(body)
            except (BadRequest, json.JSONDecodeError, ValueError) as err:
                self._reply(400, {"error": str(err)})
                return
            # the bot has stopped waiting past its deadline: a card drawn or
            # written after it would never be posted, so skip both
            if past(deadline):
                self._reply(504, {"error": "past the bot's deadline"})
                return
            try:
                card = render(data)
                if past(deadline):
                    self._reply(504, {"error": "past the bot's deadline"})
                    return
                write_atomic(out, card)
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
