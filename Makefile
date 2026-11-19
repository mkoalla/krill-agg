.PHONY : init built up down logs dev update live-daily dev-daily check
.DEFAULT_GOAL := build # -> just make

# setup data/ and link env
init:
	@if [ ! -f .env ]; then \
		echo ".env file not found"; \
		exit 1; \
	fi
	@mkdir -p ./data/logs/ ./data/dailygen/


build: init
	$(MAKE) -C ./backend/ build

up: init
	$(MAKE) -C ./backend/ up

update:
	$(MAKE) down
	git pull
	$(MAKE)

down:
	$(MAKE) -C ./backend/ down

logs:
	$(MAKE) -C ./backend/ logs

# posts today's report (always a fresh card) in every guild's main channel
# (set with /mg-mainchan) from the running bot. for cron
live-daily:
	@ docker compose exec -T main ./main ctl daily

# the docker bot's version and whether a newer release is out. uses the
# running container when there is one, else a throwaway one from the image
check:
	@ if [ -n "$$(docker compose ps --status running -q main)" ]; then \
		docker compose exec -T main ./main check; \
	else \
		docker compose run --rm --no-deps -T main ./main check; \
	fi

# live-daily against a `make dev` bot (its socket is on this host's /tmp)
dev-daily:
	@ cd backend/core && go run . ctl daily

DEV_DAILYGEN_PORT := 40404

# add env vars and overwrite MODE. the card renderer runs alongside on localhost
# and stops with the bot
dev: init
	@ln -srfn ./data ./backend/core/data
	@(cd backend/dailygen && exec uv run python -m dailygen.server --port $(DEV_DAILYGEN_PORT) --data-dir ../../data/dailygen) & \
	DAILYGEN=$$!; trap "kill $$DAILYGEN 2>/dev/null" EXIT INT TERM; \
	export $$(grep -v '^#' .env | xargs) && \
		cd backend/core && \
			go run -ldflags "-X 'krill-agg/root/info/version.version=dev' -X 'krill-agg/root/info/version.source=local' \
				-X 'krill-agg/root/info/dailygen.baseURL=http://127.0.0.1:$(DEV_DAILYGEN_PORT)'" .
