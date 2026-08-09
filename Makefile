# Linux needs the webkit2_41 build tag on distros that only ship
# webkit2gtk-4.1 (see README "Building"); Windows/macOS don't.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	WAILS_TAGS := -tags webkit2_41
endif

.PHONY: dev build vet

dev:
	wails dev $(WAILS_TAGS)

build:
	wails build $(WAILS_TAGS)

vet:
	go vet ./...
