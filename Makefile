# Linux needs the webkit2_41 build tag on distros that only ship
# webkit2gtk-4.1 (see README "Building"); Windows/macOS don't.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	WAILS_TAGS := -tags webkit2_41
endif

.PHONY: dev dev-stop build frontend frontend-watch vet clean

# dev.sh adds the tags itself, plus -assetdir (which is what makes frontend
# edits hot-reload) and detaches, so there's no terminal to keep open. It
# starts the frontend watcher alongside wails dev.
dev:
	./dev.sh

dev-stop:
	./dev.sh --stop

# frontend/dist is generated and gitignored, so every build path that
# embeds it has to produce it first.
frontend:
	go run ./tools/frontendbuild -minify

frontend-watch:
	go run ./tools/frontendbuild -watch

# wails build runs the bundler itself via "frontend:build" in wails.json --
# which is what keeps a bare `wails build` (and CI) correct, not just this
# target -- so there's no frontend prerequisite here.
build:
	wails build $(WAILS_TAGS)

vet:
	go vet ./...

clean:
	find frontend/dist -mindepth 1 ! -name .gitkeep -delete
