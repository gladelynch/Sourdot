# Linux needs the webkit2_41 build tag on distros that only ship
# webkit2gtk-4.1 (see README "Building"); Windows/macOS don't.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	WAILS_TAGS := -tags webkit2_41
endif

.PHONY: dev build frontend frontend-watch icons vet clean

# dev.sh adds the tags itself, plus -assetdir (which is what makes frontend
# edits hot-reload), and starts the frontend watcher alongside wails dev.
dev:
	./dev.sh

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

# build/appicon.svg is the one icon source; this regenerates the committed
# copies of it (Wails' PNG/ICO, and the frontend's logo) after it's edited.
# Needs ImageMagick 7 with librsvg. -strip and the excluded chunks keep the
# output byte-identical across runs, so regenerating doesn't churn git.
ICON_RENDER := magick -background none -density 600 build/appicon.svg -depth 8 \
	-strip -define png:exclude-chunks=date,time

icons:
	$(ICON_RENDER) -resize 1024x1024 build/appicon.png
	$(ICON_RENDER) -define icon:auto-resize=256,128,64,48,32,24,16 build/windows/icon.ico
	cp build/appicon.svg frontend/public/assets/images/logo.svg

vet:
	go vet ./...

clean:
	find frontend/dist -mindepth 1 ! -name .gitkeep -delete
