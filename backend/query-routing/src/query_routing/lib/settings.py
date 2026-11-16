# may be used later


# import tomllib
# from pathlib import Path

# from pydantic import BaseModel, ConfigDict, Field


# class RouterSettings(BaseModel):
#     model_config = ConfigDict(frozen=True)
#     #
#     ollama: OllamaSettings


# class OllamaSettings(BaseModel):
#     model_config = ConfigDict(frozen=True)
#     #
#     name: str
#     port: int = Field(gt=0, lt=2**16)
#     models: OllamaModels


# class OllamaModels(BaseModel):
#     model_config = ConfigDict(frozen=True)
#     #
#     router: str
#     simple: str
#     complex: str


# def load(p: Path) -> RouterSettings:
#     with open(p, "rb") as file:
#         config = tomllib.load(file)
#     return RouterSettings.model_validate(config)
