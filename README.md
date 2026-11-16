# krill-agg
## Dependencies
 - [**`make`**](https://www.google.com/search?q=what%20is%20build%20essential)
 - [**`Docker`**](https://www.docker.com/)
 - [**`go`**](https://go.dev/) (if you're testing)
## Setup
1. clone repo
2. insert .env and define `x_TOKENID` where `MODE=X`
```sh
# example .env
MODE=DEV
DEV_TOKENID="..."
```
3. `make` to build image & setup bound data

## Running the Bot
```sh
# docker image
make up

# or with local changes
make dev
```

## Updating
- the bot will check for updates on-startup
```sh
# manually...
make update
make up     # to restart
```

## Overview of Make
```sh
# using
make up     # start
make down   # stop

# updates
make        # for fresh clones
make update # this pulls first


# debug
make logs   # follow output live
make dev    # uses go run instead of docker, MODE=DEV
```