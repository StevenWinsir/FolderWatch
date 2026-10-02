GO ?= go
GOFLAGS := -mod=vendor
export GOFLAGS
export GOWORK := off
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell git show -s --format=%cI HEAD 2>/dev/null)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test race lint fmt fmt-check run smoke scripts-test fuzz bench performance release release-smoke
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/folderwatch ./cmd/folderwatch
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
lint: fmt-check
	python3 scripts/vendor_guard.py
	python3 scripts/check_core_boundary.py
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
scripts-test:
	python3 -m unittest discover -s scripts -p 'test_*.py' -v
fuzz:
	python3 scripts/fuzz.py
bench:
	$(GO) test ./internal/snapshot ./internal/tui -run='^$$' -bench=. -benchmem -benchtime=1x
performance:
	python3 scripts/performance.py
release:
	python3 scripts/release.py --version $(VERSION)
release-smoke:
	python3 scripts/release_smoke.py dist/$(VERSION)
