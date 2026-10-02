GO ?= go
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test race lint fmt fmt-check run smoke
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/folderwatch ./cmd/folderwatch
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
lint: fmt-check
	python3 scripts/vendor_guard.py
	$(GO) vet ./...
fmt:
	gofmt -w cmd internal tools vendor/github.com/fsnotify/fsnotify/backend_kqueue.go
fmt-check:
	@test -z "$$(gofmt -l cmd internal tools vendor/github.com/fsnotify/fsnotify/backend_kqueue.go)" || (gofmt -l cmd internal tools vendor/github.com/fsnotify/fsnotify/backend_kqueue.go; exit 1)
run:
	$(GO) run ./cmd/folderwatch $(ARGS)
smoke: build
	python3 scripts/smoke.py bin/folderwatch
	python3 scripts/watch_smoke.py bin/folderwatch
	python3 scripts/semantic_smoke.py bin/folderwatch
	python3 scripts/tui_smoke.py bin/folderwatch
