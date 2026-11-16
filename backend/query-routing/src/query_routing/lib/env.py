import os
from dataclasses import dataclass
from enum import Enum
from typing import Any, Callable, Never


def assert_env_var[Transformed](
    s: str, transform: Callable[[str | None], tuple[bool, Transformed | None]]
) -> Transformed:
    var = transform(os.getenv(s))
    assert var[0] and var[1] is not None, f"invalid .env variable: {s}"
    return var[1]


def var_holds_value[T](constructor: Callable[[Any], T]):
    def wrapped(x: Any) -> tuple[bool, T | None]:
        if x is None or x == "":
            return (False, None)
        #
        try:
            val = constructor(x)
            return (val is not None, val)
        except Exception:
            return (False, None)

    return wrapped
