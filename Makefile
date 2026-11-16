.PHONY : init built up down logs dev update
.DEFAULT_GOAL := build # -> just make

# setup data/ and link env
init:
	@if [ ! -f .env ]; then \
		echo ".env file not found"; \
		exit 1; \
	fi
	mkdir -p ./data/logs/


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

# add env vars and overwrite MODE
dev: init
	ln -srf ./data ./backend/core/data
	export $$(grep -v '^#' .env | xargs) && \
		cd backend/core && \
			go run -ldflags "-X 'krill-agg/root/info/version.version=dev' -X 'krill-agg/root/info/version.source=local'" .