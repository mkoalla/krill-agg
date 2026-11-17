"""python -m dailygen --out card.png [--title NAME] [--score N] [--day N] [--json data.json]
    [--user SCORE:AVATAR ...] [--user-size N] [--max-push N] [--icon PATH]

Without --json the card uses placeholder numbers from krillion's api: --score is
sent as the "guild mean" and the returned distribution and betterThan are used.
--json renders given data offline: {"day", "title"?, "mean"?, "dist"?, "percentile"?,
"users"?: [{"score", "avatar"?}], "icon"?, "sample"?, "population"?}. "mean" null/
omitted means nobody played; "dist" null/[]/omitted means no distribution that day.
--user adds players either way.
"""

import argparse
import json
import sys

from . import api
from .card import CardData, render_png, render_webp
from .players import Player, parse_user


def main() -> int:
    parser = argparse.ArgumentParser(prog="dailygen", description="render the krillion results card")
    parser.add_argument("--out", required=True, help="png path to write")
    parser.add_argument("--title", default="", help="guild name shown at the top")
    parser.add_argument("--score", type=int, default=api.MIDDLE_SCORE, help="placeholder guild mean (api mode)")
    parser.add_argument("--day", type=int, help="krillion # (default: today; the api only serves today)")
    parser.add_argument("--json", help="render from this file instead of the api")
    parser.add_argument("--scale", type=float, default=2, help="pixel density (default 2)")
    parser.add_argument(
        "--user", action="append", default=[], metavar="SCORE:AVATAR",
        help="a player pin; repeatable. AVATAR may be empty or invalid for a blank disc",
    )
    parser.add_argument("--user-size", type=float, default=28, help="avatar diameter in px (square crop)")
    parser.add_argument("--max-push", type=int, default=4, help="avatars a clashing stack grows before wrapping down")
    parser.add_argument("--icon", help="guild icon image, drawn in place of the AVG label")
    args = parser.parse_args()
    try:
        players = [parse_user(spec) for spec in args.user]
    except ValueError as err:
        parser.error(str(err))

    if args.json:
        with open(args.json) as f:
            raw = json.load(f)
        data = CardData(
            title=raw.get("title", args.title),
            day=raw["day"],
            mean=raw.get("mean"),
            dist=raw.get("dist") or [],
            percentile=raw.get("percentile"),
            icon=raw.get("icon"),
            sample=raw.get("sample"),
            population=raw.get("population", "today's players"),
        )
        players += [Player(score=u["score"], avatar=u.get("avatar")) for u in raw.get("users", [])]
    else:
        day = args.day or api.today()
        stats = api.complete(api.date_of(day), args.score)
        data = CardData(title=args.title, day=day, mean=args.score, dist=stats.dist, percentile=stats.better_than,
                        sample=stats.sample)

    data.players = players
    data.user_size = args.user_size
    data.max_push = args.max_push
    if args.icon:
        data.icon = args.icon

    with open(args.out, "wb") as f:
        # lossless webp when --out ends in .webp, png otherwise
        render = render_webp if args.out.lower().endswith(".webp") else render_png
        f.write(render(data, args.scale))
    print(f"wrote {args.out}: #{data.day}, mean {data.mean}, better than {data.better_than():.2f}%", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
