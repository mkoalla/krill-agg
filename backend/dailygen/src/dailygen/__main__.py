"""python -m dailygen --out card.png [--score N] [--day N] [--json data.json]

Without --json the card uses placeholder numbers from krillion's api: --score is
sent as the "guild mean" and the returned distribution and betterThan are used.
--json renders given data offline: {"day", "mean", "dist", "percentile"?}.
"""

import argparse
import json
import sys

from . import api
from .card import CardData, render_png


def main() -> int:
    parser = argparse.ArgumentParser(prog="dailygen", description="render the krillion results card")
    parser.add_argument("--out", required=True, help="png path to write")
    parser.add_argument("--score", type=int, default=api.MIDDLE_SCORE, help="placeholder guild mean (api mode)")
    parser.add_argument("--day", type=int, help="krillion # (default: today; the api only serves today)")
    parser.add_argument("--json", help="render from this file instead of the api")
    parser.add_argument("--scale", type=float, default=2, help="pixel density (default 2)")
    args = parser.parse_args()

    if args.json:
        with open(args.json) as f:
            raw = json.load(f)
        data = CardData(day=raw["day"], mean=raw["mean"], dist=raw["dist"], percentile=raw.get("percentile"))
    else:
        day = args.day or api.today()
        stats = api.complete(api.date_of(day), args.score)
        data = CardData(day=day, mean=args.score, dist=stats.dist, percentile=stats.better_than)

    with open(args.out, "wb") as f:
        f.write(render_png(data, args.scale))
    print(f"wrote {args.out}: #{data.day}, mean {data.mean}, better than {data.better_than():.2f}%", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
