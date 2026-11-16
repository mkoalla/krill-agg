import asyncio
from pathlib import Path

import typesafe_sdk as ts

from .lib.env import assert_env_var
from .lib.env import var_holds_value as as_type
from .lib.jev import TSClientWrapper


# throws
def init() -> TSClientWrapper:
    # global settings
    # settings_dir = Path.cwd() / assert_env_var("SETTINGS_PATH", as_type(str))
    # settings = load_settings(settings_dir / "settings.toml")
    # #
    # global client
    # client = ollama.Client(
    #     host=f"http://{settings.ollama.name}:{settings.ollama.port}"
    # )
    return TSClientWrapper(api_key=assert_env_var("SYS_ONE_KEY", as_type(str)))


def test_client(client: TSClientWrapper):
    inp = input("What color(s) is the ")[:50]
    print(f"\nWhat color(s) is the '{inp}'?")
    response = client.system_one(
        state={
            "input": inp,
        },
        questions={
            "Response": ts.Score(
                instructions="What color is the `input`?",
                criteria=[
                    "red",
                    "orange",
                    "yellow",
                    "green",
                    "blue",
                    "purple",
                    "brown",
                    "black",
                    "white",
                    "clear or grey",
                ],
            )
        },
    )
    resp: ts.ScoreAnswer = response.answers["Response"]  # type: ignore
    legend: dict[int, str] = resp.legend  # type: ignore
    probs: dict[int, float] = resp.probabilities  # type: ignore

    print(f"\nWith {int(100 * resp.confidence)}% confidence...")
    for p, idx in sorted([(probs[i], i) for i in probs], reverse=True):
        print(legend[idx], p)


def main():
    client = init()
    print(asyncio.run(client.test_query()))
    #


if __name__ == "__main__":
    main()
